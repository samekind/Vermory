import { afterEach, describe, expect, it, vi } from "vitest";

import plugin from "../src/index.js";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

const TEST_API_TOKEN =
  "vmt_0123456789abcdef01234567_MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY";
const OTHER_API_TOKEN =
  "vmt_89abcdef0123456701234567_YWJjZGVmMDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODk";

describe("Vermory OpenClaw plugin", () => {
  it("registers official lifecycle hooks without claiming a memory slot", () => {
    const harness = registerPlugin();

    expect(plugin.id).toBe("vermory");
    expect(plugin.kind).toBeUndefined();
    expect(harness.options.get("before_prompt_build")).toEqual({ timeoutMs: 15_000 });
    expect(harness.options.get("agent_end")).toEqual({ timeoutMs: 30_000 });
	expect(harness.options.get("after_tool_call")).toEqual({ timeoutMs: 15_000 });
	expect(harness.command?.name).toBe("vermory");
	expect(harness.command?.acceptsArgs).toBe(true);
	expect(harness.command?.requireAuth).toBe(true);
	expect(harness.registerTool).not.toHaveBeenCalled();
  });

  it("captures only successful allowlisted exact-identity tool results", async () => {
	vi.stubEnv("VERMORY_API_TOKEN", TEST_API_TOKEN);
	const requests: Array<{ path: string; body: Record<string, unknown> }> = [];
	const fetchMock = vi.fn(async (input: unknown, init: RequestInit) => {
		const body = JSON.parse(String(init.body)) as Record<string, unknown>;
		requests.push({ path: new URL(String(input)).pathname, body });
		return jsonResponse({
			turn_id: "11111111-1111-1111-1111-111111111111",
			observation_id: "22222222-2222-2222-2222-222222222222",
			tool_name: body.tool_name,
			replayed: false,
		});
	});
	vi.stubGlobal("fetch", fetchMock);
	const harness = registerPlugin({ toolAllowlist: ["device.storage_check"] });

	await harness.afterToolCall(
		{
			toolName: "device.storage_check",
			params: { includeHidden: true },
			runId: "run-tool-hook",
			toolCallId: "call-tool-hook",
			result: { content: [{ type: "text", text: "Storage has 87 GB available and is 82 percent used." }] },
		},
		{
			sessionKey: "agent:main:device",
			runId: "run-tool-hook",
			toolName: "device.storage_check",
			toolCallId: "call-tool-hook",
		},
	);

	expect(requests).toEqual([{
		path: "/v1/integrations/openclaw/turns/tool-results",
		body: {
			operation_id: "openclaw:run-tool-hook",
			session_key: "agent:main:device",
			run_id: "run-tool-hook",
			tool_name: "device.storage_check",
			tool_call_id: "call-tool-hook",
			content: "Storage has 87 GB available and is 82 percent used.",
		},
	}]);
	expect(JSON.stringify(requests)).not.toContain("includeHidden");
  });

  it.each([
	["unallowed", { toolName: "weather", params: {}, runId: "run-ignore", toolCallId: "call-ignore", result: "sunny" }, { sessionKey: "agent:main:a", runId: "run-ignore", toolName: "weather", toolCallId: "call-ignore" }],
	["failed", { toolName: "device.check", params: {}, runId: "run-ignore", toolCallId: "call-ignore", result: "partial", error: "failed" }, { sessionKey: "agent:main:a", runId: "run-ignore", toolName: "device.check", toolCallId: "call-ignore" }],
	["missing identity", { toolName: "device.check", params: {}, result: "ok" }, { sessionKey: "agent:main:a", toolName: "device.check" }],
	["run mismatch", { toolName: "device.check", params: {}, runId: "run-a", toolCallId: "call-a", result: "ok" }, { sessionKey: "agent:main:a", runId: "run-b", toolName: "device.check", toolCallId: "call-a" }],
	["call mismatch", { toolName: "device.check", params: {}, runId: "run-a", toolCallId: "call-a", result: "ok" }, { sessionKey: "agent:main:a", runId: "run-a", toolName: "device.check", toolCallId: "call-b" }],
	["tool mismatch", { toolName: "device.check", params: {}, runId: "run-a", toolCallId: "call-a", result: "ok" }, { sessionKey: "agent:main:a", runId: "run-a", toolName: "device.other", toolCallId: "call-a" }],
	["unsupported result", { toolName: "device.check", params: {}, runId: "run-a", toolCallId: "call-a", result: { output: "hidden" } }, { sessionKey: "agent:main:a", runId: "run-a", toolName: "device.check", toolCallId: "call-a" }],
	["sensitive result", { toolName: "device.check", params: {}, runId: "run-a", toolCallId: "call-a", result: "api_token=W23_SYNTHETIC_SECRET_MUST_NOT_PERSIST" }, { sessionKey: "agent:main:a", runId: "run-a", toolName: "device.check", toolCallId: "call-a" }],
  ])("ignores %s tool events", async (_name, event, context) => {
	const fetchMock = vi.fn();
	vi.stubGlobal("fetch", fetchMock);
	const harness = registerPlugin({ toolAllowlist: ["device.check"] });
	await expect(harness.afterToolCall(event, context)).resolves.toBeUndefined();
	expect(fetchMock).not.toHaveBeenCalled();
	expect(harness.warn).not.toHaveBeenCalled();
  });

  it("keeps tool execution fail-open when persistence fails", async () => {
	vi.stubGlobal("fetch", vi.fn(async () => new Response("private-result", { status: 503 })));
	const harness = registerPlugin({ toolAllowlist: ["device.check"] });
	await expect(harness.afterToolCall(
		{ toolName: "device.check", params: {}, runId: "run-fail-open", toolCallId: "call-fail-open", result: "visible tool result" },
		{ sessionKey: "agent:main:a", runId: "run-fail-open", toolName: "device.check", toolCallId: "call-fail-open" },
	)).resolves.toBeUndefined();
	const warning = harness.warn.mock.calls[0]?.[0] ?? "";
	expect(warning).toContain("tool result");
	expect(warning).not.toContain("visible tool result");
	expect(warning).not.toContain("private-result");
  });

	it("runs direct governance with a separate operator token and session-local short references", async () => {
		vi.stubEnv("VERMORY_API_TOKEN", TEST_API_TOKEN);
		vi.stubEnv("VERMORY_OPERATOR_API_TOKEN", OTHER_API_TOKEN);
		const requests: Array<{ path: string; authorization: string | null; body?: Record<string, unknown> }> = [];
		const fetchMock = vi.fn(async (input: unknown, init: RequestInit) => {
			const path = new URL(String(input)).pathname + new URL(String(input)).search;
			const body = init.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
			requests.push({
				path,
				authorization: new Headers(init.headers).get("authorization"),
				...(body ? { body } : {}),
			});
			if (path.startsWith("/v1/memories/candidates?")) {
				return jsonResponse({
					resolution: { status: "resolved", continuity_id: "continuity-a", channel: "openclaw", thread_id: "agent:main:a", created: false },
					candidates: [
						candidate("aaaaaaaa-1111-1111-1111-111111111111", "submission.bundle.current", "The bundle is thesis-defense-v7.zip.", "tool_result", "device.storage_check"),
						candidate("aaaaaaaa-2222-2222-2222-222222222222", "submission.deadline.current", "The deadline is Tuesday at 18:00."),
					],
				});
			}
			if (path.startsWith("/v1/conversations/inspect?")) {
				return jsonResponse({
					resolution: { status: "resolved", continuity_id: "continuity-a", channel: "openclaw", thread_id: "agent:main:a", created: false },
					observations: [{ id: "private-observation", content: "PRIVATE RAW CHAT" }],
					memories: [{
						id: "bbbbbbbb-1111-1111-1111-111111111111",
						memory_key: "submission.topic.current",
						lifecycle_status: "active",
						content: "The current topic is thesis defense.",
						effective_state: "eligible",
					}],
				});
			}
			if (path === "/v1/memories/correct") {
				return jsonResponse(governanceReceipt("cccccccc-1111-1111-1111-111111111111", "active"));
			}
			const memoryId = String(body?.candidate_memory_id ?? body?.memory_id);
			const status = path.endsWith("/reject") ? "rejected" : path.endsWith("/forget") ? "deleted" : "active";
			return jsonResponse(governanceReceipt(memoryId, status));
		});
		vi.stubGlobal("fetch", fetchMock);
		const harness = registerPlugin();

		const listed = await harness.runCommand("memories", "agent:main:a");
		expect(listed.text).toContain("aaaaaaaa1");
		expect(listed.text).toContain("aaaaaaaa2");
		expect(listed.text).toContain("bbbbbbbb");
		expect(listed.text).toContain("thesis-defense-v7.zip");
		expect(listed.text).toContain("tool device.storage_check");
		expect(listed.text).not.toContain("PRIVATE RAW CHAT");
		expect(listed.text).not.toContain("tool_call_id");
		expect(listed.text).not.toContain("aaaaaaaa-1111-1111-1111-111111111111");

		const callsAfterList = fetchMock.mock.calls.length;
		const ambiguous = await harness.runCommand("accept aaaaaaaa", "agent:main:a");
		expect(ambiguous.text).toContain("ambiguous");
		expect(fetchMock).toHaveBeenCalledTimes(callsAfterList);
		const wrongSession = await harness.runCommand("accept aaaaaaaa1", "agent:main:b");
		expect(wrongSession.text).toContain("/vermory memories");
		expect(fetchMock).toHaveBeenCalledTimes(callsAfterList);

		expect((await harness.runCommand("accept aaaaaaaa1", "agent:main:a")).text).toContain("Accepted");
		expect((await harness.runCommand("reject aaaaaaaa2", "agent:main:a")).text).toContain("Rejected");
		expect((await harness.runCommand("correct bbbbbbbb The current topic is final defense.", "agent:main:a")).text).toContain("Corrected");
		expect((await harness.runCommand("forget cccccccc", "agent:main:a")).text).toContain("Forgotten");

		for (const request of requests) {
			expect(request.authorization).toBe(`Bearer ${OTHER_API_TOKEN}`);
			expect(request.path).not.toContain("PRIVATE RAW CHAT");
		}
		expect(requests.at(-1)?.body).toMatchObject({
			channel: "openclaw",
			thread_id: "agent:main:a",
			memory_id: "cccccccc-1111-1111-1111-111111111111",
		});
	});

	it("keeps normal lifecycle fail-open when operator governance is unavailable", async () => {
		vi.stubEnv("VERMORY_API_TOKEN", TEST_API_TOKEN);
		vi.stubEnv("VERMORY_OPERATOR_API_TOKEN", "");
		const fetchMock = vi.fn(async (_input: unknown, init: RequestInit) => {
			const body = JSON.parse(String(init.body));
			return jsonResponse(prepareReceipt(body.operation_id, "governed context"));
		});
		vi.stubGlobal("fetch", fetchMock);
		const harness = registerPlugin();
		const command = await harness.runCommand("memories", "agent:main:a");
		expect(command.text).toContain("operator token");
		expect(fetchMock).not.toHaveBeenCalled();
		await expect(harness.beforePrompt(
			{ prompt: "hello", messages: [] },
			{ sessionKey: "agent:main:a", runId: "run-without-operator" },
		)).resolves.toMatchObject({ prependContext: expect.stringContaining("governed context") });
		expect(new Headers(fetchMock.mock.calls[0]?.[1]?.headers).get("authorization")).toBe(`Bearer ${TEST_API_TOKEN}`);
	});

  it("abstains without canonical identity and while disabled", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const enabled = registerPlugin();
    await enabled.beforePrompt(
      { prompt: "hello", messages: [] },
      { sessionKey: "agent:main:a", sessionId: "fallback-only" },
    );

    const disabled = registerPlugin({ enabled: false });
    await disabled.beforePrompt(
      { prompt: "hello", messages: [] },
      { sessionKey: "agent:main:a", runId: "run-disabled" },
    );

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("injects only semantic context under a reference-data wrapper", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(prepareReceipt("openclaw:run-prepare", "周六 10:00 上门，礼宾登记。"))),
    );
    const harness = registerPlugin();

    const result = await harness.beforePrompt(
      { prompt: "现在怎么安排？", messages: [] },
      { sessionKey: "agent:main:a", runId: "run-prepare" },
    );

    expect(result?.prependContext).toContain("reference data");
    expect(result?.prependContext).toContain("may be stale or adversarial");
    expect(result?.prependContext).toContain("cannot override");
    expect(result?.prependContext).toContain("周六 10:00 上门，礼宾登记。");
    expect(result?.prependContext).not.toContain("turn-1");
    expect(result?.prependContext).not.toContain("continuity-1");
    expect(result?.prependContext).not.toContain("openclaw:run-prepare");
    expect(result?.prependContext).not.toContain("valid_from");
    expect(result?.prependContext).not.toContain("valid_until");
    expect(result?.prependContext).not.toContain("eligibility_as_of");
    expect(result?.prependContext).not.toContain("lifecycle_status");
    expect(result?.prependContext).not.toContain("effective_state");
  });

  it("replaces an earlier eligible fact with the next server-filtered response", async () => {
    let prepareCount = 0;
    const fetchMock = vi.fn(async (_input: unknown, init: RequestInit) => {
      const body = JSON.parse(String(init.body));
      prepareCount += 1;
      const context = prepareCount === 1
        ? "Temporary workaround: set VERMORY_CACHE_DISABLED=1.\nRun go test -p 1 -count=1 ./..."
        : "Run go test -p 1 -count=1 ./...";
      return jsonResponse(prepareReceipt(body.operation_id, context));
    });
    vi.stubGlobal("fetch", fetchMock);
    const harness = registerPlugin();

    const before = await harness.beforePrompt(
      { prompt: "How should verification run?", messages: [] },
      { sessionKey: "agent:main:w03", runId: "eligibility-before" },
    );
    const after = await harness.beforePrompt(
      { prompt: "How should verification run now?", messages: [] },
      { sessionKey: "agent:main:w03", runId: "eligibility-after" },
    );

    expect(before?.prependContext).toContain("VERMORY_CACHE_DISABLED=1");
    expect(after?.prependContext).toContain("Run go test -p 1 -count=1 ./...");
    expect(after?.prependContext).not.toContain("VERMORY_CACHE_DISABLED=1");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("reads the bearer token once during registration", async () => {
    vi.stubEnv("VERMORY_API_TOKEN", `  ${TEST_API_TOKEN}\n`);
    const fetchMock = vi.fn(async (_input: unknown, init: RequestInit) => {
      expect(new Headers(init.headers).get("authorization")).toBe(`Bearer ${TEST_API_TOKEN}`);
      return jsonResponse(prepareReceipt("openclaw:run-authenticated", "context"));
    });
    vi.stubGlobal("fetch", fetchMock);
    const harness = registerPlugin();
    vi.stubEnv("VERMORY_API_TOKEN", OTHER_API_TOKEN);

    await harness.beforePrompt(
      { prompt: "hello", messages: [] },
      { sessionKey: "agent:main:a", runId: "run-authenticated" },
    );

    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("rejects malformed token environment without exposing its value", () => {
    const malformed = "vmt_bad token_with_private_material";
    vi.stubEnv("VERMORY_API_TOKEN", malformed);
    let message = "";
    try {
      registerPlugin();
    } catch (error) {
      message = error instanceof Error ? error.message : String(error);
    }
    expect(message).toContain("Vermory API token is invalid");
    expect(message).not.toContain(malformed);
  });

  it("does not mutate the prompt for empty context", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(prepareReceipt("openclaw:run-empty", "   "))),
    );
    const harness = registerPlugin();

    const result = await harness.beforePrompt(
      { prompt: "hello", messages: [] },
      { sessionKey: "agent:main:a", runId: "run-empty" },
    );

    expect(result).toBeUndefined();
  });

  it("fails open on prepare errors with a bounded warning", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => {
      throw new Error("secret prompt and database-id-123");
    }));
    const harness = registerPlugin();

    await expect(
      harness.beforePrompt(
        { prompt: "secret prompt", messages: [] },
        { sessionKey: "agent:main:a", runId: "run-error" },
      ),
    ).resolves.toBeUndefined();

    expect(harness.warn).toHaveBeenCalledOnce();
    const warning = harness.warn.mock.calls[0]?.[0] ?? "";
    expect(warning.length).toBeLessThan(256);
    expect(warning).not.toContain("secret prompt");
    expect(warning).not.toContain("database-id-123");
  });

  it("persists the latest visible answer with the resolved model", async () => {
    const fetchMock = vi.fn(async (_input: unknown, init: RequestInit) => {
      const body = JSON.parse(String(init.body));
      return jsonResponse(completedReceipt(body.operation_id));
    });
    vi.stubGlobal("fetch", fetchMock);
    const harness = registerPlugin();

    await harness.agentEnd(
      {
        success: true,
        messages: [
          { role: "assistant", content: "old answer" },
          { role: "assistant", content: [{ type: "text", text: "Saturday at 10:00." }] },
        ],
      },
      {
        sessionKey: "agent:main:a",
        runId: "run-complete",
        modelProviderId: "grok-cli",
        modelId: "grok-4.5",
      },
    );

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(url)).toContain("/turns/complete");
    expect(JSON.parse(String(init?.body))).toEqual({
      operation_id: "openclaw:run-complete",
      session_key: "agent:main:a",
      answer: "Saturday at 10:00.",
      model: "grok-cli/grok-4.5",
    });
  });

	it("runs a fenced tool loop through leased prepare, tool evidence, checkpoint, and completion", async () => {
		const requests: Array<{ path: string; body: Record<string, unknown> }> = [];
		const fetchMock = vi.fn(async (input: unknown, init: RequestInit) => {
			const path = new URL(String(input)).pathname;
			const body = JSON.parse(String(init.body)) as Record<string, unknown>;
			requests.push({ path, body });
			if (path === "/v1/client-operations/prepare") {
				return jsonResponse(prepareReceipt(String(body.operation_id), "current governed context"));
			}
			if (path.endsWith("/tool-results")) {
				return jsonResponse({
					turn_id: "22222222-2222-2222-2222-222222222222",
					observation_id: "33333333-3333-3333-3333-333333333333",
					tool_name: body.tool_name,
					replayed: false,
				});
			}
			if (path === "/v1/client-operations/checkpoint") {
				return jsonResponse({
					...prepareReceipt(String(body.operation_id), ""),
					checkpoint_sequence: body.sequence,
					checkpoint: body.checkpoint,
				});
			}
			return jsonResponse(completedReceipt(String(body.operation_id)));
		});
		vi.stubGlobal("fetch", fetchMock);
		const harness = registerPlugin({ toolAllowlist: ["release.verify"] });

		await harness.beforePrompt(
			{ prompt: "Continue the release repair.", messages: [] },
			{ sessionKey: "agent:main:leased", runId: "leased-tool-loop" },
		);
		await harness.afterToolCall(
			{
				toolName: "release.verify",
				params: { hidden: "must-not-persist" },
				runId: "leased-tool-loop",
				toolCallId: "call-verify",
				result: "Verification passed.",
			},
			{
				sessionKey: "agent:main:leased",
				runId: "leased-tool-loop",
				toolName: "release.verify",
				toolCallId: "call-verify",
			},
		);
		await harness.agentEnd(
			{ success: true, messages: [{ role: "assistant", content: "Release repair completed." }] },
			{ sessionKey: "agent:main:leased", runId: "leased-tool-loop", modelId: "runtime-model" },
		);

		expect(requests.map((request) => request.path)).toEqual([
			"/v1/client-operations/prepare",
			"/v1/integrations/openclaw/turns/tool-results",
			"/v1/client-operations/checkpoint",
			"/v1/client-operations/complete",
		]);
		expect(requests[1]?.body).toMatchObject({
			attempt_id: "11111111-1111-1111-1111-111111111111",
			lease_generation: 1,
		});
		expect(requests[2]?.body).toMatchObject({
			sequence: 1,
			checkpoint: { phase: "tool_completed", tool_name: "release.verify", tool_call_id: "call-verify" },
		});
		expect(requests[3]?.body).toMatchObject({
			attempt_id: "11111111-1111-1111-1111-111111111111",
			lease_generation: 1,
			answer: "Release repair completed.",
		});
		expect(JSON.stringify(requests)).not.toContain("must-not-persist");
	});

	it("reconstructs leased attempt metadata by repeating prepare after plugin restart", async () => {
		const requests: Array<{ path: string; body: Record<string, unknown> }> = [];
		const fetchMock = vi.fn(async (input: unknown, init: RequestInit) => {
			const path = new URL(String(input)).pathname;
			const body = JSON.parse(String(init.body)) as Record<string, unknown>;
			requests.push({ path, body });
			if (path === "/v1/client-operations/prepare") {
				return jsonResponse({
					...prepareReceipt(String(body.operation_id), "restart-safe context"),
					replayed: requests.filter((request) => request.path === path).length > 1,
					checkpoint_sequence: 3,
					checkpoint: { phase: "resume", completed_steps: 3 },
				});
			}
			return jsonResponse({
				...completedReceipt(String(body.operation_id)),
				checkpoint_sequence: 3,
				checkpoint: { phase: "resume", completed_steps: 3 },
			});
		});
		vi.stubGlobal("fetch", fetchMock);

		const beforeRestart = registerPlugin();
		await beforeRestart.beforePrompt(
			{ prompt: "Continue after restart.", messages: [] },
			{ sessionKey: "agent:main:restart", runId: "restart-run" },
		);
		const afterRestart = registerPlugin();
		await afterRestart.beforePrompt(
			{ prompt: "Continue after restart.", messages: [] },
			{ sessionKey: "agent:main:restart", runId: "restart-run" },
		);
		await afterRestart.agentEnd(
			{ success: true, messages: [{ role: "assistant", content: "Recovered completion." }] },
			{ sessionKey: "agent:main:restart", runId: "restart-run", modelId: "runtime-model" },
		);

		expect(requests.map((request) => request.path)).toEqual([
			"/v1/client-operations/prepare",
			"/v1/client-operations/prepare",
			"/v1/client-operations/complete",
		]);
		expect(requests[2]?.body).toMatchObject({
			attempt_id: "11111111-1111-1111-1111-111111111111",
			lease_generation: 1,
		});
	});

  it("uses model metadata from the visible assistant message when hook context omits it", async () => {
    const fetchMock = vi.fn(async (_input: unknown, init: RequestInit) => {
      const body = JSON.parse(String(init.body));
      return jsonResponse(completedReceipt(body.operation_id));
    });
    vi.stubGlobal("fetch", fetchMock);
    const harness = registerPlugin();

    await harness.agentEnd(
      {
        success: true,
        messages: [
          {
            role: "assistant",
            content: [{ type: "text", text: "NO_GOVERNED_MEMORY_YET" }],
            api: "cli",
            provider: "grok-cli",
            model: "grok-4.5",
          },
        ],
      },
      {
        sessionKey: "agent:main:home-maintenance-a",
        runId: "311d7e80-9443-4ec6-9143-936dde1746ad",
      },
    );

    const [, init] = fetchMock.mock.calls[0] ?? [];
    expect(JSON.parse(String(init?.body)).model).toBe("grok-cli/grok-4.5");
  });

  it.each([
    [false, [{ role: "assistant", content: "partial answer" }], "openclaw_agent_error"],
    [true, [{ role: "assistant", content: [{ type: "tool_call", name: "calendar" }] }], "openclaw_empty_output"],
  ])("records failed lifecycle instead of a false completion", async (success, messages, failureCode) => {
    const fetchMock = vi.fn(async (_input: unknown, init: RequestInit) => {
      const body = JSON.parse(String(init.body));
      return jsonResponse(failedReceipt(body.operation_id, body.failure_code));
    });
    vi.stubGlobal("fetch", fetchMock);
    const harness = registerPlugin();

    await harness.agentEnd(
      { success, messages },
      { sessionKey: "agent:main:a", runId: `run-${failureCode}` },
    );

    const [url, init] = fetchMock.mock.calls[0] ?? [];
    expect(String(url)).toContain("/turns/fail");
    const body = JSON.parse(String(init?.body));
    expect(body.failure_code).toBe(failureCode);
    expect(body.failure_message.length).toBeLessThan(256);
    expect(body).not.toHaveProperty("answer");
  });

  it("does not throw completion persistence failures into OpenClaw", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("unavailable-secret", { status: 503 })));
    const harness = registerPlugin();

    await expect(
      harness.agentEnd(
        { success: true, messages: [{ role: "assistant", content: "visible answer" }] },
        {
          sessionKey: "agent:main:a",
          runId: "run-persist-error",
          modelProviderId: "grok-cli",
          modelId: "grok-4.5",
        },
      ),
    ).resolves.toBeUndefined();

    const warning = harness.warn.mock.calls[0]?.[0] ?? "";
    expect(warning).not.toContain("visible answer");
    expect(warning).not.toContain("unavailable-secret");
  });
});

