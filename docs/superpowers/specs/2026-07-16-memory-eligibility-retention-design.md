# Memory Eligibility, Retention, And Forgetting Qualification Design

Status: accepted for implementation under the active Vermory goal

Date: 2026-07-16

## 1. Purpose

W18 bounded `memory_projection_events`, a disposable delivery log. It did not
qualify how authoritative memory itself becomes current, temporarily valid,
historical, archived, or forgotten.

W19 qualifies that missing boundary without turning Vermory into a TTL cache.
It must prove that:

- task-local working context stops applying outside its task without becoming
  a global preference;
- continuity-durable facts remain reusable until governed change makes them
  ineligible;
- Global Defaults remain explicit, thin, and independent from local overrides;
- scheduled and expired facts are filtered consistently by every retrieval
  path;
- archive preserves inspectable history but removes current reuse eligibility;
- authorized forgetting erases protected content rather than merely hiding it;
- expiry, archive, and forgetting remain correct across clients, provider
  degradation, projection rebuild, restart, dump/restore, and concurrent
  governance operations.

This slice tests Hypothesis H-007. It does not assume that a permanent
`retention_class` column is the right answer merely because the product has
three user-visible continuity roles.

## 2. Fixed Product Contracts

The following contracts come from the Product Constitution and are not schema
hypotheses:

1. Eligibility, retention, and forgetting are different decisions.
2. Raw conversation or tool output is not automatically durable memory.
3. Temporary usefulness does not imply durable retention.
4. Global Defaults cannot be silently learned from ordinary use.
5. Expired or archived content must not be represented as current.
6. Authorized forgetting must make protected content unrecoverable within the
   governed scope.
7. PostgreSQL remains the only memory authority.
8. Lexical remains the default retrieval mode; semantic retrieval is explicit
   opt-in and remains a disposable projection.
9. Retrieval applies authorization, continuity, lifecycle, validity, privacy,
   and deletion before relevance ranking.
10. No model, embedding provider, cache, adapter, or client owns lifecycle
    truth.

## 3. Design Alternatives

### 3.1 Explicit retention-class enum plus scheduler

Add `working`, `continuity_durable`, and `global_default` to every memory row,
then run a background scheduler that changes lifecycle state at expiry.

This is rejected for W19 because it conflates location, reuse scope, temporal
validity, and deletion. It also introduces a hidden scheduler whose lag and
restart behavior could decide whether stale facts are exposed.

### 3.2 Materialized expiry lifecycle

Keep the current model but add `scheduled` and `expired` lifecycle states. A
worker moves rows between states when wall-clock boundaries pass.

This is better than a retention enum, but it still makes current correctness
depend on worker timeliness. It also creates avoidable races between expiry,
correction, archive, and forgetting.

### 3.3 Orthogonal eligibility and explicit governance

Keep continuity role, lifecycle, temporal validity, and privacy deletion as
separate dimensions:

- working context remains observations, current input, and bounded recent
  turns rather than governed durable memory;
- continuity-durable memory remains an active governed fact attached to a
  workspace or conversation continuity;
- Global Defaults remain explicitly governed `global_default` memories in the
  dedicated defaults continuity;
- `valid_from` and `valid_until` control temporal eligibility without deleting
  content or requiring a scheduler;
- `archived` means retained historically but not eligible for reuse;
- `deleted` remains the result of authorized forgetting and irreversibly
  redacts protected content.

This is the selected design. It tests whether H-007 is better expressed by
continuity, authority tier, validity, and lifecycle than by one stored class.

## 4. User-Visible Semantics

### 4.1 Working context

Working context is the current request plus the client-owned or
Vermory-recorded recent task trajectory. It is not a governed memory merely
because the model used it successfully.

A local instruction such as "write this table in English" applies to that
task. It may remain in the task transcript for explanation, but it does not
change the stable Chinese Global Default and is not injected into a new
unrelated continuity.

### 4.2 Continuity-durable memory

