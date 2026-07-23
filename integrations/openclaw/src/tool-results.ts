const MAX_TOOL_RESULT_BYTES = 8 * 1024;
const MAX_TEXT_BLOCKS = 64;

export function extractToolResultText(result: unknown): string | undefined {
  let text: string | undefined;
  if (typeof result === "string") {
    text = result;
  } else if (Array.isArray(result)) {
    text = extractTextBlocks(result);
  } else if (isRecord(result) && Array.isArray(result.content)) {
    text = extractTextBlocks(result.content);
  }
  if (text === undefined) {
    return undefined;
  }
  const normalized = text.trim();
  if (
    normalized === "" ||
    !hasValidUnicode(normalized) ||
    new TextEncoder().encode(normalized).byteLength > MAX_TOOL_RESULT_BYTES ||
    containsSensitiveToolResult(normalized)
  ) {
    return undefined;
  }
  return normalized;
}

function extractTextBlocks(value: unknown[]): string | undefined {
  if (value.length === 0 || value.length > MAX_TEXT_BLOCKS) {
    return undefined;
  }
  const parts: string[] = [];
  for (const block of value) {
    if (!isRecord(block) || block.type !== "text" || typeof block.text !== "string") {
      return undefined;
    }
    const text = block.text.trim();
    if (text !== "") {
      parts.push(text);
    }
  }
  return parts.length === 0 ? undefined : parts.join("\n");
}

function containsSensitiveToolResult(value: string): boolean {
  return (
    /\bsk-[A-Za-z0-9_-]{16,}\b/.test(value) ||
    /\b(?:api[_-]?token|api[_-]?key|secret|token)\s*[:=]\s*[^\s,;]{8,}/i.test(value) ||
    /-----BEGIN(?: [A-Z0-9]+)* PRIVATE KEY-----/.test(value)
  );
}

function hasValidUnicode(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const current = value.charCodeAt(index);
    if (current >= 0xd800 && current <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      if (next < 0xdc00 || next > 0xdfff) {
        return false;
      }
      index += 1;
      continue;
    }
    if (current >= 0xdc00 && current <= 0xdfff) {
      return false;
    }
  }
  return true;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

