# Production Retrieval Runtime Evidence

Replay completed: 2026-07-15 Asia/Shanghai

Qualification: `production_path_integrated`

Default product mode: `lexical`

This evidence connects the W08 retrieval finding to the real workspace MCP and
conversation Web Chat runtimes. It proves one production-shaped, active-only
`BAAI/bge-m3` path with durable projection events, a restricted worker,
auditable lexical/shadow/vector modes, and exact lexical degradation. It does
not select a default ranking strategy or qualify scale.

## Runtime

| Component | Value |
|---|---|
| Vermory revision | `bde15c29f8f044addc6624adc403ecd1f19ba468` |
| Binary SHA-256 | `e4ab396e4002760c3ec191e13bd83d783924027a2d52210557959e3f7d679f64` |
| Go | `go1.26.5 darwin/arm64` |
| PostgreSQL | `18.4` |
| pgvector | `0.8.5` |
| schema | `14` |
| Grok CLI | `0.2.101 (5bc4b5dfadcf)` |
| Grok model | `grok-4.5` |
| retrieval profile | `siliconflow-bge-m3-1024-v1` |
| embedding route | direct `https://api.siliconflow.cn/v1` |
| embedding model | `BAAI/bge-m3`, 1024 dimensions |
| frozen case SHA-256 | `84950c650a2e20f70cce98d33561f899d9cd38d5890fd0190fff3a9a0d9ffd6e` |

The isolated binary, database, and non-owner runtime role were created only for
this replay. The role was neither superuser nor `BYPASSRLS` and owned zero
served tables. The SiliconFlow key was injected into transient process
environments and was absent from files, arguments, logs, database rows, dump,
and committed artifacts.

## Durable Projection

The W09 software-release and database-migration trajectory was formed through
public runtime/operator APIs. The first direct SiliconFlow worker run processed
15 target-tenant events to cursor 16 with zero lag and ten active-only vector
rows. The distractor tenant was processed separately and had one vector row.

After the later correction scenario, the target tenant ended with:

```text
active memories:       11
proposed memories:      2
superseded memories:    2
deleted memories:       1
eligible active facts: 11
vector rows:           11
cursor/latest event:   20/20
```

The second proposed memory is the real Grok task writeback. Proposed,
superseded, deleted, cross-workspace, and cross-tenant content never entered a
delivered vector result.

## Real Grok Workspace Loop

`grok mcp doctor` completed protocol `2025-06-18` initialization and discovered
exactly `prepare_context` and `commit_observation`. Real Grok session
`019f6196-c04a-7e70-a5d9-a7a4217a3d6a` then executed:

```text
prepare_context (w09-grok-vector, explicit vector mode)
-> consume one Chinese semantic fact and four exact technical facts
-> create and verify grok-release-check.md
-> commit_observation (w09-grok-writeback)
-> retain the agent result as proposed
```

PostgreSQL, rather than model self-report, recorded:

| Record | Value |
|---|---|
| delivery | `5c016f19-0d10-4b5e-ab06-4487187c4566` |
| retrieval | `vector -> vector`, six delivered IDs, no degradation |
| observation | `6796e252-4153-4a49-a45e-4da15941743c` |
| writeback memory | `7a4b2e64-dd33-4e57-a7f9-294315c67d1d` |
| writeback lifecycle | `proposed` |
| artifact SHA-256 | `b0bb9ca96782b92f1458595a801f4fd47309dc8ead58340d90d81ec3f6dd9bc7` |
| client JSON SHA-256 | `7bd538b9788d799d9213b9c634b66bba5cf9a824e365c2b2805b46ade58f8ce9` |

Deterministic artifact checks required the current two-maintainer rollback rule,
`deploy/prod/release.yaml`, `--canary-percent=10`, `REL-SIG-409`, and
`deepseek-ai/DeepSeek-V4-Flash`. They rejected the superseded one-maintainer
rule, proposed `--force` bypass, deleted legacy token, other-workspace rule, and
other-tenant rule.

## Real Conversation And Shadow Paths

The first real Web Chat attempt occurred immediately after the Grok proposed
writeback advanced the projection event stream. Its audit correctly recorded
`vector -> lexical / projection_lag`; the lexical paraphrase did not recover the
linked fact, and Grok returned the wrong answer. This failure is retained.

After the worker processed the one pending absent-state event, a fresh real
`grok-4.5` Web Chat turn over the linked conversation scope returned:

```text
time: Friday 22:30
prerequisite: complete the read-only check first
```

The persisted audit was `vector -> vector`, contained both authorized
conversation continuity IDs, delivered the primary and linked active memories,
and excluded the unlinked next-month migration fact.