A confirmed fact in a workspace or conversation continuity is durable by
default. It remains eligible until one of these governed events occurs:

- it is superseded by a newer revision;
- it reaches its explicit `valid_until` boundary;
- it is archived;
- it is forgotten.

Durable does not mean permanent. A user or trusted source can give a fact a
bounded validity window when the fact itself is temporary.

### 4.3 Global Defaults

Global Defaults remain explicit and open-ended unless corrected or forgotten.
A temporary override belongs to the active task or continuity, not to the
Global Defaults layer. W19 does not add expiring "temporary global defaults";
that phrase is a scope contradiction and would make pollution easier.

### 4.4 Expiry

Expiry changes reuse eligibility, not historical truth. At or after
`valid_until`, the memory:

- is absent from current context, lexical retrieval, vector retrieval, bridge
  export, and active source matching;
- remains visible in authorized inspection with an effective state of
  `expired`;
- keeps its content and provenance unless a separate forget action requires
  redaction;
- can still be targeted by an explicit correction or forget operation.

The interval is half-open: `[valid_from, valid_until)`. A memory is not
eligible before `valid_from` and is no longer eligible exactly at
`valid_until`.

### 4.5 Archive

Archive is an explicit governance action. It changes lifecycle state to
`archived`, preserves content and provenance for authorized inspection, and
removes the memory from current use. W19 does not silently archive at expiry.

An archived row is not reactivated in place. Reuse after archive creates a new
governed revision so history remains explainable.

### 4.6 Forget

Forget is privacy deletion, not ordinary expiry or archive. Existing redaction
semantics remain authoritative:

- governed content becomes `[redacted]`;
- origin and affected downstream conversation content is redacted;
- lexical and vector projections are removed or made absent;
- delivery history containing the exact governed content is redacted;
- source-match and source-formation audit content is redacted while permitted
  structural evidence remains;
- rebuild and restart cannot recover the content.

Forget may target current, scheduled, expired, archived, or superseded memory.
It wins over concurrent validity changes and must not be reversible through a
late operation replay.

## 5. Authority Model

W19 does not add a `retention_class` field.

The user-visible role is derived as follows:

| Role | Authoritative representation | Default reuse duration |
|---|---|---|
| Working context | current request, observations, and recent turns | task or continuity dependent; never silently promoted |
| Continuity-durable | governed `fact` in workspace or conversation continuity | open-ended unless bounded, superseded, archived, or forgotten |
| Global Default | governed `global_default` in the dedicated defaults continuity | open-ended until explicit correction or forget |

The selected physical change adds temporal eligibility to governed memory and
an immutable audit record for explicit eligibility/archive operations.

### 5.1 Governed memory validity

Schema 18 adds:

```text
governed_memories.valid_from   TIMESTAMPTZ NULL
governed_memories.valid_until  TIMESTAMPTZ NULL
```

`NULL` means unbounded on that side. A database constraint requires
`valid_until > valid_from` when both are present.

Schema 18 also extends the lifecycle constraint with `archived`. It does not
add `scheduled` or `expired`; those are effective states derived at one
request-level `as_of` timestamp.

Schema 18 also records that request-level timestamp on the evidence produced
by serving paths:

```text
memory_deliveries.eligibility_as_of
memory_retrieval_runs.eligibility_as_of
```

These fields are audit evidence. They do not let clients request historical
context and they do not change the authority timestamp of the memory itself.

### 5.2 Effective state

Inspection derives exactly one state in this order:

```text
deleted or redacted lifecycle      -> deleted
superseded lifecycle               -> superseded
rejected lifecycle                 -> rejected
proposed lifecycle                 -> proposed
archived lifecycle                 -> archived
active and as_of < valid_from      -> scheduled
active and as_of >= valid_until    -> expired
active inside validity interval    -> current
```

Lifecycle remains authoritative history. Effective state answers whether the
memory is eligible for current use at one evaluation instant.

### 5.3 Request-level time

