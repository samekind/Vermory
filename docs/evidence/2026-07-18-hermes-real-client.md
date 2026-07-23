# Hermes Real-Client Continuity Qualification

Date: 2026-07-18

## Accepted Run

| Field | Value |
|---|---|
| Run ID | `w20-hermes-real-20260718` |
| Frozen case | `H01-hermes-linked-sessions` |
| Implementation revision | `4f5e04a5dbfd68eba25326a456d8529926fe7a02` |
| H01 fixture lock SHA-256 | `8248f9ac22a50bcd4d33ec0f715301294404afc98954c27b892e96a308fb5511` |
| Vermory schema | `18` |
| Hermes version / revision | `0.18.2 / 0f102fa4dc04b7dfdab048169aaaa640d09d7523` |
| Provider endpoint | `https://api.siliconflow.cn/v1` |
| Model | `deepseek-ai/DeepSeek-V4-Flash` |
| Hard gates | `16 / 16 PASS` |

The accepted trajectory used the official Hermes CLI and a direct
OpenAI-compatible SiliconFlow route. It did not use Mac mini NewAPI. The model
is a compatibility target for this run, not a model-ranking result.

Raw transcripts, database inspection, provider response bodies, and runtime
logs remain outside Git under the Mac mini evidence root. The repository keeps
the frozen public case, implementation, deterministic tests, and this
normalized readout.

## Acceptance Contract

The frozen H01 case required the complete external-client lifecycle:

1. create a real Hermes session A and persist its completed turn through the
   Vermory provider;
2. explicitly confirm the user observation that `thesis-defense-v7.zip` is
   current and `thesis-defense-v6.zip` is obsolete;
3. create session B in a fresh isolated `HERMES_HOME` whose transcript and
   Hermes-owned memory contain neither filename;
4. prove that similar wording alone does not connect A and B;
5. explicitly link the two Vermory continuities;
6. resume B through `hermes chat -Q --resume`, consume only the governed A
   memory, and answer with the current filename;
7. exclude raw transcript history and unrelated Hermes/OpenClaw continuities;
8. reverse the link and prove a fresh direct prepare for B returns no A memory;
9. stop only the Hermes-specific Vermory canary and prove Hermes still returns
   a visible model answer without a false persistence receipt;
10. keep the provider credential out of artifacts, configuration, command
    arguments, logs, and full-environment dumps.

Pinned Hermes `v0.18.2` does not honor `--resume` when it is combined with the
top-level `--oneshot` path. That command creates a new session and is rejected
as continuity evidence. The accepted path uses `hermes chat -Q --resume`.

## Accepted Linked Recall

Session A formed and then explicitly confirmed one current governed memory:

```text
memory_id: 49a5bca6-468f-4399-a95d-dbc35ad70f7c
current:    thesis-defense-v7.zip
obsolete:   thesis-defense-v6.zip
```

Session B used a newly created isolated Hermes home. Before linking, both the
official redacted Hermes transcript and Vermory inspection contained zero
occurrences of either filename. The only cross-session relationship was the
explicit Vermory bridge:

```text
bridge_id: 90321c0c-1e35-4d43-9b7b-a391ee2f1a2a
session_b:  20260718_001553_e383cc
```

After linking, the real DeepSeek turn answered:

```text
thesis-defense-v7.zip
```

The corresponding Vermory delivery was:

```text
delivery_id: ca31a11c-4675-40f6-94fc-115cbe462fb3
model:       deepseek-ai/DeepSeek-V4-Flash
context:     Governed memory:
             Please acknowledge this current thesis submission fact in one
             sentence without using tools: the current upload bundle is
             thesis-defense-v7.zip, and thesis-defense-v6.zip is obsolete.
```

The delivery contained exactly the confirmed governed memory. It contained no
`Recent conversation:` block, no unrelated Hermes continuity, no OpenClaw
continuity, and no raw session A transcript. The obsolete filename appeared
only as an obsolete fact and was not returned as the current answer.

`HERMES_INFERENCE_MODEL` was set to the same model selected by the CLI. The
database therefore recorded `deepseek-ai/DeepSeek-V4-Flash` instead of guessing
or retaining the earlier honest fallback value `hermes/unreported`.

## Reversal

The bridge was explicitly reversed. A fresh direct call to the Hermes prepare
endpoint for session B then returned `0` context bytes. Neither thesis filename
from session A remained deliverable to B.

Hermes-owned transcript history was not deleted and is not claimed to be
deleted. Reversal stops future governed-memory sharing through that bridge; it
does not rewrite an external client's own history.

## Fail-Open Without False Persistence

The fault run stopped only the Hermes canary on `127.0.0.1:8789`. The stable
Vermory service on `127.0.0.1:8787` and the OpenClaw gateway on
`127.0.0.1:18789` remained untouched.

A new isolated Hermes session still returned the real model answer:

```text
FAIL-OPEN-OK
```

