# Grok Web Chat Runtime Evidence

Date: 2026-07-13

Runtime: Vermory local Web Chat API

Provider: authenticated Grok CLI `0.2.99`

Model: `grok-4.5`

Database: PostgreSQL authority under an isolated replay tenant

## Result

| Gate | Result |
|---|---|
| Real HTTP turn | Completed with `grok-4.5` |
| Explicit confirmation | Assistant observation became `active` memory |
| Context consumption | Follow-up returned the correct marker length, `13`, without repeating the value |
| Duplicate operation | Returned identical turn and assistant observation IDs with `replayed=true` |
| Server restart | Same thread resumed and returned the retained prefix `VMRY-` |
| Targeted forget | Memory receipt returned `status=deleted` |
| Inspection | Content-bearing observation and governed memory returned `[redacted]` |
| Exact post-delete probe | `UNAVAILABLE` |
| Paraphrased post-delete probe | `UNAVAILABLE` |
| Original operation replay after delete | Same turn returned `answer=[redacted]` |

The full generated marker was removed from retained artifacts after deletion. It is not present in this evidence document or tracked repository files.

## Failure Preserved During Replay

The first real attempts produced a durable `provider_error`. Database diagnostics showed that Grok returned valid JSON with:

```json
{
  "text": "",
  "stopReason": "Cancelled"
}
```

The same model and prompt completed when launched through the terminal. Controlled probes isolated the difference to the Grok CLI process boundary: direct Go child execution was cancelled, while a shell that remained the direct parent and opened the CLI output file completed normally.

The provider adapter now:

1. passes every argument as a positional shell argument rather than interpolating prompt content into shell source;
2. keeps `/bin/sh` as the Grok process parent;
3. lets the shell open the Grok JSON output file;
4. reads and validates that file after process completion;
5. retains the existing isolated one-turn, no-memory, no-web-search, no-subagent flags.

Provider regression tests cover both the shell-parent requirement and regular-file output capture.

## Deterministic Evidence

The real replay is complemented by automated hard gates:

- C01 reopens PostgreSQL before the final continuation and requires the verified device-maintenance state.
- S01 forgets a synthetic target, rebuilds projections, reopens PostgreSQL, and checks observations, governed memory, projection rows, delivery history, turn answers, exact probes, paraphrased probes, and inspection.

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...
```

Local redacted request/response artifacts are retained under the ignored path:

```text
artifacts/runtime/C04-grok-webchat-runtime/
```

The directory remains ignored by repository policy; this document is the tracked evidence summary.
