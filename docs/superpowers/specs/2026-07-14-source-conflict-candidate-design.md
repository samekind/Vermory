# Source Conflict Candidate Design

Date: 2026-07-14

Status: frozen for implementation

## Goal

Vermory must notice a changed source fact without silently replacing current
memory. A source ingestor may provide a stable fact key, an exact source
revision, and new semantic content. Vermory resolves that key only inside the
current tenant and workspace continuity, records a proposed candidate, and
keeps the current active fact in model context until a trusted operator accepts
the candidate.

This slice adds deterministic keyed formation. It is the safe foundation for a
later provider-assisted unkeyed matcher; it does not claim general semantic
conflict detection.

## Frozen Case W05

The `release-control` workspace contains:

| Fact | Stable key | Source | State |
|---|---|---|---|
| Production releases use a macOS keychain certificate. | `release.signing.mode` | `repo:deploy/production.yaml@sha-old` | active |
| The deployment API timeout is 800 ms. | `deploy.api.timeout` | `repo:deploy/runtime.yaml@sha-stable` | active |
| Another tenant uses static cloud credentials. | `release.signing.mode` | other tenant | active distractor |

The recognized source changes `release.signing.mode` to:

```text
Production releases use GitHub Actions OIDC keyless signing.
```

with source reference `repo:deploy/production.yaml@sha-new`.

## User-Visible Flow

```text
source import with stable fact key
-> Vermory proposes a new or replacement candidate
-> current AI context remains unchanged
-> operator inspects candidate and target
-> operator accepts or rejects
-> accepted content becomes current; rejected content remains audit history
```

The normal AI client never receives candidate IDs, lifecycle metadata, or
review instructions. Operator CLI and diagnostics may expose them.

## Data Contract

No second memory lifecycle is introduced. Existing tables remain authoritative:

- `observations` records source candidate submissions and operator decisions;
- `governed_memories` stores candidate semantic content;
- `memory_key` is the stable fact identity supplied by a trusted ingestor;
- `supersedes_memory_id` links a replacement candidate to its current target;
- lifecycle adds `rejected` beside `proposed`, `active`, `superseded`, and
  `deleted`;
- `memory_search_documents` still contains active memory only.

Candidate source content and exact `source_ref` are retained in the observation
and governed memory lineage. PostgreSQL remains authoritative; no provider or
projection can directly activate a candidate.

## Formation Rules

Target resolution is constrained to one tenant, one confirmed workspace
continuity, and one non-empty stable memory key.

| Active keyed source matches | Result |
|---:|---|
| 0 | proposed new source candidate with no supersession target |
| 1, different content | proposed replacement linked to that active memory |
| 1, identical normalized content | durable unchanged observation; no candidate |
| more than 1 | abstain with an ambiguity error; no mutation |

Deleted, superseded, rejected, proposed, cross-tenant, and cross-continuity
memories are never eligible targets. Source reference similarity alone is not a
target key.

## Decision Rules

Accepting a candidate must, in one transaction:

1. lock the proposed candidate;
2. record a `user_confirmation` observation;
3. if a target exists, require it to still be active and supersede exactly it;
4. activate the candidate;
5. remove the target projection and add the candidate projection.

Rejecting a candidate must record a `candidate_rejection` observation and move
only that proposed candidate to `rejected`. The target remains active.

Repeated use of the same operation ID and same logical request returns the
original result. Reusing it for different content, key, continuity, candidate,
or decision fails.

## Hard Gates

- A proposed or rejected candidate is absent from normal retrieval and MCP
  context.
- Proposal alone never changes the active target or its projection.
- Acceptance supersedes exactly one same-scope active target.
- Rejection changes no active memory.
- Ambiguous key resolution abstains and writes no candidate.
- Cross-tenant and cross-continuity target access is rejected by application
  checks, tenant foreign keys, and RLS.
- Projection rebuild includes accepted content and excludes proposed, rejected,
  superseded, and deleted content.
- Proposal, acceptance, and rejection replay safely; conflicting replay fails.
- Provider outage is irrelevant to this deterministic path and cannot lose
  source input after the caller receives success.

## Real Runtime Proof

After deterministic acceptance, a real logged-in Grok CLI task must consume the
workspace through Vermory MCP, produce an artifact containing OIDC keyless
signing plus the unchanged 800 ms timeout, exclude the old keychain instruction
and other-tenant distractor, and write its result back as `proposed`.

## Non-Claims

This slice does not prove provider-assisted unkeyed matching, automatic claim
extraction from arbitrary documents, conversation candidate formation,
multi-source truth ranking, formation quality at scale, or final release
readiness. Those remain separate measured slices.