Shadow parity used two databases cloned from the same pre-turn snapshot and two
real loopback OpenClaw prepare routes. The lexical and shadow delivery bodies
both had SHA-256
`a1acb83a30b20609cf73d70fdcdbac775ae4640c076516f28815056b8046b77b`.
The shadow audit recorded one lexical ID, two vector IDs, and delivered exactly
the lexical ID set. The temporary clone databases were removed after the
comparison.

## Degradation And Rebuild

An operator correction replaced a `21:30` release-freeze fact with `21:45`
while the worker was stopped. A real Grok client queried both lexical and vector
MCP endpoints. Their persisted delivery bodies were byte-identical with
SHA-256 `80334f77ee257fa5a7754e1a101fa292aefe07b55e2030d547cdb73c8479ab82`;
the vector audit recorded `vector -> lexical / projection_lag`.

The worker caught up two correction events. A local HTTP 503 proxy then rejected
the direct SiliconFlow TLS `CONNECT` without changing the frozen provider URL.
The real Grok outage and lexical deliveries again had the same SHA-256, and the
audit recorded `vector -> lexical / embedding_unavailable`.

Finally, `retrieval-rebuild` deleted all eleven target-tenant vectors and reset
the cursor. The restricted worker replayed the target tenant's event stream
through real SiliconFlow. Before and after rebuild:

```text
delivered result IDs: identical, including order (7)
delivery SHA-256:     76898dc86a0cb8125ad2a03ee152944fcc8834cf9367b7d1aa9bcba4a90ec04f
authority SHA-256:    7548e4705968e976619570c0fc944975409d83a67b3e4e4c920e58ca2a39881b
```

## RLS And Recovery

Restricted-role filter-omission results for events, cursors, vectors, and
retrieval audits were:

| Tenant context | Events | Cursors | Vectors | Audits |
|---|---:|---:|---:|---:|
| absent | 0 | 0 | 0 | 0 |
| target | 19 | 1 | 11 | 7 |
| distractor | 1 | 1 | 1 | 0 |

A restricted-role cross-tenant retrieval insert was rejected by the
tenant-bearing foreign key and left zero rows.

The PostgreSQL custom-format dump was 190,635 bytes with SHA-256
`2915ff9b8430870641c2b2e4d4a037ecdb225e1f80328637c9e4ca34c032f42f`.
The restored schema remained version 14. Source and restored counts were both
20 events, two cursors, 12 vectors, and seven audits. The complete authority
fingerprint was
`16cc09f9158d2b8dd0b8d05705ed74d753dc6f7ad82f2845510d542c9e6ce7ca`.

The restored target vectors were then deleted and rebuilt through the
restricted role plus real SiliconFlow. A real Grok MCP query returned the same
seven IDs in the same order and the same delivery hash as the source database.
The pre-query authority fingerprint remained unchanged by restore-side rebuild.

## Preserved Failures

1. The initial seed readiness check treated an HTTP 503 during startup as ready;
   the corrected probe waits for the listener without creating authority data.
2. The first MCP doctor failed because the project folder was not trusted. After
   explicit trust, the same server completed the handshake and exposed two tools.
3. The successful Grok workspace command's outer zsh wrapper attempted to assign
   the reserved variable `status` and returned exit 1 after Grok had completed.
   `EndTurn`, artifact, delivery, audit, observation, and writeback were verified
   independently; the model task was not rerun to erase this orchestration error.
4. The first vector conversation turn degraded after the proposed writeback
   advanced the cursor and therefore failed the semantic question. Catch-up plus
   a new operation produced the successful vector result.
5. A local embedding base URL was rejected by the frozen direct-SiliconFlow
   profile. The outage test retained that contract and used a process-scoped 503
   proxy instead.
6. The first credential scan did not enable shell fail-fast and printed PASS
   after two known password-shaped documentation examples. The strict rerun
   required zero new matches and allowlisted exactly those two existing examples.

## Credential And Claim Boundary

Strict scanning found zero exact provider-key matches in tracked files, the
worktree, and the temporary runtime directory; zero retained Authorization
headers; and zero new password-bearing database URLs. Two pre-existing
password-shaped examples remain in CI and the identity/RLS guide.

This W09 result changes H-009 only to `production_path_integrated` while its
status remains `testing`. It does not claim:

- a second independent retrieval batch or threshold decision;
- an accepted retrieval default or an RRF benefit;
- source-authority or conflict ranking;
- embedding generation migration or rollback;
- million-record, HA, PITR, or long-running fault qualification;
- withheld or externally sealed evaluation;
- signed release artifacts or final release acceptance.

The normalized machine-readable record is
[`snapshots/2026-07-14-production-retrieval-runtime.json`](snapshots/2026-07-14-production-retrieval-runtime.json).
