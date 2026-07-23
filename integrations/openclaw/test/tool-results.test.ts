import { describe, expect, it } from "vitest";

import { extractToolResultText } from "../src/tool-results.js";

describe("extractToolResultText", () => {
  it.each([
    ["plain string", "Storage has 87 GB available."],
    ["content envelope", { content: [{ type: "text", text: "Removed successfully." }] }],
    ["direct blocks", [{ type: "text", text: "Keyboard count: 1,333,470" }]],
  ])("extracts bounded documented %s results", (_name, result) => {
    expect(extractToolResultText(result)).toBe(
      typeof result === "string"
        ? result
        : Array.isArray(result)
          ? "Keyboard count: 1,333,470"
          : "Removed successfully.",
    );
  });

  it.each([
    ["arbitrary object", { output: "must not be flattened", secret: "hidden" }],
    ["non-text block", { content: [{ type: "image", data: "hidden" }] }],
    ["mixed invalid block", { content: [{ type: "text", text: "ok" }, { type: "text", value: "bad" }] }],
    ["sensitive assignment", "api_token=W23_SYNTHETIC_SECRET_MUST_NOT_PERSIST"],
    ["private key", "-----BEGIN PRIVATE KEY-----"],
    ["oversized", "x".repeat(8193)],
    ["blank", "   "],
  ])("rejects %s results", (_name, result) => {
    expect(extractToolResultText(result)).toBeUndefined();
  });
});

