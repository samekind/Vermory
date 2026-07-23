export interface PluginConfig {
  enabled: boolean;
  baseUrl: string;
  timeoutMs: number;
	toolAllowlist: string[];
}

const DEFAULT_CONFIG: PluginConfig = {
  enabled: true,
  baseUrl: "http://127.0.0.1:8787",
  timeoutMs: 5000,
	toolAllowlist: [],
};

const CONFIG_FIELDS = new Set(["enabled", "baseUrl", "timeoutMs", "toolAllowlist"]);
const TOOL_NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/;

export function normalizePluginConfig(input: unknown): PluginConfig {
  if (input === undefined) {
    return { ...DEFAULT_CONFIG };
  }
  if (typeof input !== "object" || input === null || Array.isArray(input)) {
    throw new Error("Vermory plugin config must be an object");
  }

  const values = input as Record<string, unknown>;
  for (const field of Object.keys(values)) {
    if (!CONFIG_FIELDS.has(field)) {
      throw new Error(`Unknown Vermory plugin config field: ${field}`);
    }
  }

  const enabled = values.enabled ?? DEFAULT_CONFIG.enabled;
  if (typeof enabled !== "boolean") {
    throw new Error("Vermory enabled must be a boolean");
  }

  const timeoutMs = values.timeoutMs ?? DEFAULT_CONFIG.timeoutMs;
  if (
    typeof timeoutMs !== "number" ||
    !Number.isInteger(timeoutMs) ||
    timeoutMs < 250 ||
    timeoutMs > 30_000
  ) {
    throw new Error("Vermory timeoutMs must be an integer from 250 to 30000");
  }

  const configuredBaseUrl = values.baseUrl ?? DEFAULT_CONFIG.baseUrl;
  if (typeof configuredBaseUrl !== "string" || configuredBaseUrl.trim() === "") {
    throw new Error("Vermory baseUrl must be a non-empty string");
  }

  let url: URL;
  try {
    url = new URL(configuredBaseUrl.trim());
  } catch {
    throw new Error("Vermory baseUrl must be a valid URL");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("Vermory baseUrl must use HTTP or HTTPS");
  }
  if (url.username !== "" || url.password !== "") {
    throw new Error("Vermory baseUrl must not contain credentials");
  }

	const rawToolAllowlist = values.toolAllowlist ?? DEFAULT_CONFIG.toolAllowlist;
	if (!Array.isArray(rawToolAllowlist) || rawToolAllowlist.length > 64) {
		throw new Error("Vermory toolAllowlist must be an array of at most 64 tool names");
	}
	const toolAllowlist: string[] = [];
	const seenTools = new Set<string>();
	for (const raw of rawToolAllowlist) {
		if (typeof raw !== "string") {
			throw new Error("Vermory toolAllowlist entries must be strings");
		}
		const tool = raw.trim();
		if (!TOOL_NAME_PATTERN.test(tool)) {
			throw new Error("Vermory toolAllowlist contains an invalid tool name");
		}
		if (seenTools.has(tool)) {
			throw new Error("Vermory toolAllowlist contains a duplicate tool name");
		}
		seenTools.add(tool);
		toolAllowlist.push(tool);
	}

  return {
    enabled,
    baseUrl: configuredBaseUrl.trim().replace(/\/+$/, ""),
    timeoutMs,
	toolAllowlist,
  };
}
