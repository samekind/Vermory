# W27 Native Retrieval Follow-up

## Scope

This record covers the follow-up run after wiring the W27 native context
preparation command to the production retrieval coordinator. It is evidence
for the native PostgreSQL delivery path only. It is not a completed W27 model
utility comparison and it does not establish a retrieval or provider ranking.

The run used the dedicated Mac mini PostgreSQL database
`vermory_w27_utility_20260719`, PostgreSQL 18.3, and a darwin/arm64 binary
built from the working tree. It did not use NewAPI, mem0, or a replacement
model.

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

## Semantic Attempt

Run ID: `w27-native-vector-attempt-20260719`

The command reached database migration and then stopped with:

```text
embedding API key is required for utility semantic retrieval
```

The Mac mini contains a `vermory-siliconflow` Keychain item, but the SSH
non-interactive session cannot read its secret (`security ... -w` returns
status 36). No vector request was sent, no projection was claimed current,
and no vector score or utility result was generated. This is intentionally
recorded as an environmental prerequisite failure rather than a zero score.

## Interpretation

This run establishes three bounded facts:

- PostgreSQL authority, lifecycle filtering, delivery generation, and deletion
  handling are operating in the dedicated remote runtime.
- The current lexical path can lose a semantically equivalent technical fact
  even when the fact is stored and indexed.
- The new semantic path is wired to the real store and projection worker, but
  its effect is not yet evidenced because the remote non-interactive execution
  path cannot obtain the embedding credential.

No claim is made here that Vermory improves downstream model task success. That
claim requires a completed real-provider W27 matrix with per-call input,
output, score, and raw artifacts, plus the independently produced mem0 lane.
