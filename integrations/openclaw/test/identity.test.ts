import { describe, expect, it } from "vitest";

import { resolveTurnIdentity } from "../src/identity.js";

describe("resolveTurnIdentity", () => {
  it("maps canonical OpenClaw identity without reinterpretation", () => {
    expect(
      resolveTurnIdentity({
        sessionKey: "  agent:main:home-maintenance-a  ",
        runId: "  4da22893-a47f-4d33-b1dc-40b285f3d201  ",
        sessionId: "ignored-session-id",
        channel: "ignored-channel",
        senderId: "ignored-sender",
      }),
    ).toEqual({
      sessionKey: "agent:main:home-maintenance-a",
      operationId: "openclaw:4da22893-a47f-4d33-b1dc-40b285f3d201",
    });
  });

  it.each([
    ["missing session key", { runId: "run-1", sessionId: "fallback" }],
    ["blank session key", { sessionKey: "  ", runId: "run-1" }],
    ["missing run id", { sessionKey: "agent:main:a", sessionId: "fallback" }],
    ["blank run id", { sessionKey: "agent:main:a", runId: "  " }],
  ])("abstains for %s", (_name, context) => {
    expect(resolveTurnIdentity(context)).toBeUndefined();
  });
});
