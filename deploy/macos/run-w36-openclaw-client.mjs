#!/usr/bin/env node

import { pathToFileURL } from "node:url";

const [pluginModulePath, clientModulePath, mode, baseUrl, sessionKey, operationId, message] = process.argv.slice(2);
const apiToken = process.env.VERMORY_API_TOKEN?.trim();

if (!pluginModulePath || !clientModulePath || !["start", "resume-complete"].includes(mode) ||
  !baseUrl || !sessionKey || !operationId || !message || !apiToken) {
  throw new Error(
    "usage: VERMORY_API_TOKEN=... run-w36-openclaw-client.mjs /path/to/plugin.js /path/to/client.js <start|resume-complete> <base-url> <session-key> <operation-id> <message>",
  );
}

const [{ default: plugin }, { VermoryClient }] = await Promise.all([
  import(pathToFileURL(pluginModulePath).href),
  import(pathToFileURL(clientModulePath).href),
]);
const hooks = new Map();
const warnings = [];
plugin.register({
  pluginConfig: {
    enabled: true,
    baseUrl,
    timeoutMs: 10_000,
    toolAllowlist: ["release.verify"],
  },
  logger: {
    info() {},
    warn(messageValue) { warnings.push(String(messageValue)); },
    error() {},
  },
  on(name, handler) { hooks.set(name, handler); },
  registerCommand() {},
  registerTool() { throw new Error("Vermory must not claim an OpenClaw tool slot"); },
});

const beforePrompt = requireHook("before_prompt_build");
const afterToolCall = requireHook("after_tool_call");
const agentEnd = requireHook("agent_end");
const runId = operationId.replace(/^openclaw:/, "");
const context = { sessionKey, runId, modelProviderId: "openclaw", modelId: "w36-runtime" };
const promptResult = await beforePrompt({ prompt: message, messages: [] }, context);

await afterToolCall({
  toolName: "release.verify",
  params: { private_runtime_input: "must-not-persist" },
  runId,
  toolCallId: mode === "start" ? "w36-call-1" : "w36-call-2",
  result: mode === "start"
    ? "Release verification passed before the client and database restart."
    : "Release verification passed after the client and database restart.",
}, {
  sessionKey,
  runId,
  toolName: "release.verify",
  toolCallId: mode === "start" ? "w36-call-1" : "w36-call-2",
});

if (mode === "resume-complete") {
  await agentEnd({
    success: true,
    messages: [{
      role: "assistant",
      content: "The authenticated OpenClaw plugin resumed after client, service, and PostgreSQL restart and completed the operation.",
    }],
  }, context);
}
if (warnings.length !== 0) {
  throw new Error(`OpenClaw plugin emitted warnings: ${warnings.join("; ")}`);
}

const client = new VermoryClient({ baseUrl, timeoutMs: 10_000, apiToken });
const receipt = await client.prepareLeased({ operationId, sessionKey, message });
const expectedStatus = mode === "start" ? "in_progress" : "completed";
if (receipt.status !== expectedStatus) {
  throw new Error(`operation status is ${receipt.status}, expected ${expectedStatus}`);
}

process.stdout.write(`${JSON.stringify({
  mode,
  turn_id: receipt.turnId,
  operation_id: receipt.operationId,
  status: receipt.status,
  protocol: receipt.protocol,
  continuity_id: receipt.continuityId,
  delivery_id: receipt.deliveryId,
  attempt_id: receipt.attemptId,
  lease_generation: receipt.leaseGeneration,
  checkpoint_sequence: receipt.checkpointSequence,
  replayed_prepare: receipt.replayed,
  context_injected: typeof promptResult?.prependContext === "string" && promptResult.prependContext.includes("Default user-facing replies are written in Chinese."),
  assistant_observation_id: receipt.assistantObservationId ?? "",
})}\n`);

function requireHook(name) {
  const hook = hooks.get(name);
  if (typeof hook !== "function") {
    throw new Error(`OpenClaw plugin did not register ${name}`);
  }
  return hook;
}
