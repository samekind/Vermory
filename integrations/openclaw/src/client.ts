const MAX_RESPONSE_BYTES = 256 * 1024;

type TurnStatus = "in_progress" | "completed" | "failed" | "cancelled";

export interface ClientConfig {
  baseUrl: string;
  timeoutMs: number;
  apiToken?: string | undefined;
}

export interface TurnIdentityInput {
  operationId: string;
  sessionKey: string;
}

export interface PreparedTurnRequest extends TurnIdentityInput {
  message: string;
}

export interface CompleteTurnRequest extends TurnIdentityInput {
  answer: string;
  model: string;
}

export interface FailTurnRequest extends TurnIdentityInput {
  failureCode: string;
  failureMessage: string;
}

export interface ToolResultRequest extends TurnIdentityInput {
	runId: string;
	toolName: string;
	toolCallId: string;
	content: string;
	attemptId?: string;
	leaseGeneration?: number;
}

export interface LeasedTurnIdentityInput extends TurnIdentityInput {
	attemptId: string;
	leaseGeneration: number;
}

export interface CheckpointOperationRequest extends LeasedTurnIdentityInput {
	sequence: number;
	checkpoint: Record<string, unknown>;
}

export interface CompleteLeasedOperationRequest extends LeasedTurnIdentityInput {
	answer: string;
	model: string;
}

export interface FailLeasedOperationRequest extends LeasedTurnIdentityInput {
	failureCode: string;
	failureMessage: string;
}

export interface ToolResultReceipt {
	turnId: string;
	observationId: string;
	toolName: string;
	replayed: boolean;
}

export interface TurnReceipt {
  turnId: string;
  operationId: string;
  status: TurnStatus;
  continuityId: string;
  deliveryId: string;
  userObservationId: string;
  assistantObservationId?: string;
  answer?: string;
  model?: string;
  failureCode?: string;
  replayed: boolean;
}

export interface PreparedTurnReceipt extends TurnReceipt {
  context?: string;
}

export interface LeasedOperationReceipt extends TurnReceipt {
	protocol: "leased_v1";
	attemptId: string;
	leaseGeneration: number;
	leaseExpiresAt?: string;
	checkpointSequence: number;
	checkpoint: Record<string, unknown>;
	context?: string;
}

export interface ReviewCandidate {
  candidateMemoryId: string;
  memoryKey: string;
  content: string;
  sourceQuote: string;
  sourceObservationId: string;
	sourceKind: "user_message" | "tool_result";
	sourceLabel?: string;
  decision: "new" | "update";
  targetMemoryId?: string;
  createdAt: string;
}

export interface ReviewInbox {
  continuityId: string;
  candidates: ReviewCandidate[];
}

export interface CurrentMemory {
  memoryId: string;
  memoryKey: string;
  content: string;
}

export interface GovernanceReceipt {
  memoryId: string;
  status: "active" | "rejected" | "deleted";
  replayed: boolean;
}

export class VermoryClient {
  private readonly config: ClientConfig;

  constructor(config: ClientConfig) {
    this.config = { ...config, apiToken: normalizeApiToken(config.apiToken) };
  }

  async prepare(request: PreparedTurnRequest): Promise<PreparedTurnReceipt> {
    const body = await this.post("prepare", {
      operation_id: request.operationId,
      session_key: request.sessionKey,
      message: request.message,
    });
    return parseReceipt(body, "prepare", request.operationId, true);
  }

  async complete(request: CompleteTurnRequest): Promise<TurnReceipt> {
    const body = await this.post("complete", {
      operation_id: request.operationId,
      session_key: request.sessionKey,
      answer: request.answer,
      model: request.model,
    });
    return parseReceipt(body, "complete", request.operationId, false);
  }

  async fail(request: FailTurnRequest): Promise<TurnReceipt> {
    const body = await this.post("fail", {
      operation_id: request.operationId,
      session_key: request.sessionKey,
      failure_code: request.failureCode,
      failure_message: request.failureMessage,
    });
    return parseReceipt(body, "fail", request.operationId, false);
  }