function registerPlugin(pluginConfig: Record<string, unknown> = {}) {
  const hooks = new Map<string, Function>();
  const options = new Map<string, unknown>();
  const warn = vi.fn();
	let command: Record<string, unknown> | undefined;
	const registerTool = vi.fn();
  plugin.register({
    pluginConfig,
    logger: { info: vi.fn(), warn, error: vi.fn() },
    on(name: string, handler: Function, hookOptions: unknown) {
      hooks.set(name, handler);
      options.set(name, hookOptions);
    },
	registerCommand(definition: Record<string, unknown>) {
		command = definition;
	},
	registerTool,
  } as never);

  return {
    options,
    warn,
	command,
	registerTool,
	runCommand: async (args: string, sessionKey: string, isAuthorizedSender = true) => {
		if (!command || typeof command.handler !== "function") {
			throw new Error("Vermory command was not registered");
		}
		return command.handler({
			args,
			commandBody: `/vermory ${args}`,
			channel: "webchat",
			isAuthorizedSender,
			sessionKey,
			config: {},
		}) as Promise<{ text?: string }>;
	},
    beforePrompt: hooks.get("before_prompt_build") as (
      event: { prompt: string; messages: unknown[] },
      context: Record<string, unknown>,
    ) => Promise<{ prependContext?: string } | undefined>,
    agentEnd: hooks.get("agent_end") as (
      event: { success: boolean; messages: unknown[] },
      context: Record<string, unknown>,
    ) => Promise<void>,
	afterToolCall: hooks.get("after_tool_call") as (
	  event: Record<string, unknown>,
	  context: Record<string, unknown>,
	) => Promise<void>,
  };
}

