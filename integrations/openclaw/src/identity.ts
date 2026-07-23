export interface TurnIdentity {
  sessionKey: string;
  operationId: string;
}

export function resolveTurnIdentity(context: {
  sessionKey?: string;
  runId?: string;
  [key: string]: unknown;
}): TurnIdentity | undefined {
  const sessionKey = context.sessionKey?.trim();
  const runId = context.runId?.trim();
  if (!sessionKey || !runId) {
    return undefined;
  }

  return {
    sessionKey,
    operationId: `openclaw:${runId}`,
  };
}