	async prepareLeased(request: PreparedTurnRequest): Promise<LeasedOperationReceipt> {
		return this.prepareOrReclaimLeased("prepare", request);
	}

	async reclaimLeased(request: PreparedTurnRequest): Promise<LeasedOperationReceipt> {
		return this.prepareOrReclaimLeased("reclaim", request);
	}

	async heartbeatLeased(request: LeasedTurnIdentityInput): Promise<LeasedOperationReceipt> {
		const body = await this.clientOperationRequest("heartbeat", request, {});
		return parseLeasedOperationReceipt(body, "heartbeat", request.operationId, "in_progress");
	}

	async checkpointLeased(request: CheckpointOperationRequest): Promise<LeasedOperationReceipt> {
		if (!Number.isSafeInteger(request.sequence) || request.sequence <= 0) {
			throw new Error("Vermory checkpoint sequence is invalid");
		}
		const body = await this.clientOperationRequest("checkpoint", request, {
			sequence: request.sequence,
			checkpoint: request.checkpoint,
		});
		return parseLeasedOperationReceipt(body, "checkpoint", request.operationId, "in_progress");
	}

	async completeLeased(request: CompleteLeasedOperationRequest): Promise<LeasedOperationReceipt> {
		const body = await this.clientOperationRequest("complete", request, {
			answer: requireValue(request.answer, "answer"),
			model: requireValue(request.model, "model"),
		});
		return parseLeasedOperationReceipt(body, "complete", request.operationId, "completed");
	}

	async failLeased(request: FailLeasedOperationRequest): Promise<LeasedOperationReceipt> {
		const body = await this.clientOperationRequest("fail", request, {
			failure_code: requireValue(request.failureCode, "failure code"),
			failure_message: request.failureMessage.trim(),
		});
		return parseLeasedOperationReceipt(body, "fail", request.operationId, "failed");
	}

	async recordToolResult(request: ToolResultRequest): Promise<ToolResultReceipt> {
		const toolName = requireValue(request.toolName, "tool name");
		const body = await this.request("POST", "/v1/integrations/openclaw/turns/tool-results", {
			operation_id: requireValue(request.operationId, "operation ID"),
			session_key: requireValue(request.sessionKey, "session key"),
			run_id: requireValue(request.runId, "run ID"),
			tool_name: toolName,
			tool_call_id: requireValue(request.toolCallId, "tool call ID"),
			content: requireValue(request.content, "tool result content"),
			...(request.attemptId === undefined ? {} : { attempt_id: requireValue(request.attemptId, "attempt ID") }),
			...(request.leaseGeneration === undefined ? {} : { lease_generation: requireLeaseGeneration(request.leaseGeneration) }),
		}, "tool result");
		return parseToolResultReceipt(body, toolName);
	}

  async listCandidates(sessionKey: string): Promise<ReviewInbox> {
	const query = new URLSearchParams({ channel: "openclaw", thread_id: requireValue(sessionKey, "session key") });
	const body = await this.request("GET", `/v1/memories/candidates?${query.toString()}`, undefined, "candidate list");
	return parseReviewInbox(body);
  }

  async listCurrentMemories(sessionKey: string): Promise<CurrentMemory[]> {
	const query = new URLSearchParams({ channel: "openclaw", thread_id: requireValue(sessionKey, "session key") });
	const body = await this.request("GET", `/v1/conversations/inspect?${query.toString()}`, undefined, "memory list");
	return parseCurrentMemories(body);
  }

  async acceptCandidate(sessionKey: string, candidateMemoryId: string, operationId: string): Promise<GovernanceReceipt> {
	return this.reviewCandidate("accept", sessionKey, candidateMemoryId, operationId);
  }

  async rejectCandidate(sessionKey: string, candidateMemoryId: string, operationId: string): Promise<GovernanceReceipt> {
	return this.reviewCandidate("reject", sessionKey, candidateMemoryId, operationId);
  }

