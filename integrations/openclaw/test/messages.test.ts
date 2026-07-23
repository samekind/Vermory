import { describe, expect, it } from "vitest";

import { extractLatestAssistantText } from "../src/messages.js";

describe("extractLatestAssistantText", () => {
  it("extracts trimmed string content", () => {
    expect(
      extractLatestAssistantText([
        { role: "user", content: "When is the appointment?" },
        { role: "assistant", content: "  Saturday at 10:00.  " },
      ]),
    ).toBe("Saturday at 10:00.");
  });

  it("joins visible text blocks", () => {
    expect(
      extractLatestAssistantText([
        {
          role: "assistant",
          content: [
            { type: "text", text: "The concierge will " },
            { type: "thinking", thinking: "Do not expose this." },
            { type: "text", text: "check them in." },
          ],
        },
      ]),
    ).toBe("The concierge will check them in.");
  });

  it("returns the latest visible assistant iteration", () => {
    expect(
      extractLatestAssistantText([
        { role: "assistant", content: "The appointment is Friday." },
        { role: "tool", content: [{ type: "tool_result", text: "updated" }] },
        { role: "assistant", content: [{ type: "text", text: "It is Saturday at 10:00." }] },
      ]),
    ).toBe("It is Saturday at 10:00.");
  });

  it("skips tool-only assistant tails", () => {
    expect(
      extractLatestAssistantText([
        { role: "assistant", content: "I will check the current appointment." },
        { role: "assistant", content: [{ type: "tool_call", name: "calendar" }] },
      ]),
    ).toBe("I will check the current appointment.");
  });

  it("does not expose reasoning-only assistant content", () => {
    expect(
      extractLatestAssistantText([
        {
          role: "assistant",
          content: [
            { type: "reasoning", text: "private reasoning" },
            { type: "thought", text: "private thought" },
          ],
        },
      ]),
    ).toBeUndefined();
  });

  it("ignores malformed and non-assistant messages", () => {
    expect(
      extractLatestAssistantText([
        null,
        "assistant-like string",
        { role: "assistant" },
        { role: "assistant", content: 42 },
        { role: "user", content: "not an answer" },
      ]),
    ).toBeUndefined();
  });
});
