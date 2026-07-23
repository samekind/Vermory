from __future__ import annotations

import json
import os
import sys
import tempfile
import threading
import types
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


agent_module = types.ModuleType("agent")
memory_provider_module = types.ModuleType("agent.memory_provider")


class MemoryProvider:
    pass


memory_provider_module.MemoryProvider = MemoryProvider
sys.modules.setdefault("agent", agent_module)
sys.modules["agent.memory_provider"] = memory_provider_module
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from vermory import VermoryMemoryProvider  # noqa: E402


class RecordingHandler(BaseHTTPRequestHandler):
    requests = []

    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(size))
        self.__class__.requests.append(
            {"path": self.path, "body": body, "authorization": self.headers.get("Authorization")}
        )
        phase = self.path.rsplit("/", 1)[-1]
        status = {"prepare": "in_progress", "complete": "completed", "fail": "failed"}[phase]
        response = {
            "turn_id": "turn-1",
            "operation_id": body["operation_id"],
            "status": status,
            "continuity_id": "continuity-1",
            "delivery_id": "delivery-1",
            "user_observation_id": "user-observation-1",
            "replayed": False,
        }
        if phase == "prepare":
            response["context"] = "The durable appointment is Saturday at 10:00."
        elif phase == "complete":
            response.update(
                {
                    "assistant_observation_id": "assistant-observation-1",
                    "answer": body["answer"],
                    "model": body["model"],
                }
            )
        else:
            response["failure_code"] = body["failure_code"]
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, format, *args):
        return


class VermoryMemoryProviderTest(unittest.TestCase):
    def setUp(self):
        RecordingHandler.requests = []
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), RecordingHandler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.temp_dir = tempfile.TemporaryDirectory()
        self.previous = dict(os.environ)
        os.environ["HERMES_HOME"] = self.temp_dir.name
        os.environ["VERMORY_API_TOKEN"] = "test-token"
        config = {
            "base_url": f"http://127.0.0.1:{self.server.server_port}",
            "timeout_seconds": 2,
        }
        Path(self.temp_dir.name, "vermory.json").write_text(json.dumps(config), encoding="utf-8")

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.temp_dir.cleanup()
        os.environ.clear()
        os.environ.update(self.previous)

    def test_prefetch_and_sync_use_hermes_lifecycle(self):
        provider = VermoryMemoryProvider()
        self.assertTrue(provider.is_available())
        provider.initialize("cli-session-a", platform="cli")
        provider.on_turn_start(1, "continue", model="hermes/test-model")

        context = provider.prefetch("Continue the appointment.", session_id="cli-session-a")
        self.assertIn("Vermory governed reference data", context)
        self.assertIn("Saturday at 10:00", context)
        provider.sync_turn(
            "Continue the appointment.",
            "The appointment remains Saturday at 10:00.",
            session_id="cli-session-a",
        )

        self.assertEqual(
            [request["path"] for request in RecordingHandler.requests],
            [
                "/v1/integrations/hermes/turns/prepare",
                "/v1/integrations/hermes/turns/complete",
            ],
        )
        prepare = RecordingHandler.requests[0]
        complete = RecordingHandler.requests[1]
        self.assertEqual(prepare["authorization"], "Bearer test-token")
        self.assertEqual(prepare["body"]["session_key"], "session:cli-session-a")
        self.assertEqual(complete["body"]["operation_id"], prepare["body"]["operation_id"])
        self.assertEqual(complete["body"]["model"], "hermes/test-model")
        self.assertEqual(provider.get_tool_schemas(), [])

    def test_process_model_fallback_is_recorded_when_hook_omits_model(self):
        os.environ["HERMES_INFERENCE_MODEL"] = "deepseek-ai/DeepSeek-V4-Flash"
        provider = VermoryMemoryProvider()
        provider.initialize("cli-session-model-fallback", platform="cli")
        provider.on_turn_start(1, "Continue without model kwargs.")

        provider.prefetch(
            "Continue without model kwargs.",
            session_id="cli-session-model-fallback",
        )
        provider.sync_turn(
            "Continue without model kwargs.",
            "Completed with the process-selected model.",
            session_id="cli-session-model-fallback",
        )

        complete = RecordingHandler.requests[1]
        self.assertEqual(
            complete["body"]["model"],
            "deepseek-ai/DeepSeek-V4-Flash",
        )

    def test_gateway_anchor_survives_internal_session_switch(self):
        provider = VermoryMemoryProvider()
        provider.initialize(
            "gateway-session-a",
            platform="telegram",
            gateway_session_key="telegram:chat:42:thread:7",
        )
        provider.on_session_switch("gateway-session-b")
        provider.prefetch("Continue.", session_id="gateway-session-b")

        self.assertEqual(
            RecordingHandler.requests[0]["body"]["session_key"],
            "gateway:telegram:chat:42:thread:7",
        )

    def test_empty_assistant_is_recorded_as_failed_turn(self):
        provider = VermoryMemoryProvider()
        provider.initialize("cli-session-failure", platform="cli")
        provider.prefetch("This turn will fail.", session_id="cli-session-failure")
        provider.sync_turn("This turn will fail.", "", session_id="cli-session-failure")

        self.assertEqual(
            [request["path"] for request in RecordingHandler.requests],
            [
                "/v1/integrations/hermes/turns/prepare",
                "/v1/integrations/hermes/turns/fail",
            ],
        )
        self.assertEqual(
            RecordingHandler.requests[1]["body"]["failure_code"],
            "hermes_empty_output",
        )

    def test_invalid_or_unavailable_service_fails_open(self):
        Path(self.temp_dir.name, "vermory.json").write_text(
            json.dumps({"base_url": "http://user:pass@example.test"}), encoding="utf-8"
        )
        self.assertFalse(VermoryMemoryProvider().is_available())

        Path(self.temp_dir.name, "vermory.json").write_text(
            json.dumps({"base_url": "http://127.0.0.1:1", "timeout_seconds": 0.25}),
            encoding="utf-8",
        )
        provider = VermoryMemoryProvider()
        provider.initialize("offline", platform="cli")
        self.assertEqual(provider.prefetch("Continue.", session_id="offline"), "")

    def test_direct_setup_activates_provider_and_writes_native_config(self):
        Path(self.temp_dir.name, "vermory.json").write_text("{}", encoding="utf-8")
        hermes_module = types.ModuleType("hermes_cli")
        config_module = types.ModuleType("hermes_cli.config")
        saved = {}

        def save_config(config):
            saved["config"] = config

        config_module.save_config = save_config
        previous_hermes = sys.modules.get("hermes_cli")
        previous_config = sys.modules.get("hermes_cli.config")
        sys.modules["hermes_cli"] = hermes_module
        sys.modules["hermes_cli.config"] = config_module
        try:
            provider = VermoryMemoryProvider()
            provider.post_setup(self.temp_dir.name, {})
        finally:
            if previous_hermes is None:
                sys.modules.pop("hermes_cli", None)
            else:
                sys.modules["hermes_cli"] = previous_hermes
            if previous_config is None:
                sys.modules.pop("hermes_cli.config", None)
            else:
                sys.modules["hermes_cli.config"] = previous_config

        self.assertEqual(saved["config"]["memory"]["provider"], "vermory")
        native = json.loads(Path(self.temp_dir.name, "vermory.json").read_text())
        self.assertEqual(native["base_url"], "http://127.0.0.1:8787")


if __name__ == "__main__":
    unittest.main()