Every context preparation or retrieval operation uses one PostgreSQL-derived
UTC `as_of` timestamp. Global Defaults, lexical candidates, vector candidates,
bridge export, and final eligibility checks for the same operation use that
same timestamp.

The timestamp is stored with the resulting delivery and retrieval audit so a
later inspection can reproduce the boundary decision.

The public client cannot supply an arbitrary historical `as_of` value for
normal context generation. Tests and administrative inspection may use an
explicit bounded timestamp through a separate diagnostic path.

Correctness never depends on a scheduler changing a row at the wall-clock
boundary.

## 6. Governance Operations

Schema 18 adds an RLS-protected immutable operation table:

```text
memory_eligibility_operations
```

Each row records:

- tenant, continuity, memory, and operation identity;
- action: `set_validity` or `archive`;
- previous lifecycle and validity bounds;
- resulting lifecycle and validity bounds;
- content-free request fingerprint;
- creation time.

The table is audit evidence, not memory content. Runtime roles may inspect
their tenant's permitted receipts only through services; they cannot mutate
memory eligibility directly. Operator/admin paths perform mutations.

### 6.1 Set validity

`SetMemoryValidity` accepts one exact memory target and optional UTC
`valid_from`/`valid_until` values.

Rules:

- the target must belong to the caller's tenant and continuity;
- deleted, superseded, rejected, and archived targets cannot be changed;
- an expired active target may be extended explicitly;
- equal operation replay returns byte-stable receipt data;
- the same operation ID with different input is rejected;
- no content is copied into the operation record;
- setting validity does not promote proposed content or change scope;
- validity change never overrides a concurrent completed forget.

### 6.2 Archive

`ArchiveMemory` changes one current or expired active memory to `archived`.

Rules:

- the action is explicit and idempotent;
- proposed, rejected, superseded, deleted, and already archived targets cannot
  be silently transformed;
- lexical projection is removed transactionally;
- projection events remove optional vector rows through the existing outbox;
- content remains inspectable to authorized users;
- future correction requires a new governed revision rather than in-place
  reactivation.

### 6.3 Existing correction and forget

Correction may target an active memory even when its effective state is
scheduled or expired. The old revision becomes superseded and the replacement
never inherits a stale deadline implicitly. The replacement is unbounded by
default unless the caller performs an explicit validity operation.

Forget can target every non-deleted lifecycle state and retains existing
redaction behavior. A forget transaction locks the target row; a concurrent
validity or archive operation must observe the committed deletion or lose with
no partial receipt.

## 7. Projection And Retrieval Semantics

### 7.1 Projection storage

Temporal validity is not a privacy boundary. Active non-redacted facts may
remain in disposable lexical/vector projection storage while scheduled or
expired so natural activation and expiry require no scheduler.

Every serving query joins authoritative memory and applies the request-level
validity predicate before ranking. Therefore stale projection rows cannot be
returned as current.

Archive and forget are different:

- archive removes current search projection through lifecycle change;
- forget removes projection and redacts protected authority content;
- rebuild includes only non-redacted active lifecycle rows, regardless of
  temporal eligibility, then serving-time validity still controls exposure.

This keeps future activation possible without a timed projection worker.

### 7.2 Required retrieval paths

The same eligibility predicate must cover:

- workspace lexical search;
- conversation and linked-conversation lexical search;
- Global Defaults listing;
- production lexical, shadow, and vector coordinator paths;
- bridge export source selection;
- source candidate and source-match current-target selection;
- MCP context delivery;
- Web Chat and OpenClaw context preparation;
- snapshot rebuild and dump/restore qualification.

No path may rely only on `lifecycle_status = 'active'` after schema 18.

### 7.3 Degradation

Embedding timeout, provider rejection, projection lag, or a non-current vector
profile may degrade an explicit semantic request to lexical. The lexical path
must apply the same `as_of`, validity, scope, and lifecycle checks. Provider
failure cannot make an expired fact visible.

## 8. Client Surface