  async correctMemory(sessionKey: string, memoryId: string, content: string, operationId: string): Promise<GovernanceReceipt> {
	const body = await this.request("POST", "/v1/memories/correct", {
		operation_id: requireValue(operationId, "operation ID"),
		channel: "openclaw",
		thread_id: requireValue(sessionKey, "session key"),
		memory_id: requireValue(memoryId, "memory ID"),
		content: requireValue(content, "replacement content"),
	}, "memory correction");
	return parseGovernanceReceipt(body, "memory correction");
  }

  async forgetMemory(sessionKey: string, memoryId: string, operationId: string): Promise<GovernanceReceipt> {
	const body = await this.request("POST", "/v1/memories/forget", {
		operation_id: requireValue(operationId, "operation ID"),
		channel: "openclaw",
		thread_id: requireValue(sessionKey, "session key"),
		memory_id: requireValue(memoryId, "memory ID"),
	}, "memory deletion");
	return parseGovernanceReceipt(body, "memory deletion");
  }

  private async reviewCandidate(action: "accept" | "reject", sessionKey: string, candidateMemoryId: string, operationId: string): Promise<GovernanceReceipt> {
	const phase = `candidate ${action}`;
	const body = await this.request("POST", `/v1/memories/candidates/${action}`, {
		operation_id: requireValue(operationId, "operation ID"),
		channel: "openclaw",
		thread_id: requireValue(sessionKey, "session key"),
		candidate_memory_id: requireValue(candidateMemoryId, "candidate memory ID"),
	}, phase);
	return parseGovernanceReceipt(body, phase);
  }

  private async post(phase: "prepare" | "complete" | "fail", body: unknown): Promise<unknown> {
	return this.request("POST", `/v1/integrations/openclaw/turns/${phase}`, body, phase);
  }

	private async prepareOrReclaimLeased(phase: "prepare" | "reclaim", request: PreparedTurnRequest): Promise<LeasedOperationReceipt> {
		const operationId = requireValue(request.operationId, "operation ID");
		const body = await this.request("POST", `/v1/client-operations/${phase}`, {
			operation_id: operationId,
			channel: "openclaw",
			thread_id: requireValue(request.sessionKey, "session key"),
			message: requireValue(request.message, "message"),
		}, `leased ${phase}`);
		return parseLeasedOperationReceipt(body, phase, operationId, undefined, true);
	}

	private async clientOperationRequest(
		phase: "heartbeat" | "checkpoint" | "complete" | "fail",
		request: LeasedTurnIdentityInput,
		extra: Record<string, unknown>,
	): Promise<unknown> {
		return this.request("POST", `/v1/client-operations/${phase}`, {
			operation_id: requireValue(request.operationId, "operation ID"),
			channel: "openclaw",
			thread_id: requireValue(request.sessionKey, "session key"),
			attempt_id: requireValue(request.attemptId, "attempt ID"),
			lease_generation: requireLeaseGeneration(request.leaseGeneration),
			...extra,
		}, `leased ${phase}`);
	}

  private async request(method: "GET" | "POST", path: string, body: unknown | undefined, phase: string): Promise<unknown> {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.config.timeoutMs);
	const url = `${this.config.baseUrl}${path}`;

    try {
      let response: Response;
      try {
		const headers: Record<string, string> = {};
		if (body !== undefined) {
			headers["content-type"] = "application/json";
		}
        if (this.config.apiToken) {
          headers.authorization = `Bearer ${this.config.apiToken}`;
        }
        response = await fetch(url, {
		  method,
          headers,
		  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
          signal: controller.signal,
        });
      } catch (error) {
        if (controller.signal.aborted) {
          throw new Error(`Vermory ${phase} failed: timeout`);
        }
        throw new Error(`Vermory ${phase} failed: network error`);
      }

      if (!response.ok) {
        throw new Error(`Vermory ${phase} failed: HTTP ${response.status}`);
      }

      const text = await readBoundedResponse(response, phase);
      try {
        return JSON.parse(text) as unknown;
      } catch {
        throw new Error(`Vermory ${phase} returned invalid JSON`);
      }
    } catch (error) {
      if (error instanceof Error && error.message.startsWith("Vermory ")) {
        throw error;
      }
      throw new Error(`Vermory ${phase} failed`);
    } finally {
      clearTimeout(timeout);
    }
  }
}

