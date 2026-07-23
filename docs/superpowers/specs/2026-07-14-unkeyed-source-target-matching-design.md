# Provider-Assisted Unkeyed Source Target Matching Design

Date: 2026-07-14

Status: frozen for implementation

## Goal

Vermory must handle a trusted source revision that supplies an exact source
reference and exact new fact content but does not know Vermory's internal
`memory_key`. A configured provider may select exactly one key from the current
tenant and confirmed workspace's closed set of active keyed facts, or abstain.
Vermory validates and audits that decision before creating the same governed
source candidate introduced by W05.

This slice adds closed-set target matching. It does not claim arbitrary
document extraction, open-ended semantic conflict detection, source authority
ranking, or automatic activation of model output.

## Frozen Case W06

The `release-control-unkeyed` workspace contains three active keyed facts:

| Stable key | Current fact |
|---|---|
| `release.signing.mode` | Production releases use a macOS keychain certificate. |
| `deploy.api.timeout` | The deployment API timeout is 800 ms. |
| `release.attestation.format` | Production releases publish a signed SLSA provenance statement. |

The recognized source revision says:

```text
Production releases now use GitHub Actions OIDC keyless signing.
```

The source ingestor supplies no key. The provider must select
`release.signing.mode`; Vermory must reject any key outside the closed set and
must not let the provider activate or supersede memory directly.

An isolated other tenant contains `release.signing.mode` with static cloud
credentials. That fact must be absent from the provider candidate packet,
ordinary retrieval, and the final model task.

## User-Visible Flow

```text
trusted source revision without memory_key
-> Vermory snapshots active keyed facts in the confirmed workspace
-> provider selects one listed key or abstains
-> Vermory validates the closed-set decision and persists the evidence
-> a matched decision creates a proposed source candidate
-> normal AI context remains on the current active fact
-> operator accepts or rejects through the existing candidate lifecycle
```

The normal AI client never receives provider diagnostics, candidate-set JSON,
decision IDs, raw provider output, or candidate lifecycle instructions.

## Candidate Set

The candidate set contains only active governed memories in the current tenant
and continuity with a non-empty `memory_key`. Each entry contains the memory
ID, stable key, current content, and origin source reference. Entries are sorted
by key and memory ID before canonical JSON and SHA-256 fingerprints are formed.

Proposed, rejected, superseded, deleted, unkeyed, cross-tenant, and
cross-continuity memories are excluded. Duplicate active rows for one key are
retained in the audit snapshot but that key is not a valid unique target.

## Provider Contract

The provider receives source content as untrusted reference data plus the
closed candidate set. It may return exactly one JSON object:

```json
{"decision":"matched","memory_key":"release.signing.mode","reason":"The new source changes the production signing mechanism."}
```

or:

```json
{"decision":"abstained","memory_key":"","reason":"No single listed fact is a safe target."}
```

Unknown fields, trailing content, missing fields, unknown decisions, a matched
key outside the candidate set, or a non-empty abstained key are invalid. The
provider cannot create a key, choose another scope, set authority, activate a
memory, or write global defaults.

For Grok CLI, source and candidate content is written to a mode-`0600`
temporary prompt file rather than process arguments. The provider call has a
two-minute deadline. Cancellation kills the entire spawned process group, and
the prompt file is removed after completion.

## Durable Audit Contract

A new authoritative `source_match_decisions` table records:

- tenant, continuity, operation ID, and request fingerprint;
- exact source reference and source fact content;
- canonical candidate snapshot and fingerprint;
- provider name, requested model, resolved model, normalized output, artifact
  SHA-256, and bounded reason or failure message;
- lifecycle status `pending`, `matched`, `abstained`, or `failed`;
- selected key, target memory, observation, and candidate memory when present;
- created and completed timestamps.

The table is protected by tenant-aware foreign keys, PostgreSQL RLS, and the
restricted runtime role boundary. It is authoritative audit data and therefore
included in backup/restore fingerprints. It is never a search projection and
never contributes text to model-facing context.

