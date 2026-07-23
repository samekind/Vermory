#!/usr/bin/env node

import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

const [runtimeModulePath, configPath, method, rawParams] = process.argv.slice(2);
const allowedMethods = new Set(["chat.send", "sessions.reset"]);

if (!runtimeModulePath || !configPath || !allowedMethods.has(method) || !rawParams) {
  throw new Error(
    "usage: openclaw-loopback-backend-call.mjs /path/to/gateway-runtime.js /path/to/openclaw.json <chat.send|sessions.reset> '{...}'",
  );
}

const config = JSON.parse(await readFile(configPath, "utf8"));
const port = config?.gateway?.port;
const token = config?.gateway?.auth?.token;
if (!Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error("OpenClaw config does not contain a valid gateway port");
}
if (typeof token !== "string" || token.trim() === "") {
  throw new Error("OpenClaw config does not contain a gateway token");
}

const params = JSON.parse(rawParams);
if (!params || typeof params !== "object" || Array.isArray(params)) {
  throw new Error("chat.send params must be a JSON object");
}

const { GatewayClient } = await import(pathToFileURL(runtimeModulePath).href);
let resolveReady;
let rejectReady;
const ready = new Promise((resolve, reject) => {
  resolveReady = resolve;
  rejectReady = reject;
});
const timeout = setTimeout(() => rejectReady(new Error("gateway connection timeout")), 10_000);
const client = new GatewayClient({
  url: `ws://127.0.0.1:${port}`,
  token,
  clientName: "gateway-client",
  clientDisplayName: "Vermory loopback backend",
  clientVersion: "0.1.0",
  platform: process.platform,
  mode: "backend",
  role: "operator",
  scopes: [
    "operator.admin",
    "operator.approvals",
    "operator.pairing",
    "operator.read",
    "operator.talk.secrets",
    "operator.write",
  ],
  deviceIdentity: null,
  onHelloOk: () => resolveReady(),
  onConnectError: (error) => rejectReady(error),
});

try {
  client.start();
  await ready;
  clearTimeout(timeout);
  const result = await client.request(method, params, {
    expectFinal: method === "chat.send",
    timeoutMs: 30_000,
  });
  process.stdout.write(`${JSON.stringify(result)}\n`);
} finally {
  clearTimeout(timeout);
  await client.stopAndWait({ timeoutMs: 2_000 });
}