The Hermes process exited `0`. Vermory binding count delta and conversation
turn delta were both `0`. No successful Vermory receipt was created and the
evidence makes no persistence claim for that answer. The canary was restored
after the fault injection.

## Mac Mini Deployment

The accepted canary is a user-level LaunchAgent:

| Field | Value |
|---|---|
| Label | `org.vermory.hermes-canary` |
| Listen | `127.0.0.1:8789` |
| Tenant | `hermes-canary` |
| Binary | `$HOME/.vermory/vermory-hermes/vermory` |
| Binary SHA-256 | `b90e8f941d4aff054f4abe128924ac1951619620f6442ebbbedaa5eb9b17e56a` |
| Provider source SHA-256 | `9b2dbda2c33bc728515318f9e7522623dcdf4cd38eae7f23b5f29b39720afdbb` |
| Restart state | `running`, loopback listener present |

The credential is stored in the Mac mini login Keychain. A user GUI
LaunchAgent reads it into the one-shot client process environment. It is not
written to `.env`, Hermes configuration, command arguments, or evidence logs.
No `sudo` is used for the Hermes provider, canary, or client trajectory.

## Tests, CI, And Release Package

Hermes provider tests pass `6 / 6`. CI creates its uv environment under `/tmp`,
disables Python bytecode in the worktree, and verifies that the repository
remains clean.

Protected CI for the implementation revision completed successfully:

| Field | Value |
|---|---|
| Run / job | `29595348849 / 87934267544` |
| Result | `SUCCESS` |
| Artifact ID | `8412888557` |
| Artifact bytes | `21,560,419` |
| Artifact ZIP SHA-256 | `826fb0b96b04239f7f092200516f73394b1bea19de846bd8b5bc9f71c4dff4d9` |

The protected artifact was streamed directly to the Mac mini and independently
verified. Its Hermes package sidecar matched, and the package contained exactly
these six files:

```text
vermory-hermes-0.1.0/LICENSE
vermory-hermes-0.1.0/integrations/hermes/README.md
vermory-hermes-0.1.0/integrations/hermes/pyproject.toml
vermory-hermes-0.1.0/integrations/hermes/uv.lock
vermory-hermes-0.1.0/integrations/hermes/vermory/__init__.py
vermory-hermes-0.1.0/integrations/hermes/vermory/plugin.yaml
```

The package contains no virtual environment, bytecode, credential, user
configuration, or transcript data.

## Integrity And Privacy

At accepted-trajectory normalization, before final documentation-delivery
metadata was appended, the Mac mini evidence root contained 116 files and the
relative checksum manifest verified `115 / 115` entries. The point-in-time
trajectory hashes were:

| Artifact | SHA-256 |
|---|---|
| `summary.json` | `8e1921244bc4a8a4145717ab0616babed5a574241f19654ac82880716206e9e6` |
| `failure-ledger.md` | `2dbeee92b90d3ff41e13f667c771cff2241c9ab3e4a58d313bebfb7cca3bdc58` |
| `privacy-report.json` | `69a533b944c53002ae61e74dc9504933ea6b0b55996b41565d2f6802cedea7d8` |
| `checksums.sha256` | `7405e685b0fbb4f88dc841234b8be0704ef3d7fd917abc6f1da73ab441bfbc84` |

The external `summary.json`, privacy report, and checksum manifest are updated
again when the final protected documentation artifact is attached. That
delivery metadata does not alter the accepted Hermes sessions, memory,
delivery, bridge, model answer, or failure ledger recorded above.

The privacy scan covered credential token shapes, bearer authorization values,
inline password assignments, environment files, and full environment dumps.
Credential leak count, environment file count, and full-environment dump count
were all zero.

## Preserved Failure Ledger

Seven failed or rejected paths remain visible before the accepted attempt:

1. no inference provider configured;
2. local shell quoting failed before SSH execution;
3. noninteractive SSH could inspect Keychain metadata but not read the value;
4. `--oneshot --resume` created a new session and Hermes-owned memory produced a
   false-positive answer while the Vermory delivery was empty;
5. the correct resume path first stopped at Hermes' noninteractive setup gate;
6. valid linked recall recorded `hermes/unreported` because Hermes did not pass
   the model keyword;
7. the intentional fail-open run returned an answer without persistence.

Attempt 8 added explicit process-level model reporting and passed the complete
H01 contract. The false positive and intermediate failures were not removed or
reclassified as successful continuity evidence.

## Non-Claims

- This run does not rank DeepSeek or any other model.
- This run does not claim Hermes built-in memory is disabled globally; session
  B was isolated with a fresh `HERMES_HOME`.
- This run does not claim bridge reversal deletes provider-owned transcripts.
- This run does not qualify every Hermes gateway or messaging channel.
- This run does not qualify OpenClaw messaging-channel behavior.
- This run does not complete the overall Vermory product, scale, security,
  cross-host deployment, signing, or open-source release acceptance.