Target memory, observation, and candidate memory references include tenant and
continuity in their foreign keys. Historical rows with invalid same-tenant
cross-continuity references are failed and redacted during migration rather
than deleted.

If a referenced memory is forgotten, deletion wins over diagnostic retention.
Vermory preserves decision IDs, status, selected-key metadata, and artifact
hashes, but redacts matching candidate content and source references, redacts
provider output and reason text, redacts source content when it represents the
deleted memory, and recomputes the candidate-set fingerprint.

## Transaction And Replay Rules

The initial request and candidate snapshot are inserted as `pending` before the
provider call. Reusing the same operation ID with the same logical request
returns the stored terminal decision without another provider call. Reusing it
with changed source content, source reference, provider, model, workspace, or
candidate snapshot fails.

A pending operation older than the provider deadline plus one minute becomes a
durable `pending_expired` failure on replay. A memory deletion affecting a
pending candidate set immediately ends that decision as
`referenced_memory_deleted`, so an in-flight provider cannot restore deleted
text when it eventually returns.

Provider failure, timeout, malformed output, invalid selection, empty candidate
set, or candidate-set drift becomes a durable `failed` or `abstained` terminal
decision. Retrying requires a new operation ID.

For a valid match, one PostgreSQL transaction must:

1. lock the pending match decision;
2. lock and rebuild the current active keyed candidate set;
3. require its fingerprint to equal the pre-provider snapshot;
4. require the selected key to resolve to exactly one snapshotted active fact;
5. create the source-candidate observation and proposed governed memory, or an
   unchanged observation when normalized content is identical;
6. link the decision to the target, observation, and candidate;
7. mark the decision `matched`.

No transaction step supersedes or activates the current target. Existing
operator accept/reject commands remain the only path to a terminal candidate
decision.

## Hard Gates

- Provider input is limited to one tenant and one confirmed workspace.
- Provider output can select only one exact key from the frozen closed set.
- Provider source/candidate text is absent from Grok process arguments, provider
  runtime is bounded, and cancellation terminates the process group.
- Ambiguous, unrelated, malformed, timed-out, or injected requests never create
  a candidate.
- A changed candidate set between provider call and proposal fails atomically.
- Match replay does not invoke the provider again or create another candidate.
- Conflicting operation replay fails.
- Changed candidate snapshots reject old operation replay; orphaned pending
  operations expire to a terminal audited failure.
- Proposed match results remain absent from normal retrieval and MCP context.
- Match audit rows are RLS protected, included in native backup/restore, and
  excluded from projection rebuild.
- Forget removes deleted fact text from source content, candidate snapshots,
  provider output, and reason fields without erasing structural audit IDs.
- The restricted runtime role can use the table but cannot bypass tenant RLS.
- The primary real runtime uses the locally authenticated Grok CLI, not a mock
  or Mac mini NewAPI route.

## Real Runtime Proof

A real Grok CLI call must select `release.signing.mode` from the W06 closed set.
Before acceptance, ordinary context must still expose the keychain fact and
exclude OIDC. After explicit operator acceptance and projection rebuild, a real
Grok task consuming Vermory through MCP must create
`release-control-policy.md` containing OIDC keyless signing, the 800 ms timeout,
and the SLSA provenance statement, while excluding the keychain and other
tenant's static credentials. The task result must write back as proposed.

Separate real Grok calls must abstain for one ambiguous source and one unrelated
source. Deterministic tests cover malformed output, invalid keys, timeouts,
prompt injection, replay conflict, candidate-set drift, RLS, restore, and
projection exclusion.

## Non-Claims

This slice does not prove automatic fact extraction from arbitrary documents,
provider-generated memory content, open-vocabulary target discovery,
conversation formation, multi-source authority ranking, hybrid retrieval,
formation quality at scale, sealed evaluation, or final release readiness.