function parseReviewInbox(value: unknown): ReviewInbox {
	if (!isRecord(value) || !isRecord(value.resolution) || typeof value.resolution.continuity_id !== "string" || !Array.isArray(value.candidates) || value.candidates.length > 100) {
		throw new Error("Vermory candidate list returned an invalid receipt");
	}
	const candidates = value.candidates.map((raw): ReviewCandidate => {
		if (!isRecord(raw) || !isUUID(raw.candidate_memory_id) || typeof raw.memory_key !== "string" || raw.memory_key === "" ||
			typeof raw.content !== "string" || raw.content === "" || typeof raw.source_quote !== "string" || raw.source_quote === "" ||
			!isUUID(raw.source_observation_id) || (raw.decision !== "new" && raw.decision !== "update") ||
			(raw.source_kind !== "user_message" && raw.source_kind !== "tool_result") ||
			typeof raw.created_at !== "string" || Number.isNaN(Date.parse(raw.created_at)) ||
			(raw.target_memory_id !== undefined && !isUUID(raw.target_memory_id)) ||
			(raw.source_label !== undefined && (typeof raw.source_label !== "string" || raw.source_label === ""))) {
			throw new Error("Vermory candidate list returned an invalid receipt");
		}
		return {
			candidateMemoryId: raw.candidate_memory_id,
			memoryKey: raw.memory_key,
			content: raw.content,
			sourceQuote: raw.source_quote,
			sourceObservationId: raw.source_observation_id,
			sourceKind: raw.source_kind,
			...(typeof raw.source_label === "string" ? { sourceLabel: raw.source_label } : {}),
			decision: raw.decision,
			...(typeof raw.target_memory_id === "string" ? { targetMemoryId: raw.target_memory_id } : {}),
			createdAt: raw.created_at,
		};
	});
	return { continuityId: value.resolution.continuity_id, candidates };
}

function parseToolResultReceipt(value: unknown, expectedToolName: string): ToolResultReceipt {
	if (!isRecord(value) || !isUUID(value.turn_id) || !isUUID(value.observation_id) ||
		value.tool_name !== expectedToolName || typeof value.replayed !== "boolean") {
		throw new Error("Vermory tool result returned an invalid receipt");
	}
	return {
		turnId: value.turn_id,
		observationId: value.observation_id,
		toolName: value.tool_name,
		replayed: value.replayed,
	};
}

function parseCurrentMemories(value: unknown): CurrentMemory[] {
	if (!isRecord(value) || !Array.isArray(value.memories) || value.memories.length > 100) {
		throw new Error("Vermory memory list returned an invalid receipt");
	}
	const memories: CurrentMemory[] = [];
	for (const raw of value.memories) {
		if (!isRecord(raw) || typeof raw.lifecycle_status !== "string") {
			throw new Error("Vermory memory list returned an invalid receipt");
		}
		if (raw.lifecycle_status !== "active") {
			continue;
		}
		if (!isUUID(raw.id) || (raw.memory_key !== undefined && typeof raw.memory_key !== "string") || typeof raw.content !== "string" || raw.content === "") {
			throw new Error("Vermory memory list returned an invalid receipt");
		}
		const memoryKey = typeof raw.memory_key === "string" && raw.memory_key.trim() !== "" ? raw.memory_key : "memory";
		memories.push({ memoryId: raw.id, memoryKey, content: raw.content });
	}
	return memories;
}

