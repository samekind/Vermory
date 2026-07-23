import { createServer } from "node:http";
import type { AddressInfo } from "node:net";

import { describe, expect, it } from "vitest";

import { VermoryClient } from "../src/client.js";

const TEST_API_TOKEN =
  "vmt_0123456789abcdef01234567_MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY";

describe("VermoryClient", () => {
  it("posts an exact prepare request and validates the receipt", async () => {
    await withServer(async (request, response) => {
      expect(request.method).toBe("POST");
      expect(request.url).toBe("/v1/integrations/openclaw/turns/prepare");
      expect(request.headers["content-type"]).toBe("application/json");
      expect(request.headers.authorization).toBeUndefined();
      expect(JSON.parse(await readRequest(request))).toEqual({
        operation_id: "openclaw:run-1",
        session_key: "agent:main:a",
        message: "What is the current appointment?",
      });
      writeJSON(response, prepareReceipt("openclaw:run-1", "Use Saturday at 10:00."));
    }, async (baseUrl) => {
      const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
      const receipt = await client.prepare({
        operationId: "openclaw:run-1",
        sessionKey: "agent:main:a",
        message: "What is the current appointment?",
      });

      expect(receipt.context).toBe("Use Saturday at 10:00.");
      expect(receipt.status).toBe("in_progress");
    });
  });

  it("uses each server-filtered context without retaining an earlier eligible fact", async () => {
    let prepareCount = 0;
    await withServer(async (request, response) => {
      const body = JSON.parse(await readRequest(request));
      prepareCount += 1;
      if (prepareCount === 1) {
        writeJSON(
          response,
          prepareReceipt(body.operation_id, "Temporary workaround: set VERMORY_CACHE_DISABLED=1.\nRun go test -p 1 -count=1 ./..."),
        );
        return;
      }
      writeJSON(response, prepareReceipt(body.operation_id, "Run go test -p 1 -count=1 ./..."));
    }, async (baseUrl) => {
      const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
      const before = await client.prepare({
        operationId: "openclaw:eligibility-before",
        sessionKey: "agent:main:w03",
        message: "How should verification run?",
      });
      const after = await client.prepare({
        operationId: "openclaw:eligibility-after",
        sessionKey: "agent:main:w03",
        message: "How should verification run now?",
      });

      expect(before.context).toContain("VERMORY_CACHE_DISABLED=1");
      expect(after.context).toBe("Run go test -p 1 -count=1 ./...");
      expect(after.context).not.toContain("VERMORY_CACHE_DISABLED=1");
      expect(prepareCount).toBe(2);
    });
  });

  it("adds one exact bearer header without exposing it elsewhere", async () => {
    await withServer(async (request, response) => {
      expect(request.headers.authorization).toBe(`Bearer ${TEST_API_TOKEN}`);
      writeJSON(response, prepareReceipt("openclaw:authenticated", "governed context"));
    }, async (baseUrl) => {
      const client = new VermoryClient({ baseUrl, timeoutMs: 1000, apiToken: `  ${TEST_API_TOKEN}\n` });
      await client.prepare({
        operationId: "openclaw:authenticated",
        sessionKey: "agent:main:a",
        message: "hello",
      });
    });
  });

  it.each([
    `vmt_0123456789abcdef01234567_bad secret`,
    `vmt_0123456789abcdef01234567_bad\u0000secret`,
    "not-a-vermory-token",
  ])("rejects malformed bearer material without echoing it", (apiToken) => {
    let message = "";
    try {
      new VermoryClient({ baseUrl: "http://127.0.0.1:8787", timeoutMs: 1000, apiToken });
    } catch (error) {
      message = error instanceof Error ? error.message : String(error);
    }
    expect(message).toContain("Vermory API token is invalid");
    expect(message).not.toContain(apiToken);
  });

  it("posts exact complete and fail requests", async () => {
    const requests: Array<{ path: string; body: unknown }> = [];
    await withServer(async (request, response) => {
      const body = JSON.parse(await readRequest(request));
      requests.push({ path: request.url ?? "", body });
      if (request.url?.endsWith("/complete")) {
        writeJSON(response, completedReceipt(body.operation_id));
        return;
      }
      writeJSON(response, failedReceipt(body.operation_id, body.failure_code));
    }, async (baseUrl) => {
      const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
      await client.complete({
        operationId: "openclaw:run-complete",
        sessionKey: "agent:main:a",
        answer: "The appointment is Saturday at 10:00.",
        model: "grok-cli/grok-4.5",
      });
      await client.fail({
        operationId: "openclaw:run-fail",
        sessionKey: "agent:main:a",
        failureCode: "openclaw_agent_error",
        failureMessage: "no visible answer",
      });
    });

    expect(requests).toEqual([
      {
        path: "/v1/integrations/openclaw/turns/complete",
        body: {
          operation_id: "openclaw:run-complete",
          session_key: "agent:main:a",
          answer: "The appointment is Saturday at 10:00.",
          model: "grok-cli/grok-4.5",
        },
      },
      {
        path: "/v1/integrations/openclaw/turns/fail",
        body: {
          operation_id: "openclaw:run-fail",
          session_key: "agent:main:a",
          failure_code: "openclaw_agent_error",
          failure_message: "no visible answer",
        },
      },
    ]);
  });

	it("posts and validates the leased operation lifecycle", async () => {
		const requests: Array<{ path: string; body: Record<string, unknown> }> = [];
		await withServer(async (request, response) => {
			const body = JSON.parse(await readRequest(request)) as Record<string, unknown>;
			const path = request.url ?? "";
			requests.push({ path, body });
			if (path.endsWith("/prepare")) {
				writeJSON(response, leasedReceipt(String(body.operation_id), "governed context"));
				return;
			}
			if (path.endsWith("/checkpoint")) {
				writeJSON(response, {
					...leasedReceipt(String(body.operation_id), ""),
					checkpoint_sequence: body.sequence,
					checkpoint: body.checkpoint,
				});
				return;
			}
			writeJSON(response, {
				...leasedReceipt(String(body.operation_id), ""),
				status: "completed",
				lease_expires_at: undefined,
				assistant_observation_id: "22222222-2222-2222-2222-222222222222",
				answer: body.answer,
				model: body.model,
			});
		}, async (baseUrl) => {
			const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
			const prepared = await client.prepareLeased({
				operationId: "openclaw:leased-client",
				sessionKey: "agent:main:leased-client",
				message: "Continue the tool loop.",
			});
			const checkpoint = await client.checkpointLeased({
				operationId: prepared.operationId,
				sessionKey: "agent:main:leased-client",
				attemptId: prepared.attemptId,
				leaseGeneration: prepared.leaseGeneration,
				sequence: 1,
				checkpoint: { phase: "tool_completed" },
			});
			expect(checkpoint.checkpointSequence).toBe(1);
			await client.completeLeased({
				operationId: prepared.operationId,
				sessionKey: "agent:main:leased-client",
				attemptId: prepared.attemptId,
				leaseGeneration: prepared.leaseGeneration,
				answer: "Completed.",
				model: "openclaw/test",
			});
		});

		expect(requests).toEqual([
			{
				path: "/v1/client-operations/prepare",
				body: {
					operation_id: "openclaw:leased-client",
					channel: "openclaw",
					thread_id: "agent:main:leased-client",
					message: "Continue the tool loop.",
				},
			},
			{
				path: "/v1/client-operations/checkpoint",
				body: {
					operation_id: "openclaw:leased-client",
					channel: "openclaw",
					thread_id: "agent:main:leased-client",
					attempt_id: "11111111-1111-1111-1111-111111111111",
					lease_generation: 1,
					sequence: 1,
					checkpoint: { phase: "tool_completed" },
				},
			},
			{
				path: "/v1/client-operations/complete",
				body: {
					operation_id: "openclaw:leased-client",
					channel: "openclaw",
					thread_id: "agent:main:leased-client",
					attempt_id: "11111111-1111-1111-1111-111111111111",
					lease_generation: 1,
					answer: "Completed.",
					model: "openclaw/test",
				},
			},
		]);
	});

	it("accepts a terminal leased receipt when prepare is replayed after completion", async () => {
		await withServer(async (_request, response) => {
			writeJSON(response, {
				...leasedReceipt("openclaw:leased-terminal-replay", ""),
				status: "completed",
				lease_expires_at: undefined,
				assistant_observation_id: "22222222-2222-2222-2222-222222222222",
				answer: "Already completed.",
				model: "openclaw/test",
				replayed: true,
			});
		}, async (baseUrl) => {
			const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
			const receipt = await client.prepareLeased({
				operationId: "openclaw:leased-terminal-replay",
				sessionKey: "agent:main:leased-terminal-replay",
				message: "Same request.",
			});
			expect(receipt.status).toBe("completed");
			expect(receipt.replayed).toBe(true);
		});
	});

  it("posts an exact bounded tool-result request and validates its receipt", async () => {
	await withServer(async (request, response) => {
		expect(request.method).toBe("POST");
		expect(request.url).toBe("/v1/integrations/openclaw/turns/tool-results");
		expect(JSON.parse(await readRequest(request))).toEqual({
			operation_id: "openclaw:run-tool-1",
			session_key: "agent:main:a",
			run_id: "run-tool-1",
			tool_name: "device.storage_check",
			tool_call_id: "call-storage-1",
			content: "Storage has 87 GB available and is 82 percent used.",
		});
		writeJSON(response, {
			turn_id: "11111111-1111-1111-1111-111111111111",
			observation_id: "22222222-2222-2222-2222-222222222222",
			tool_name: "device.storage_check",
			replayed: false,
		});
	}, async (baseUrl) => {
		const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
		const receipt = await client.recordToolResult({
			operationId: "openclaw:run-tool-1",
			sessionKey: "agent:main:a",
			runId: "run-tool-1",
			toolName: "device.storage_check",
			toolCallId: "call-storage-1",
			content: "Storage has 87 GB available and is 82 percent used.",
		});
		expect(receipt.toolName).toBe("device.storage_check");
		expect(receipt.replayed).toBe(false);
	});
  });

  it("uses an operator token for scoped review and governance requests", async () => {
	const requests: Array<{ method: string; path: string; authorization?: string; body?: unknown }> = [];
	await withServer(async (request, response) => {
		const body = request.method === "POST" ? JSON.parse(await readRequest(request)) : undefined;
		requests.push({
			method: request.method ?? "",
			path: request.url ?? "",
			...(request.headers.authorization ? { authorization: request.headers.authorization } : {}),
			...(body === undefined ? {} : { body }),
		});
		if (request.url?.startsWith("/v1/memories/candidates?")) {
			writeJSON(response, {
				resolution: { status: "resolved", continuity_id: "continuity-a", channel: "openclaw", thread_id: "agent:main:a", created: false },
				candidates: [{
					candidate_memory_id: "11111111-1111-1111-1111-111111111111",
					memory_key: "submission.bundle.current",
					content: "The bundle is thesis-defense-v7.zip.",
					source_quote: "The bundle is thesis-defense-v7.zip.",
					source_observation_id: "22222222-2222-2222-2222-222222222222",
					source_kind: "user_message",
					decision: "new",
					created_at: "2026-07-18T00:00:00Z",
				}],
			});
			return;
		}
		if (request.url?.startsWith("/v1/conversations/inspect?")) {
			writeJSON(response, {
				resolution: { status: "resolved", continuity_id: "continuity-a", channel: "openclaw", thread_id: "agent:main:a", created: false },
				observations: [{ id: "private-observation", content: "must not be returned by the client" }],
				memories: [{
					id: "33333333-3333-3333-3333-333333333333",
					memory_key: "submission.deadline.current",
					lifecycle_status: "active",
					content: "The deadline is Wednesday at 12:00.",
					effective_state: "eligible",
				}, {
					id: "66666666-6666-6666-6666-666666666666",
					lifecycle_status: "active",
					content: "A directly confirmed unkeyed memory.",
					effective_state: "eligible",
				}],
			});
			return;
		}
		writeJSON(response, {
			observation: { observation_id: "44444444-4444-4444-4444-444444444444", replayed: false },
			memory: {
				memory_id: request.url?.endsWith("/correct")
					? "55555555-5555-5555-5555-555555555555"
					: String((body as Record<string, unknown>).candidate_memory_id ?? (body as Record<string, unknown>).memory_id),
				status: request.url?.endsWith("/reject") ? "rejected" : request.url?.endsWith("/forget") ? "deleted" : "active",
				replayed: false,
			},
		});
	}, async (baseUrl) => {
		const client = new VermoryClient({ baseUrl, timeoutMs: 1000, apiToken: TEST_API_TOKEN });
		const inbox = await client.listCandidates("agent:main:a");
		expect(inbox.candidates).toHaveLength(1);
		expect(inbox.candidates[0]?.sourceQuote).toBe("The bundle is thesis-defense-v7.zip.");
		const memories = await client.listCurrentMemories("agent:main:a");
		expect(memories).toEqual([{
			memoryId: "33333333-3333-3333-3333-333333333333",
			memoryKey: "submission.deadline.current",
			content: "The deadline is Wednesday at 12:00.",
		}, {
			memoryId: "66666666-6666-6666-6666-666666666666",
			memoryKey: "memory",
			content: "A directly confirmed unkeyed memory.",
		}]);
		await client.acceptCandidate("agent:main:a", "11111111-1111-1111-1111-111111111111", "accept-op");
		await client.rejectCandidate("agent:main:a", "11111111-1111-1111-1111-111111111111", "reject-op");
		const corrected = await client.correctMemory("agent:main:a", "33333333-3333-3333-3333-333333333333", "The deadline is Friday.", "correct-op");
		expect(corrected.memoryId).toBe("55555555-5555-5555-5555-555555555555");
		await client.forgetMemory("agent:main:a", "33333333-3333-3333-3333-333333333333", "forget-op");
	});

	expect(requests).toHaveLength(6);
	for (const request of requests) {
		expect(request.authorization).toBe(`Bearer ${TEST_API_TOKEN}`);
	}
	expect(requests[0]).toMatchObject({ method: "GET", path: "/v1/memories/candidates?channel=openclaw&thread_id=agent%3Amain%3Aa" });
	expect(requests[1]).toMatchObject({ method: "GET", path: "/v1/conversations/inspect?channel=openclaw&thread_id=agent%3Amain%3Aa" });
	expect(requests[2]?.body).toEqual({ operation_id: "accept-op", channel: "openclaw", thread_id: "agent:main:a", candidate_memory_id: "11111111-1111-1111-1111-111111111111" });
	expect(requests[3]?.body).toEqual({ operation_id: "reject-op", channel: "openclaw", thread_id: "agent:main:a", candidate_memory_id: "11111111-1111-1111-1111-111111111111" });
	expect(requests[4]?.body).toEqual({ operation_id: "correct-op", channel: "openclaw", thread_id: "agent:main:a", memory_id: "33333333-3333-3333-3333-333333333333", content: "The deadline is Friday." });
	expect(requests[5]?.body).toEqual({ operation_id: "forget-op", channel: "openclaw", thread_id: "agent:main:a", memory_id: "33333333-3333-3333-3333-333333333333" });
  });

  it("aborts requests at the configured timeout", async () => {
    await withServer((_request, response) => {
      setTimeout(() => writeJSON(response, prepareReceipt("openclaw:slow", "late")), 500);
    }, async (baseUrl) => {
      const client = new VermoryClient({ baseUrl, timeoutMs: 250 });
      await expect(
        client.prepare({
          operationId: "openclaw:slow",
          sessionKey: "agent:main:a",
          message: "secret prompt",
        }),
      ).rejects.toThrow("Vermory prepare failed");
    });
  });

  it("does not expose request or response bodies in HTTP errors", async () => {
    await withServer((_request, response) => {
      response.writeHead(500, { "content-type": "application/json" });
      response.end('{"error":"secret prompt and database-id-123"}');
    }, async (baseUrl) => {
      const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
      let message = "";
      try {
        await client.prepare({
          operationId: "openclaw:http-error",
          sessionKey: "agent:main:a",
          message: "secret prompt",
        });
      } catch (error) {
        message = error instanceof Error ? error.message : String(error);
      }

      expect(message).toContain("HTTP 500");
      expect(message).not.toContain("secret prompt");
      expect(message).not.toContain("database-id-123");
    });
  });

  it("rejects an oversized response before JSON parsing", async () => {
    await withServer((_request, response) => {
      writeJSON(response, {
        ...prepareReceipt("openclaw:oversized", "ok"),
        padding: "x".repeat(300 * 1024),
      });
    }, async (baseUrl) => {
      const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
      await expect(
        client.prepare({
          operationId: "openclaw:oversized",
          sessionKey: "agent:main:a",
          message: "hello",
        }),
      ).rejects.toThrow("Vermory prepare response is too large");
    });
  });

  it("rejects invalid JSON and malformed receipts without leaking content", async () => {
    const bodies = [
      "not-json-secret",
      JSON.stringify({ ...prepareReceipt("openclaw:wrong", "context"), turn_id: "" }),
    ];

    for (const body of bodies) {
      await withServer((_request, response) => {
        response.writeHead(200, { "content-type": "application/json" });
        response.end(body);
      }, async (baseUrl) => {
        const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
        let message = "";
        try {
          await client.prepare({
            operationId: "openclaw:wrong",
            sessionKey: "agent:main:a",
            message: "hello",
          });
        } catch (error) {
          message = error instanceof Error ? error.message : String(error);
        }
        expect(message).toContain("Vermory prepare");
        expect(message).not.toContain("not-json-secret");
        expect(message).not.toContain("context");
      });
    }
  });

  it("rejects a receipt for another operation or lifecycle phase", async () => {
    for (const receipt of [
      prepareReceipt("openclaw:other", "context"),
      { ...prepareReceipt("openclaw:expected", "context"), status: "completed" },
    ]) {
      await withServer((_request, response) => writeJSON(response, receipt), async (baseUrl) => {
        const client = new VermoryClient({ baseUrl, timeoutMs: 1000 });
        await expect(
          client.prepare({
            operationId: "openclaw:expected",
            sessionKey: "agent:main:a",
            message: "hello",
          }),
        ).rejects.toThrow("Vermory prepare returned an invalid receipt");
      });
    }
  });
});

async function withServer(
  handler: Parameters<typeof createServer>[0],
  run: (baseUrl: string) => Promise<void>,
): Promise<void> {
  const server = createServer(handler);
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address() as AddressInfo;
  try {
    await run(`http://127.0.0.1:${address.port}`);
  } finally {
    await new Promise<void>((resolve, reject) => {
      server.close((error) => (error ? reject(error) : resolve()));
    });
  }
}

async function readRequest(request: AsyncIterable<Uint8Array>): Promise<string> {
  const chunks: Uint8Array[] = [];
  for await (const chunk of request) {
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

function writeJSON(response: { writeHead: Function; end: Function }, value: unknown): void {
  response.writeHead(200, { "content-type": "application/json" });
  response.end(JSON.stringify(value));
}

function prepareReceipt(operationId: string, context: string) {
  return {
    turn_id: "turn-1",
    operation_id: operationId,
    status: "in_progress",
    continuity_id: "continuity-1",
    delivery_id: "delivery-1",
    user_observation_id: "observation-user-1",
    replayed: false,
    context,
  };
}

function leasedReceipt(operationId: string, context: string) {
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
    answer: "The appointment is Saturday at 10:00.",
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
