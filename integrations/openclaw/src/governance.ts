import { randomUUID } from "node:crypto";

import type { CurrentMemory, ReviewCandidate } from "./client.js";
import { VermoryClient } from "./client.js";

type CommandContext = {
  args?: string;
  isAuthorizedSender: boolean;
  sessionKey?: string;
};

type ReferenceEntry = {
  id: string;
  kind: "candidate" | "memory";
  key: string;
  content: string;
  decision?: "new" | "update";
  sourceQuote?: string;
	sourceKind?: "user_message" | "tool_result";
	sourceLabel?: string;
  ref: string;
};

const MAX_ENTRIES_PER_KIND = 20;
const MAX_SESSIONS = 256;
const MAX_REPLY_CHARS = 8_000;

export class VermoryGovernanceCommand {
  private readonly client: VermoryClient | undefined;
  private readonly sessions = new Map<string, ReferenceEntry[]>();

  constructor(client: VermoryClient | undefined) {
    this.client = client;
  }

  async handle(context: CommandContext): Promise<{ text: string; continueAgent: false }> {
    if (!context.isAuthorizedSender) {
      return reply("Vermory governance is not authorized for this sender.");
    }
    if (!this.client) {
      return reply("Vermory operator token is not configured.");
    }
    const sessionKey = context.sessionKey?.trim();
    if (!sessionKey) {
      return reply("Vermory could not identify the current OpenClaw session.");
    }
    const args = context.args?.trim() ?? "";
    try {
      if (args === "" || args === "help") {
        return reply(commandHelp());
      }
      if (args === "memories") {
        return reply(await this.list(sessionKey));
      }
      const [action] = args.split(/\s+/, 1);
      switch (action) {
        case "accept":
          return reply(await this.acceptOrReject(sessionKey, args, true));
        case "reject":
          return reply(await this.acceptOrReject(sessionKey, args, false));
        case "correct":
          return reply(await this.correct(sessionKey, args));
        case "forget":
          return reply(await this.forget(sessionKey, args));
        default:
          return reply(commandHelp());
      }
    } catch {
      return reply("Vermory command failed without changing the current OpenClaw turn. Try again later.");
    }
  }

  private async list(sessionKey: string): Promise<string> {
    const [inbox, memories] = await Promise.all([
      this.client!.listCandidates(sessionKey),
      this.client!.listCurrentMemories(sessionKey),
    ]);
    const pending = inbox.candidates.slice(0, MAX_ENTRIES_PER_KIND);
    const active = memories.slice(0, MAX_ENTRIES_PER_KIND);
    const entries = assignReferences([
      ...pending.map(candidateEntry),
      ...active.map(memoryEntry),
    ]);
    this.remember(sessionKey, entries);

    const lines: string[] = [];
    const pendingEntries = entries.filter((entry) => entry.kind === "candidate");
    const memoryEntries = entries.filter((entry) => entry.kind === "memory");
    if (pendingEntries.length > 0) {
      lines.push("Pending candidates:");
      for (const entry of pendingEntries) {
		const source = entry.sourceKind === "tool_result"
			? `tool ${entry.sourceLabel ?? "unknown"}`
			: "user message";
		lines.push(`[${entry.ref}] ${entry.decision} ${entry.key}: ${oneLine(entry.content)} | source: ${source} | quote: ${oneLine(entry.sourceQuote ?? "")}`);
      }
      if (inbox.candidates.length > pendingEntries.length) {
        lines.push(`... ${inbox.candidates.length - pendingEntries.length} more pending candidates not shown.`);
      }
    }
    if (memoryEntries.length > 0) {
      if (lines.length > 0) {
        lines.push("");
      }
      lines.push("Current memories:");
      for (const entry of memoryEntries) {
        lines.push(`[${entry.ref}] active ${entry.key}: ${oneLine(entry.content)}`);
      }
      if (memories.length > memoryEntries.length) {
        lines.push(`... ${memories.length - memoryEntries.length} more active memories not shown.`);
      }
    }
    if (lines.length === 0) {
      lines.push("No pending candidates or active memories for this OpenClaw session.");
    }
    return bounded(lines.join("\n"));
  }

  private async acceptOrReject(sessionKey: string, args: string, accept: boolean): Promise<string> {
    const match = /^(?:accept|reject)\s+(\S+)\s*$/.exec(args);
    if (!match?.[1]) {
      return `Usage: /vermory ${accept ? "accept" : "reject"} <candidate-ref>`;
    }
    const resolved = this.resolve(sessionKey, match[1], "candidate");
    if (typeof resolved === "string") {
      return resolved;
    }
    const receipt = accept
      ? await this.client!.acceptCandidate(sessionKey, resolved.id, operationID())
      : await this.client!.rejectCandidate(sessionKey, resolved.id, operationID());
    const entries = this.sessions.get(sessionKey) ?? [];
    if (accept) {
      resolved.kind = "memory";
	  delete resolved.decision;
	  delete resolved.sourceQuote;
      resolved.id = receipt.memoryId;
      this.sessions.set(sessionKey, assignReferences(entries));
      return `Accepted [${resolved.ref}] ${resolved.key}.`;
    }
    this.sessions.set(sessionKey, assignReferences(entries.filter((entry) => entry !== resolved)));
    return `Rejected [${resolved.ref}] ${resolved.key}.`;
  }

