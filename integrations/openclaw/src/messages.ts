export type VisibleAssistantOutput = {
  text: string;
  model?: string;
};

export function extractLatestAssistantOutput(
  messages: unknown[],
): VisibleAssistantOutput | undefined {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (!isRecord(message) || message.role !== "assistant") {
      continue;
    }

    const text = extractVisibleContent(message.content);
    if (text) {
      const model = extractModelLabel(message);
      return {
        text,
        ...(model ? { model } : {}),
      };
    }
  }

  return undefined;
}

export function extractLatestAssistantText(messages: unknown[]): string | undefined {
  return extractLatestAssistantOutput(messages)?.text;
}

function extractVisibleContent(content: unknown): string | undefined {
  if (typeof content === "string") {
    return content.trim() || undefined;
  }
  if (!Array.isArray(content)) {
    return undefined;
  }

  const text = content
    .filter(
      (block): block is Record<string, unknown> =>
        isRecord(block) && block.type === "text" && typeof block.text === "string",
    )
    .map((block) => block.text)
    .join("")
    .trim();

  return text || undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function extractModelLabel(message: Record<string, unknown>): string | undefined {
  const provider = typeof message.provider === "string" ? message.provider.trim() : "";
  const model = typeof message.model === "string" ? message.model.trim() : "";
  if (provider && model) {
    return `${provider}/${model}`;
  }
  return model || provider || undefined;
}
