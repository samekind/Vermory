# W27 Native Retrieval Follow-up

## Scope

This record covers the native retrieval slice after wiring the W27 native context
preparation command to the production retrieval coordinator. It is evidence
for the native PostgreSQL delivery path only. The subsequently completed model
utility comparison is recorded in
`docs/evidence/2026-07-19-w27-real-utility-comparison.md`.

The run used the dedicated Mac mini PostgreSQL database
`vermory_w27_utility_20260719`, PostgreSQL 18.3, and a darwin/arm64 binary
built from the working tree. The final remote binary SHA-256 was
`9637b1e0786038f3c38c35d134190514877f27c3c32c5fe729ae256843c93639`.
The run did not use NewAPI, mem0, or a replacement model.

## Implementation Change

`prepare-native-contexts` now supports `lexical`, `shadow`, and `vector`
retrieval modes. For semantic modes it:

1. Creates the retrieval coordinator with the same PostgreSQL store used for
   authoritative memories.
2. Rebuilds the current vector projection for each W27 tenant using the
   selected registered profile.
3. Delivers workspace and conversation context through the configured
   retriever, while keeping global defaults in their explicit defaults path.
4. Reads the embedding credential only from the environment variable named by
   `--embedding-api-key-env`.

The registered production profile remains direct SiliconFlow
`BAAI/bge-m3`, 1024 dimensions, through `https://api.siliconflow.cn/v1`.

## Lexical Run

Run ID: `w27-native-lexical-20260719`

The command completed all four frozen cases:

| Case | Delivered result |
| --- | --- |
| `W01-synapseloom-continuity` | Current API Gateway, frontend port 5173, source authority ordering, and Humanizer non-revival boundary |
| `C01-device-maintenance-continuity` | QQ/WeChat exclusion boundary, deleted Game A resource bundle, and current storage usage |
| `G01-language-default-local-override` | Chinese global default plus a local-scope task override |
| `S01-deletion-and-source-injection` | Recovery-code rotation guidance and rejection of untrusted source instructions as defaults |

Database checks after the run:

| Check | Result |
| --- | ---: |
| Active governed memories | 12 |
| Search projection rows | 12 |
| Memory deliveries | 4 |
| Governed content containing deleted `ORCHID` value | 0 |
| Governed content containing the Gboard fact `1,333,470` | 1 |

The Gboard fact was present in the authoritative database but was absent from
the C01 lexical delivery. The delivered C01 context contained the other
device-maintenance facts. This is a real retrieval failure, not a write or
deletion failure: the lexical query terms did not match the stored technical
fact closely enough.

## Vector Run

Run ID: `w27-native-vector-20260719`

The direct SSH shell could identify the `vermory-siliconflow` Keychain item but
could not read its secret. A one-shot user GUI launchd job could read the item,
so the final vector run was launched there. The secret existed only in that
child process environment and was not written to the command line, evidence
files, logs, or PostgreSQL.

The run used direct SiliconFlow `BAAI/bge-m3` embeddings with the registered
1024-dimensional production profile. All four cases completed:

| Case | Delivered result |
| --- | --- |
| `W01-synapseloom-continuity` | All four expected current workspace facts |
| `C01-device-maintenance-continuity` | The three lexical results plus `Gboard had 1,333,470 personal-dictionary rows.` |
| `G01-language-default-local-override` | The same two explicit global-default facts; this path does not use vector projection |
| `S01-deletion-and-source-injection` | The same two governed safety facts without the deleted recovery code |

Post-run checks:

| Check | Result |
| --- | ---: |
| Active governed memories | 12 |
| Vector projection rows | 10 |
| Memory deliveries | 4 |
| Governed content containing deleted `ORCHID` value | 0 |
| Governed content containing the Gboard fact `1,333,470` | 1 |

The workspace and conversation tenants each had an `idle` production-profile
cursor with lag 0. Their vector counts were 4 for W01, 4 for C01, and 2 for
S01. G01 intentionally had no vector cursor because global defaults are read
through the explicit defaults path.

An intermediate deployment attempt overwrote a previously executed Mach-O path
in place and was killed with status 137 before application logging. It was
discarded as deployment evidence. Uploading the same binary under a new,
versioned filename produced a valid checksum, executed successfully, and was
used for the final run. Mac mini releases should therefore use versioned files
and atomic activation rather than in-place executable overwrite.

## Interpretation

This run establishes three bounded facts:

- PostgreSQL authority, lifecycle filtering, delivery generation, and deletion
  handling are operating in the dedicated remote runtime.
- The current lexical path can lose a semantically equivalent technical fact
  even when the fact is stored and indexed.
- On the same governed authority, the vector path restored the missing C01
  Gboard fact while preserving the deletion and scope boundaries in these four
  cases.

This section alone is evidence of a native retrieval improvement for one
frozen failure. The later W27 record supplies the completed real-provider
matrix, official mem0 lane, proposed-only writeback receipts, and bounded
utility decision.
