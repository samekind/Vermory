# Memory Eligibility And Retention Qualification

W19 qualifies whether Vermory can preserve durable history while ensuring that
AI clients receive only the facts that are currently eligible for the exact
tenant, continuity, lifecycle, and database-clock boundary.

## Accepted Run

| Field | Value |
|---|---|
| Run ID | `w19-formal-2e97280-20260717-v1` |
| Profile | `memory-eligibility-retention-v1` |
| Implementation revision | `2e9728043cc2c4f39e296fd2612c23299c5a541f` |
| Case SHA-256 | `fbe689a3877bda4b1085a22755f26fa615af38daaa3f56fc9965c36ab34371f2` |
| PostgreSQL / pgvector | `18.3 / 0.8.5` |
| Schema | `18` |
| Report JSON SHA-256 | `a61ddbcd2f5c49a7843ad206b694b38b13027739574e68abc54a487e45b47e0d` |
| Report Markdown SHA-256 | `c5a2d0f6798470488b1c6e15338a6efc331cd6f4ea29947c8d2a2459a9d43005` |
| Hard gates | `16 / 16 PASS` |

The normalized JSON report is committed at
[`snapshots/2026-07-16-memory-eligibility-retention.json`](snapshots/2026-07-16-memory-eligibility-retention.json).
Raw client transcripts, authenticated state, database files, provider bodies,
and vectors remain outside Git.

## Implemented Behavior

Vermory evaluates current reuse independently from durable location and
lifecycle history:

- working input remains request-local unless a separate governance action forms
  a durable memory;
- workspace and conversation memories remain durable inside their resolved
  continuity;
- Global Defaults remain a thin, strongly governed cross-continuity layer;
- `valid_from` and `valid_until` determine half-open current-use eligibility;
- expiry preserves inspectable history but removes current reuse;
- archive preserves inspectable history while removing lexical, vector, bridge,
  Web Chat, OpenClaw, and MCP eligibility;
- forget redacts the target and prevents exact, paraphrased, historical,
  projection, restore, and client delivery residue;
- every request uses one PostgreSQL-derived eligibility timestamp across all
  context sources.

Expiry is not deletion. Archive is not deletion. Neither operation is reported
as proof that content has been forgotten.

## Deterministic Database And Retrieval Evidence

The formal profile created four tenants, twenty continuities, and exactly
10,000 governed memories:

| Effective or lifecycle state | Count |
|---|---:|
| current, open-ended | 4,000 |
| scheduled | 1,500 |
| expired | 1,500 |
| archived | 1,000 |
| superseded | 1,000 |
| deleted | 1,000 |

Sixteen concurrent clients each executed twenty scoped queries. All `320 / 320`
completed. P50/P95/P99 latency was `23 / 40 / 53 ms`. Current-fact recall was
`4,000 / 4,000`, while scheduled premature use, expired misuse, archived misuse,
deletion residue, Global Default pollution, and cross-scope leakage were all
zero.

The profile retained `7,000` eligible lexical rows and `7,000` vector rows.
Projection rebuild and PostgreSQL restore produced the same projection
fingerprint. Immediate restart and restore produced the same authority
fingerprint, and forgotten content remained absent. A forced embedding outage
degraded to eligible lexical serving with zero stale, archived, deleted, or
cross-scope results.

Validity, archive, and forget operations produced two idempotent replays, two
conflict rejections, and two races in which forget won. False receipts and audit
content residue were both zero.

## Baseline Outcomes

The four frozen product tasks separate availability of context from eligibility
governance:

| Condition | Task success | Current hits | Invalid reuse | Context tokens |
|---|---:|---:|---:|---:|
| `no_context` | `0 / 4` | 0 | 0 | 0 |
| `full_history` | `1 / 4` | 4 | 6 | 160 |
| `lifecycle_only` | `2 / 4` | 4 | 3 | 96 |
| `vermory_eligibility` | `4 / 4` | 4 | 0 | 48 |

