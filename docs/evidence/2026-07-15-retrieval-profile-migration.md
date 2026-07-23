# Retrieval Profile Migration Evidence

Date: 2026-07-15

Status: completed as a parallel projection migration rehearsal; measured cutover decision recorded separately

## Contract

Vermory keeps PostgreSQL governed memories authoritative and treats embeddings
as disposable, profile-scoped projections. Migration 15 adds a registry and
allows two 1024-dimensional profiles to consume the same durable projection
events independently:

| Profile | Model | Lifecycle | Runtime role |
|---|---|---|---|
| `siliconflow-bge-m3-1024-v1` | `BAAI/bge-m3` | `active` | default |
| `siliconflow-bge-large-zh-1024-v2` | `BAAI/bge-large-zh-v1.5` | `candidate` | migration comparison |

The migration does not replace v1, mutate governed memory, or make the
candidate profile the default. Each profile has its own cursor and vector
rows; the same authority events can therefore be rebuilt, compared, and
reversed independently. Both profiles share the current fixed 1024-dimensional
projection class. A future dimensionality change requires a separate physical
projection class and is not silently accepted by this migration.

## Real Run

The live test ran against the W10 authority database after migration 15, using
the direct SiliconFlow endpoint and real provider responses. Before rebuilding,
both disposable profiles were reset to cursor zero. The test then rebuilt v1
and v2, executed vector retrieval through two profile-specific coordinators,
and checked authority status and result content.

| Field | Value |
|---|---|
| Implementation | `145993d` |
| Database | dedicated `vermory_w10_clean` |
| PostgreSQL schema | `15` |
| v1 model | `BAAI/bge-m3` |
| v2 model | `BAAI/bge-large-zh-v1.5` |
| v1 embedding requests | `31` |
| v2 embedding requests | `31` |
| v1 vector rows | `30` |
| v2 vector rows | `30` |
| v1 cursor lag | `0` |
| v2 cursor lag | `0` |
| v1 top result | rollback approval fact |
| v2 top result | production release command fact |
| shared required fact | `pnpm exec verify:release --profile production` present in both results |

The different top rank is expected model behavior and is retained as evidence;
the migration gate is preservation of governed facts, projection isolation,
complete rebuild, and explicit default status, not byte-identical ranking from
two different embedding models.

## Database Gates

- Migration 15 `Up` and `Down` both pass, including foreign-key restoration on `Down`.
- The registry contains exactly the active v1 and candidate v2 profiles.
- v1 and v2 projections coexist without sharing primary keys or cursors.
- Rebuilding v2 leaves v1 row count unchanged.
- Both profile cursors reach lag zero.
- A real vector query through each profile returns the governed release command.
- Profile-specific retrieval fingerprints prevent a v1/v2 audit replay from being treated as the same profile.
- The candidate worker's in-flight embedding completion is rejected after authority deletion and a retry does not restore the candidate vector row.
- The default CLI/runtime profile remains `siliconflow-bge-m3-1024-v1`.

The follow-up W10 production comparison froze promotion thresholds and retained
v2 as a candidate after repeatable quality regression. See
[the promotion decision](2026-07-15-retrieval-profile-promotion-decision.md).

## Non-Claims

- The candidate profile is not the production default.
- This is not a model quality ranking or a claim that v2 is better.
- This does not prove arbitrary embedding dimensions or automatic cutover.
- This does not replace the required scale, sealed-evaluation, or final release gates.