  private async correct(sessionKey: string, args: string): Promise<string> {
    const match = /^correct\s+(\S+)\s+([\s\S]+)$/.exec(args);
    if (!match?.[1] || !match[2]?.trim()) {
      return "Usage: /vermory correct <memory-ref> <replacement>";
    }
    const resolved = this.resolve(sessionKey, match[1], "memory");
    if (typeof resolved === "string") {
      return resolved;
    }
    const replacement = match[2].trim();
    const receipt = await this.client!.correctMemory(sessionKey, resolved.id, replacement, operationID());
    resolved.id = receipt.memoryId;
    resolved.content = replacement;
    this.sessions.set(sessionKey, assignReferences(this.sessions.get(sessionKey) ?? []));
    return `Corrected [${resolved.ref}] ${resolved.key}.`;
  }

  private async forget(sessionKey: string, args: string): Promise<string> {
    const match = /^forget\s+(\S+)\s*$/.exec(args);
    if (!match?.[1]) {
      return "Usage: /vermory forget <memory-ref>";
    }
    const resolved = this.resolve(sessionKey, match[1], "memory");
    if (typeof resolved === "string") {
      return resolved;
    }
    await this.client!.forgetMemory(sessionKey, resolved.id, operationID());
    const entries = this.sessions.get(sessionKey) ?? [];
    this.sessions.set(sessionKey, assignReferences(entries.filter((entry) => entry !== resolved)));
    return `Forgotten [${resolved.ref}] ${resolved.key}.`;
  }

  private resolve(sessionKey: string, rawReference: string, kind: ReferenceEntry["kind"]): ReferenceEntry | string {
    const entries = this.sessions.get(sessionKey);
    if (!entries) {
      return "Run /vermory memories in this session before using a memory reference.";
    }
    const reference = compactID(rawReference);
    if (reference.length < 4) {
      return "The memory reference is too short.";
    }
    const matches = entries.filter((entry) => entry.kind === kind && compactID(entry.id).startsWith(reference));
    if (matches.length === 0) {
      return `No current ${kind} matches that reference. Run /vermory memories again.`;
    }
    if (matches.length > 1) {
      return "That memory reference is ambiguous. Use the longer reference shown by /vermory memories.";
    }
    return matches[0]!;
  }

  private remember(sessionKey: string, entries: ReferenceEntry[]): void {
    this.sessions.delete(sessionKey);
    this.sessions.set(sessionKey, entries);
    while (this.sessions.size > MAX_SESSIONS) {
      const oldest = this.sessions.keys().next().value as string | undefined;
      if (!oldest) {
        break;
      }
      this.sessions.delete(oldest);
    }
  }
}

function candidateEntry(candidate: ReviewCandidate): Omit<ReferenceEntry, "ref"> {
  return {
    id: candidate.candidateMemoryId,
    kind: "candidate",
    key: candidate.memoryKey,
    content: candidate.content,
    decision: candidate.decision,
    sourceQuote: candidate.sourceQuote,
	sourceKind: candidate.sourceKind,
	...(candidate.sourceLabel ? { sourceLabel: candidate.sourceLabel } : {}),
  };
}

function memoryEntry(memory: CurrentMemory): Omit<ReferenceEntry, "ref"> {
  return {
    id: memory.memoryId,
    kind: "memory",
    key: memory.memoryKey,
    content: memory.content,
  };
}

function assignReferences(entries: Array<Omit<ReferenceEntry, "ref"> | ReferenceEntry>): ReferenceEntry[] {
  return entries.map((entry) => {
    const compact = compactID(entry.id);
    let length = Math.min(8, compact.length);
    while (length < compact.length && entries.some((other) => other !== entry && compactID(other.id).startsWith(compact.slice(0, length)))) {
      length += 1;
    }
    return { ...entry, ref: compact.slice(0, length) };
  });
}

function compactID(value: string): string {
  return value.trim().toLowerCase().replaceAll("-", "");
}

function oneLine(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized.length <= 180 ? normalized : `${normalized.slice(0, 177)}...`;
}

function bounded(value: string): string {
  return value.length <= MAX_REPLY_CHARS ? value : `${value.slice(0, MAX_REPLY_CHARS - 3)}...`;
}

function operationID(): string {
  return `openclaw-command:${randomUUID()}`;
}

function commandHelp(): string {
  return [
    "/vermory memories",
    "/vermory accept <candidate-ref>",
    "/vermory reject <candidate-ref>",
    "/vermory correct <memory-ref> <replacement>",
    "/vermory forget <memory-ref>",
  ].join("\n");
}

function reply(text: string): { text: string; continueAgent: false } {
  return { text: bounded(text), continueAgent: false };
}