function parseGovernanceReceipt(value: unknown, phase: string): GovernanceReceipt {
	if (!isRecord(value) || !isRecord(value.memory) || !isUUID(value.memory.memory_id) ||
		(value.memory.status !== "active" && value.memory.status !== "rejected" && value.memory.status !== "deleted") ||
		typeof value.memory.replayed !== "boolean") {
		throw new Error(`Vermory ${phase} returned an invalid receipt`);
	}
	return { memoryId: value.memory.memory_id, status: value.memory.status, replayed: value.memory.replayed };
}

function requireValue(value: string, label: string): string {
	const normalized = value.trim();
	if (normalized === "") {
		throw new Error(`Vermory ${label} is required`);
	}
	return normalized;
}

function requireLeaseGeneration(value: number): number {
	if (!Number.isSafeInteger(value) || value <= 0) {
		throw new Error("Vermory lease generation is invalid");
	}
	return value;
}

function isUUID(value: unknown): value is string {
	return typeof value === "string" && /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(value);
}

export function normalizeApiToken(value: string | undefined): string | undefined {
  if (value === undefined) {
    return undefined;
  }
  const normalized = value.trim();
  if (normalized === "") {
    return undefined;
  }
  if (!/^vmt_[A-Za-z0-9-]{8,64}_[A-Za-z0-9_-]{32,64}$/.test(normalized)) {
    throw new Error("Vermory API token is invalid");
  }
  return normalized;
}