function candidate(
	id: string,
	key: string,
	content: string,
	sourceKind: "user_message" | "tool_result" = "user_message",
	sourceLabel?: string,
) {
	return {
		candidate_memory_id: id,
		memory_key: key,
		content,
		source_quote: content,
		source_observation_id: "dddddddd-1111-1111-1111-111111111111",
		source_kind: sourceKind,
		...(sourceLabel ? { source_label: sourceLabel } : {}),
		decision: "new",
		created_at: "2026-07-18T00:00:00Z",
	};
}

function governanceReceipt(memoryId: string, status: string) {
	return {
		observation: { observation_id: "eeeeeeee-1111-1111-1111-111111111111", replayed: false },
		memory: { memory_id: memoryId, status, replayed: false },
	};
}

function jsonResponse(value: unknown): Response {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function prepareReceipt(operationId: string, context: string) {
  return {
    turn_id: "turn-1",
    operation_id: operationId,
    status: "in_progress",
	protocol: "leased_v1",
    continuity_id: "continuity-1",
    delivery_id: "delivery-1",
    user_observation_id: "observation-user-1",
	attempt_id: "11111111-1111-1111-1111-111111111111",
	lease_generation: 1,
	lease_expires_at: "2026-07-23T01:00:00Z",
	checkpoint_sequence: 0,
	checkpoint: {},
    replayed: false,
    context,
  };
}

function completedReceipt(operationId: string) {
  return {
    ...prepareReceipt(operationId, ""),
    status: "completed",
    assistant_observation_id: "observation-assistant-1",
    answer: "Saturday at 10:00.",
    model: "grok-cli/grok-4.5",
  };
}

function failedReceipt(operationId: string, failureCode: string) {
  return {
    ...prepareReceipt(operationId, ""),
    status: "failed",
    failure_code: failureCode,
  };
}