`Invalid reuse` is the sum of premature scheduled use, expired misuse,
archived misuse, deletion residue, and Global Default pollution. These are
deterministic case outcomes, not an LLM-judge score or a universal token-saving
claim.

## Real Client Evidence

Three accepted client trajectories are bound into the formal report:

| Surface | Client / model | Evidence SHA-256 | Artifact SHA-256 |
|---|---|---|---|
| Web Chat | Grok CLI `0.2.101` / `grok-4.5` | `9d2d8fbf3dfccd12bc643bcd104b762b651e7c34de2ba825c17d766df1e4e7df` | `58daaf2625869eab44508fecfcfccc9b51a06fd0a3eb95d4bc25bd22252fe350` |
| workspace MCP | Grok CLI `0.2.101` / `grok-4.5` | `cb5041363fd58c4cdeb606e5cdc454e2a3b85ab52b1c358846c52afed3f9d886` | `ab97a28882afbcbb7bc7b7e78ec1f50b46494fe23c654ea15878cd1f44417a95` |
| workspace MCP | official Codex CLI `0.144.3` / `gpt-5.5` | `1570aa87f321e0d9f4785cc14abab0bfa684af2b490d36f59d90335dbce88cfc` | `067031a5ae9c73b86a3f409236a851d1abeaa5a240b3a8b1045bdcf3c936cc1f` |

The clients consumed eligible memory, completed bounded tasks, and wrote results
back as proposed observations. No client output was automatically promoted to
current memory or Global Defaults.

## Direct Provider Proof

The formal run connected directly to the SiliconFlow OpenAI-compatible endpoint
without Mac mini NewAPI:

| Field | Value |
|---|---|
| Endpoint | `https://api.siliconflow.cn/v1` |
| Model | `BAAI/bge-m3` |
| Dimensions | `1,024` |
| Requests | `2` |
| Duration | `764 ms` |

The report contains only response hashes. It contains no credential, raw vector,
request body, or provider response body. This proves the named embedding route
for the bounded projection/query probe; it does not rank models.

## Preserved Failures

The report retains fifteen chronological failures rather than replacing them
with the final successful path:

```text
duplicate_flag
invalid_timestamp
stale_sibling_history
unknown_command
usage_limit
remote_command_quoting
idle_ssh_closed
helper_failure
postgres_version_mismatch
public_acl_probe
overstrict_empty_result
overstrict_topk_shape
module_proxy_timeout
invalid_preflight_credential
provider_unavailable
```

The first full 10,000-memory preflight deliberately used an invalid credential
and reached the provider gate before rejection. The accepted run used a fresh
run ID and did not reuse that failed artifact.

## Replay And Integrity

With the provider credential unset and both PostgreSQL root and binary paths
pointing to nonexistent locations, the completed formal report replayed in
`0.406 s` before database or network startup. JSON and Markdown hashes remained
unchanged. Independent scans found no credential pattern, DSN, private path, raw
vector, embedding payload, or provider response body in the normalized report.
The plan's intentionally broad scan also returned two reviewed documentation
identifiers, `Task-aware` and `--embedding-api-key-env`; neither line contains a
credential value. A value-bearing credential and private-path scan returned zero
matches.

## H-007 Decision

W19 supports the retention concern but rejects a single mandatory
`retention_class` enum. The accepted model is orthogonal:

1. request-local working input versus workspace, conversation, or Global
   Defaults location;
2. lifecycle state for proposal, current use, supersession, archive, rejection,
   and deletion;
3. temporal validity for scheduled and expired current reuse;
4. source authority, confirmation, privacy, and deployment policy.

This representation covered the frozen positive and negative cases without
creating a separate retention-class column. H-007 remains reopenable if a real
cross-deployment, privacy, or legal-retention rule cannot be represented by
these dimensions.

## Non-Claims

W19 is not a universal capacity claim, a model ranking, automatic memory
formation quality, a generic compliance policy, cross-host HA evidence, or a
sealed external evaluation. It qualifies the named memory-eligibility profile
and the three bound real-client trajectories only.
