# Long-Running Client Operation Qualification

Date: 2026-07-23

Reality case: `W36-long-running-client-operation`

Status: `runtime-qualified`

Evidence level: `public`

The normalized runtime report is retained on the external evidence disk at:

`/Volumes/JSData/.cache/vermory/w36/runs/w36-exact-bc0eb0e9/report.json`

The report binds the run to frozen case SHA
`553126cd1af677018c68d79d13145203c4a64c3b329f3fbdecef32197b3efe21`, exact
implementation revision `bc0eb0e90bba0d9f0013f38cd12f788d979ad1cd`,
authoritative PostgreSQL counts, native dump SHA-256, and all 28 hard gates.
The exact-commit run qualifies the W36 runtime contract. H-017 remains
`testing` until the same pushed revision completes the protected PR checks.

## Qualified Contract

W36 qualifies the lifecycle required by a long-running coding or tool-using
client:

```text
OpenClaw before_prompt_build
-> PostgreSQL-authoritative leased operation
-> successful allowlisted tool result
-> monotonic checkpoint
-> client, Vermory, and PostgreSQL restart
-> repeated prepare restores the same attempt and delivery
-> more tool work and checkpoint
-> completion creates one assistant observation and one formation request
```

It also qualifies explicit expired-lease reclaim, generation fencing,
cancel-versus-complete serialization, tenant isolation, continuity isolation,
bounded-endpoint protection, and failure/cancellation without semantic
write-back. The stored object remains the existing `conversation_turn`.
W36 does not add a generic workflow scheduler and does not treat checkpoint
data as memory, truth, tool evidence, or model context.

## Runtime Boundary

| Field | Accepted value |
|---|---|
| PostgreSQL | 18.4 Homebrew binaries, fresh disposable cluster |
| Cluster and evidence | external disk under `/Volumes/JSData/.cache/vermory` |
| Candidate service | real `vermory serve` process |
| Client surface | compiled Vermory OpenClaw plugin hooks and client module |
| OpenClaw hooks | `before_prompt_build`, `after_tool_call`, `agent_end` |
| Hermes surface | authenticated bounded `prepare`, `complete`, and `fail` |
| Provider | deterministic mock; no model-quality claim is made |
| Authentication | real digest-backed bearer tokens and restricted runtime role |
| Privilege | no `sudo`, no NewAPI, no `~/.codex` state |
| Restarts | 2 Vermory/service and 2 PostgreSQL |
| Native recovery | custom-format `pg_dump` and empty-database `pg_restore` |
| Restore check | original bearer token authenticated against restored data |

The temporary cluster, database, role, socket, service, and work root were
removed after the run. The dump and normalized report remain on the external
evidence disk. Raw credentials, sensitive checkpoint content, and private
OpenClaw parameters were not persisted.

## Hard Gates

All 28 frozen W36 gates passed. The normalized report contains the exact frozen
case text; the checks covered:

1. New operation authority: one user observation, delivery, generation-one
   attempt, and lease.
2. Exact prepare replay: turn, delivery, attempt, generation, context, and
   checkpoint were unchanged.
3. Ordinary prepare did not reclaim a live lease.
4. Explicit reclaim rejected a live lease.
5. Expired reclaim incremented generation and issued a different attempt.
6. Reclaim preserved user observation, delivery, context, and checkpoint.
7. Current heartbeat renewed the lease without semantic side effects.
8. A higher checkpoint sequence replaced the latest checkpoint.
9. Checkpoint replay was idempotent; drift and regression were rejected.
10. Checkpoint content did not enter observations, formation, memories,
    lexical/vector projections, deliveries, or model context.
11. Sensitive and oversized checkpoints were rejected before persistence.
12. Old heartbeat after reclaim was rejected.
13. Old checkpoint after reclaim was rejected.
14. Old tool result after reclaim was rejected.
15. Old completion, failure, and cancellation after reclaim were rejected.
16. The current attempt resumed after Vermory process restart.
17. PostgreSQL restart preserved in-progress and terminal operation state.
18. Completion alone created one assistant observation and one formation request.
19. Failure created no semantic write-back.
20. Cancellation created no semantic write-back.
21. Late completion after cancellation was rejected.
22. Concurrent cancel and complete produced one terminal winner.
23. Exact terminal replay was idempotent and changed payloads were rejected.
24. Another tenant could not observe or mutate the original operation.
25. Another continuity could not resume or mutate it.
26. A leased operation could not bypass fencing through the bounded endpoint.
27. The actual OpenClaw plugin rebuilt attempt metadata after a fresh plugin
   process and used fenced tool results plus checkpoints.
28. Hermes bounded prepare/complete/fail compatibility remained intact.

## Authoritative Result

The restored database and service reported:

```text
leased turns              6
completed turns           3
failed turns              3
cancelled turns           2
assistant observations    3
tool-result observations  3
formation schedules       3
race assistant effects    0
race formation effects    0
```

The native dump was `174318` bytes with SHA-256
`98e4a0a8fb19bdcb52d3e4480a03ecd8125f4e9f75c9899a8d2c5b780c620321`.
Completed, cancelled, and failed terminal receipts retained their original
identities after restore.

The cancel/complete race was won by cancellation in this exact-commit run.
That is the second accepted terminal outcome of the frozen race contract; it
correctly produced zero assistant and formation effects for the raced turn.

## Verification Evidence

The repository protection chain passed with the following lanes:

```text
go test -p 1 -count=1 ./...
go test -race -p 1 -count=1 <CI runtime package set>
go test -count=1 -race ./internal/reality
go vet ./...
scripts/repository-policy.sh
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
Hermes unittest discovery with external UV_PROJECT_ENVIRONMENT
```

The W36 focused suite and reset test passed against a fresh external PostgreSQL
18 cluster. OpenClaw reported `76/76`; Hermes reported `6/6`.

## Retained Failures

Earlier worktree runs remain under the external evidence root:

| Run | Result | Cause |
|---|---|---|
| `w36-worktree-b8fc0b380f597c7e` | rejected | Unix socket path exceeded the macOS 103-byte limit |
| `w36-worktree-b8fc0b380f597c7e-attempt2` | rejected | Readiness probe used local `jstar`, not `postgres` |
| `w36-worktree-b8fc0b380f597c7e-attempt3` | rejected | Full operation ID was used as OpenClaw `run_id` |
| `w36-worktree-b8fc0b380f597c7e-attempt4` | passed pre-dump | Established the 28-gate trajectory before dump/restore was added |
| `w36-worktree-dumprestore-attempt5` | accepted worktree precursor | Complete W36 run including native dump/restore |
| `w36-exact-bc0eb0e9` | accepted exact commit | All 28 gates, native dump/restore, and residue checks passed on `bc0eb0e90bba0d9f0013f38cd12f788d979ad1cd` |

Failed run directories contain sanitized logs only; their temporary clusters
and credentials were removed.

## Claim Boundary

This evidence qualifies the PostgreSQL-authoritative leased lifecycle for the
maintained OpenClaw plugin and the Hermes bounded compatibility path. It does
not qualify model quality, provider ranking, embedding quality, Hermes
long-running recovery, a generic workflow scheduler, offline multi-device
conflict resolution, distributed ownership, arbitrary checkpoint history,
every OpenClaw channel, or a production SLO/HA topology.