async function readBoundedResponse(response: Response, phase: string): Promise<string> {
  const contentLength = response.headers.get("content-length");
  if (contentLength !== null && Number(contentLength) > MAX_RESPONSE_BYTES) {
    throw new Error(`Vermory ${phase} response is too large`);
  }

  if (!response.body) {
    const text = await response.text();
    if (new TextEncoder().encode(text).byteLength > MAX_RESPONSE_BYTES) {
      throw new Error(`Vermory ${phase} response is too large`);
    }
    return text;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let total = 0;
  let text = "";
  try {
    while (true) {
      const result = await reader.read();
      if (result.done) {
        text += decoder.decode();
        return text;
      }
      total += result.value.byteLength;
      if (total > MAX_RESPONSE_BYTES) {
        await reader.cancel();
        throw new Error(`Vermory ${phase} response is too large`);
      }
      text += decoder.decode(result.value, { stream: true });
    }
  } finally {
    reader.releaseLock();
  }
}

function parseReceipt(
  value: unknown,
  phase: "prepare" | "complete" | "fail",
  operationId: string,
  includeContext: boolean,
): PreparedTurnReceipt | TurnReceipt {
  if (!isRecord(value)) {
    throw new Error(`Vermory ${phase} returned an invalid receipt`);
  }

  const expectedStatus: TurnStatus =
    phase === "prepare" ? "in_progress" : phase === "complete" ? "completed" : "failed";
  if (
    value.operation_id !== operationId ||
    value.status !== expectedStatus ||
    typeof value.turn_id !== "string" ||
    value.turn_id === "" ||
    typeof value.continuity_id !== "string" ||
    value.continuity_id === "" ||
    typeof value.delivery_id !== "string" ||
    value.delivery_id === "" ||
    typeof value.user_observation_id !== "string" ||
    value.user_observation_id === "" ||
    typeof value.replayed !== "boolean"
  ) {
    throw new Error(`Vermory ${phase} returned an invalid receipt`);
  }

  if (phase === "complete" &&
      (typeof value.assistant_observation_id !== "string" ||
        value.assistant_observation_id === "" ||
        typeof value.answer !== "string" ||
        typeof value.model !== "string")) {
    throw new Error(`Vermory ${phase} returned an invalid receipt`);
  }
  if (phase === "fail" &&
      (typeof value.failure_code !== "string" || value.failure_code === "")) {
    throw new Error(`Vermory ${phase} returned an invalid receipt`);
  }
  if (includeContext && value.context !== undefined && typeof value.context !== "string") {
    throw new Error(`Vermory ${phase} returned an invalid receipt`);
  }

  const receipt: TurnReceipt = {
    turnId: value.turn_id,
    operationId: value.operation_id,
    status: expectedStatus,
    continuityId: value.continuity_id,
    deliveryId: value.delivery_id,
    userObservationId: value.user_observation_id,
    replayed: value.replayed,
  };
  if (typeof value.assistant_observation_id === "string") {
    receipt.assistantObservationId = value.assistant_observation_id;
  }
  if (typeof value.answer === "string") {
    receipt.answer = value.answer;
  }
  if (typeof value.model === "string") {
    receipt.model = value.model;
  }
  if (typeof value.failure_code === "string") {
    receipt.failureCode = value.failure_code;
  }
  if (includeContext) {
    return {
      ...receipt,
      ...(typeof value.context === "string" ? { context: value.context } : {}),
    };
  }
  return receipt;
}

function parseLeasedOperationReceipt(
	value: unknown,
	phase: string,
	operationId: string,
	expectedStatus?: TurnStatus,
	includeContext = false,
): LeasedOperationReceipt {
	const actualStatus = isRecord(value) && isTurnStatus(value.status) ? value.status : undefined;
	if (!isRecord(value) || actualStatus === undefined || (expectedStatus !== undefined && actualStatus !== expectedStatus) ||
		value.operation_id !== operationId ||
		value.protocol !== "leased_v1" || typeof value.turn_id !== "string" || value.turn_id === "" ||
		typeof value.continuity_id !== "string" || value.continuity_id === "" ||
		typeof value.delivery_id !== "string" || value.delivery_id === "" ||
		typeof value.user_observation_id !== "string" || value.user_observation_id === "" ||
		!isUUID(value.attempt_id) || !Number.isSafeInteger(value.lease_generation) || Number(value.lease_generation) <= 0 ||
		typeof value.replayed !== "boolean" || !Number.isSafeInteger(value.checkpoint_sequence) || Number(value.checkpoint_sequence) < 0 ||
		(value.checkpoint !== undefined && !isRecord(value.checkpoint)) ||
		(includeContext && value.context !== undefined && typeof value.context !== "string") ||
		(actualStatus === "in_progress" && (typeof value.lease_expires_at !== "string" || Number.isNaN(Date.parse(value.lease_expires_at))))) {
		throw new Error(`Vermory leased ${phase} returned an invalid receipt`);
	}
	if (actualStatus === "completed" &&
		(typeof value.assistant_observation_id !== "string" || value.assistant_observation_id === "" ||
		 typeof value.answer !== "string" || typeof value.model !== "string")) {
		throw new Error(`Vermory leased ${phase} returned an invalid receipt`);
	}
	if (actualStatus === "failed" && (typeof value.failure_code !== "string" || value.failure_code === "")) {
		throw new Error(`Vermory leased ${phase} returned an invalid receipt`);
	}
	return {
		turnId: value.turn_id,
		operationId: value.operation_id,
		status: actualStatus,
		protocol: "leased_v1",
		continuityId: value.continuity_id,
		deliveryId: value.delivery_id,
		userObservationId: value.user_observation_id,
		attemptId: value.attempt_id,
		leaseGeneration: Number(value.lease_generation),
		...(typeof value.lease_expires_at === "string" ? { leaseExpiresAt: value.lease_expires_at } : {}),
		checkpointSequence: Number(value.checkpoint_sequence),
		checkpoint: isRecord(value.checkpoint) ? value.checkpoint : {},
		replayed: value.replayed,
		...(typeof value.context === "string" ? { context: value.context } : {}),
		...(typeof value.assistant_observation_id === "string" ? { assistantObservationId: value.assistant_observation_id } : {}),
		...(typeof value.answer === "string" ? { answer: value.answer } : {}),
		...(typeof value.model === "string" ? { model: value.model } : {}),
		...(typeof value.failure_code === "string" ? { failureCode: value.failure_code } : {}),
	};
}

function isTurnStatus(value: unknown): value is TurnStatus {
	return value === "in_progress" || value === "completed" || value === "failed" || value === "cancelled";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