Normal AI clients receive no retention metadata. They receive only eligible
semantic content.

Administrative surfaces add:

```text
vermory memory set-validity
vermory memory archive
```

Both require an operator/admin database path, tenant-bound scope, an exact
memory ID, and an operation ID. They return structured receipts suitable for
audit and replay.

MCP, Web Chat, and OpenClaw remain consumers rather than lifecycle authorities.
They can propose or record observations but cannot set validity or archive a
memory without the existing governed operator path.

## 9. Frozen Reality Cases

W19 uses four product trajectories rather than one synthetic TTL test.

### 9.1 G01 local language override

Existing `G01-language-default-local-override` remains unchanged:

- stable Global Default is Chinese;
- one MCM task explicitly requests English;
- the task ends;
- a new unrelated Chinese request receives Chinese;
- no English Global Default is created or substituted.

### 9.2 S01 authorized forgetting

Existing `S01-deletion-and-source-injection` remains unchanged:

- a temporary recovery code is forgotten;
- exact, paraphrased, related-topic, restart, and rebuild probes cannot reveal
  it;
- independent rotation guidance remains available;
- an untrusted source cannot force permanent retention or global promotion.

### 9.3 C02 bounded conversation fact

Add `C02-housing-viewing-validity`:

- a viewing appointment is confirmed for one housing-search continuity;
- the appointment fact is useful before the event and expires at the event
  boundary;
- a durable budget preference in the same continuity remains current;
- after expiry, Web Chat and Grok must not recommend acting on the old
  appointment but must still recall the budget preference;
- authorized inspection shows the appointment as expired rather than deleted.

### 9.4 W03 temporary workspace workaround

Add `W03-workspace-workaround-validity`:

- a repository uses a temporary cache-disable workaround until a fixed
  boundary;
- a durable deployment command and security constraint remain current;
- before expiry, Codex or Grok receives the workaround;
- after expiry, the same workspace no longer receives or acts on it;
- archive preserves the workaround history without current injection;
- forget of a separate synthetic secret remains irreversible.

## 10. Formal W19 Profile

The formal profile is named `memory-eligibility-retention-v1` and uses a fresh
disposable PostgreSQL 18 cluster at schema 18.

The frozen corpus contains:

- four tenants;
- five workspace or conversation continuities per tenant;
- 10,000 governed facts total;
- 4,000 current open-ended facts;
- 1,500 scheduled facts;
- 1,500 expired facts;
- 1,000 archived facts;
- 1,000 superseded facts;
- 1,000 deleted/redacted facts;
- 320 concurrent scoped queries;
- deterministic embeddings for bulk mechanics;
- one direct SiliconFlow `BAAI/bge-m3` projection/query probe;
- one real Web Chat/Grok trajectory and one real MCP/Codex or Grok workspace
  trajectory.

Counts are qualification workload, not a universal capacity claim.

## 11. Baseline Conditions

Each reality trajectory records four conditions where applicable:

1. `no_context`: no historical state;
2. `full_history`: unfiltered transcript or fact history;
3. `lifecycle_only`: current implementation behavior using lifecycle without
   temporal validity;
4. `vermory_eligibility`: continuity, lifecycle, request-level validity,
   privacy, and deletion before ranking.

The purpose is not to rank LLMs. It is to show where no context loses durable
utility, where full history or lifecycle-only reuses stale facts, and whether
Vermory preserves useful current context without stale or deleted leakage.

## 12. Acceptance Gates

W19 has sixteen hard gates:

1. schema 18 adds validity, archive, immutable operation audit, RLS, tenant
   foreign keys, and revoked PUBLIC privileges;
2. working input does not silently create continuity-durable or Global Default
   memory;
3. one request uses one PostgreSQL-derived `as_of` across every context source;
4. scheduled facts are absent before `valid_from` and eligible at the boundary;
5. facts are eligible before `valid_until` and absent exactly at the boundary;
6. expiry preserves inspectable content and provenance but removes current
   reuse;
