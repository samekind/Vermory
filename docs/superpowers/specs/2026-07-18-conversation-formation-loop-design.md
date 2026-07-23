# Conversation Formation Loop Design

Date: 2026-07-18

Status: frozen for implementation

## Goal

Vermory must turn durable facts stated by a user in a real conversation into
reviewable memory candidates without treating the whole transcript, assistant
answers, model inference, or temporary turn instructions as active memory.

This slice closes the conversation write-back loop:

```text
real client turn
-> persisted user observation
-> bounded same-continuity formation window
-> exact-evidence proposed candidates
-> explicit accept, reject, correct, or forget
-> governed memory delivered to a later real client turn
```

It extends the existing governed source-formation kernel. It does not create a
second conversation-memory store, does not make model output authoritative, and
does not allow ordinary conversation to create Global Defaults.

## Frozen Case F01

F01 is derived from the authorized and anonymized O01 home-maintenance
trajectory without modifying the frozen O01 fixtures.

The first conversation window contains these user observations:

1. Friday 15:30 plumbing inspection and concierge check-in.
2. Temporary access code `CEDAR-4826` with an explicit short lifetime.
3. Rain and lunch chatter.

The expected first formation batch is:

| Evidence | Decision | Stable key | Candidate effect |
|---|---|---|---|
| Friday 15:30 appointment sentence | `new` | `maintenance.appointment.current` | proposed |
| concierge sentence | `new` | `maintenance.concierge.check_in` | proposed |
| temporary code sentence | `new` | `maintenance.access.temporary_code` | proposed |

Rain and lunch chatter create no item. No candidate becomes active before an
explicit acceptance action.

After acceptance, a later user observation states that the building moved the
inspection to Saturday at 10:00 and that Friday is obsolete. The second
formation batch must contain one `update` for
`maintenance.appointment.current`. Accepting it supersedes the Friday fact.

A one-turn request to answer in English creates no memory candidate and cannot
create or mutate a Global Default. Forgetting the temporary code must remove it
from active retrieval, exact and paraphrased search, fresh delivery, inspection,
formation evidence, delivery history, and rebuildable projections.

## Input Contract

The trusted operator supplies:

- one confirmed conversation anchor;
- one operation ID;
- either an explicit ordered set of user-observation IDs or a bounded recent
  user-observation limit;
- one configured provider and model.

The selected observations must:

- belong to the same tenant and exact conversation continuity;
- have kind `user_message`;
- be non-redacted UTF-8 text;
- contain at most 50 observations and 65,536 content bytes in total;
- be ordered by authoritative observation sequence;
- contain no duplicate ID.

Assistant observations are not eligible formation evidence. A provider cannot
refer to an observation outside the persisted input manifest.

## Authoritative Run Contract

The existing `source_formation_runs` and `source_formation_items` remain the
authoritative formation audit.

Migration 19 adds:

- `source_formation_runs.input_kind`, with `document` and `conversation`;
- `source_formation_runs.input_manifest`, containing only ordered observation
  IDs, sequence numbers, content hashes, and byte lengths for conversation
  input;
- `source_formation_runs.input_manifest_fingerprint`;
- `source_formation_items.evidence_observation_id`.

Raw conversation text is not duplicated into the run row. It already exists as
an authoritative observation and is loaded only for provider input and exact
span validation. Provider output remains bounded audit data and is redacted when
referenced memory is forgotten.

Document formation remains compatible:

- `input_kind=document`;
- empty input manifest;
- `evidence_observation_id=NULL`.

Conversation formation requires every item to reference one observation in the
run manifest. The exact quote and occurrence are validated only inside that
observation, and byte offsets are relative to its content.

## Provider Contract

The provider receives:

- the ordered selected user observations with IDs and sequence numbers;
- the current same-continuity active keyed-memory snapshot;
- explicit instructions that all content is untrusted data.

Each provider item contains exactly:

- `decision`;
- `memory_key`;
- `source_observation_id`;
- `quote`;
- `occurrence`;
- `content`;
- `reason`.

The provider may return only `new`, `update`, or `unchanged`. Vermory verifies
the evidence reference, exact quote span, target cardinality, unchanged-content
identity, duplicate keys, overlapping spans within one observation, and active
snapshot stability before committing anything.

The provider may not activate memory, bridge continuity, choose another tenant,
use an assistant answer as evidence, or create a Global Default.

## Lifecycle Contract

`new` and `update` produce the same proposed source candidates used by document
formation. `unchanged` records audit only. Explicit conversation-scoped
acceptance and rejection call the existing candidate lifecycle store under the
resolved conversation continuity.

Accepted candidates become ordinary governed memory and are eligible for later
conversation delivery. Correction and forgetting use the existing conversation
governance path.

Forgetting a candidate formed from conversation evidence also redacts the
candidate's evidence observation and its paired conversation turn. This mirrors
the existing deletion behavior for directly confirmed conversation memory and
prevents the original turn from reviving the forgotten fact.

## Idempotency And Concurrency

- Replaying one operation with the same anchor, provider, model, and exact input
  manifest returns the stored run and does not invoke the provider again.
- Reusing an operation ID with another continuity or input manifest is rejected.
- If the active keyed-memory snapshot changes during provider execution, the run
  fails with `active_snapshot_changed` and creates no candidate.
- If any selected observation is redacted, removed, reclassified, reordered, or
  content-changed during provider execution, the run fails with
  `input_manifest_changed` and creates no candidate.
- New observations outside the bound manifest do not invalidate the run.
- Provider timeout, cancellation, malformed JSON, invalid evidence, and
  abstention are durable terminal outcomes and never create partial candidates.

## Isolation And Security Gates

The following are hard failures:

- a selected observation belongs to another tenant or continuity;
- a provider item references an observation outside the manifest;
- a conversation run is opened against a workspace continuity or vice versa;
- raw Session A history appears in Session B merely because governed memory is
  linked;
- Session C observations appear in Session A formation input;
- assistant output or a task-local instruction becomes active memory without an
  explicit governed path;
- ordinary conversation creates a Global Default;
- forgotten content remains in any native searchable, inspectable, replayable,
  or delivered surface covered by Vermory.

## User-Visible Flow

Normal users continue using OpenClaw, Hermes, Web Chat, or another client. The
client adapter persists turns as it already does. Formation can be triggered by
an operator or a future scheduler, but review is visible and explicit:

```text
conversation activity
-> candidate review list with source turn
-> accept, reject, correct, or forget
-> next AI turn receives only current accepted memory
```

No raw transcript dump is delivered as governed memory, and no internal audit
metadata is exposed in normal model-facing context.

## Acceptance

W21 is qualified only when all of the following are true:

- F01 is frozen and passes deterministic validation;
- migration, RLS, tenant-continuity foreign keys, manifest checks, and reset
  coverage pass against PostgreSQL;
- service and store tests prove exact evidence, isolation, idempotency,
  abstention, invalid output, snapshot drift, input drift, acceptance,
  correction, and deletion;
- document formation, OpenClaw O01, Hermes H01, retrieval, race, and operations
  regressions remain green;
- a real model forms F01 candidates from real client observations on Mac mini;
- a real OpenClaw or Hermes follow-up consumes only accepted current facts;
- the evidence report records provider, model, commands, database assertions,
  failure ledger, checksums, and protected CI artifact identity.