7. archive preserves inspectable content and removes lexical/vector/current
   delivery eligibility;
8. forget redacts current, scheduled, expired, archived, and superseded targets
   without deleting unrelated valid guidance;
9. G01 local override leaves the Global Default unchanged;
10. workspace and conversation durable facts remain available across client
    restart and client change;
11. lexical, shadow, vector, bridge, Web Chat, OpenClaw, and MCP paths enforce
    identical eligibility;
12. projection rebuild and PostgreSQL dump/restore preserve effective results
    and never revive forgotten content;
13. validity/archive operations are tenant-scoped, idempotent, conflict
    rejecting, and auditable without copied memory content;
14. concurrent forget wins over validity/archive with no resurrection or false
    receipt;
15. provider outage degrades safely without stale, deleted, or cross-scope
    exposure;
16. real Web Chat/Grok and MCP/Codex-or-Grok artifacts complete the intended
    task using only eligible memory.

Every hard gate is zero tolerance. Aggregate recall or latency cannot mask a
stale, deleted, or cross-scope result.

## 13. Metrics And Reports

The report records:

- task success per condition;
- current-fact recall;
- scheduled-fact premature use;
- expired-fact misuse;
- archived-fact misuse;
- deletion residue;
- Global Default pollution;
- cross-tenant and cross-continuity leakage;
- validity operation replay/conflict counts;
- archive and forget race outcomes;
- lexical/vector degradation reasons;
- context token count and delivered-memory count;
- database, projection, and restore fingerprints;
- real client/model identity and artifact hashes;
- direct embedding-provider tuple without credentials or raw vectors.

Deterministic assertions decide scope, lifecycle, boundary, redaction, replay,
and count gates. LLM output is evidence only for downstream task behavior, not
for database correctness.

## 14. Failure And Recovery Qualification

The profile preserves these failures rather than hiding them:

- a validity update interrupted before commit;
- a concurrent forget racing a validity extension;
- an embedding timeout followed by lexical degradation;
- PostgreSQL restart followed by same-pool recovery;
- a conflicting operation-ID replay;
- a deliberately stale projection row that serving-time validity must reject;
- a dump/restore and projection rebuild with expired, archived, and forgotten
  controls.

No threshold, timestamp, or case may be changed solely to convert a failed
attempt into a passing report.

## 15. Security And Privacy

- Runtime application roles cannot directly mutate validity or archive state.
- Operator actions are tenant scoped and require exact targets.
- RLS and tenant-bearing foreign keys protect operation receipts.
- Operation receipts contain no governed content, provider body, vector, DSN,
  token, or credential.
- Historical inspection is authorization controlled and never enters normal
  model-facing context.
- Expiry is not advertised as deletion.
- Archive is not advertised as deletion.
- Forget remains the only W19 operation that guarantees protected content
  erasure within the qualified scope.

## 16. Explicit Non-Goals

W19 does not add:

- a generic policy language;
- a hidden scheduler or cron service;
- automatic privacy deletion at expiry;
- legal-hold or jurisdiction-specific compliance claims;
- table partitioning;
- Redis or a second queue;
- NewAPI;
- automatic Global Default promotion;
- automatic semantic-profile promotion;
- cross-host HA evidence;
- artifact signing;
- a final release.

## 17. Decision After Qualification

If all gates pass, H-007 becomes supported with this interpretation:

> Vermory does not need one stored retention-class enum for V1. Working
> context, continuity-durable memory, and Global Defaults are distinct
> authority/scope roles. Temporal eligibility, archive, supersession, and
> authorized forgetting remain orthogonal governed dimensions.

If the cases require repeated special handling that cannot be expressed by
continuity, authority tier, validity, and lifecycle, H-007 remains open and a
separate retention-policy entity is reconsidered with the failures preserved.

W19 completion does not complete the overall Vermory goal. Genuine external
sealed evaluation, cross-host HA evidence, artifact signing, and final release
acceptance remain separate boundaries.
