# Official OpenClaw WebChat Governed Continuity Qualification

Date: 2026-07-23

Reality case: `W38-official-openclaw-webchat`

Status: `28 / 28 PASS`, `runtime-qualified`

The machine-readable result is the
[W38 public snapshot](snapshots/2026-07-23-official-openclaw-webchat.json).
It is bound to frozen case SHA-256
`4cc844975c0be930bb3029e9336c62bfa5a7577271ee3389606f83ee1654116a`,
exact implementation revision
`6811b117292484872e0e3c89379edbf664efaa06`, and exact candidate binary
SHA-256 `e5146e8c61e8920ea6e7c865fa61daac8b4c5bc8159ff1dbc4a2249a9c1b6b66`.

## Qualified Boundary

W38 qualifies the maintained Vermory plugin inside the official OpenClaw
Control UI, not a synthetic WebChat facade:

```text
real Chrome
-> official OpenClaw 2026.6.11 Control UI
-> Gateway WebSocket chat.send / chat.history
-> maintained Vermory before_prompt_build / agent_end hooks
-> stateless real Grok CLI turn
-> PostgreSQL-authoritative formation, governance, recall, correction, deletion
```

The accepted runtime used PostgreSQL 18.4 schema 25 and Grok CLI 0.2.111. The
OpenClaw configuration requested `grok-4.5`; all six WebChat turns reported the
actual provider route `grok-4.5-build-free`. Formation made one additional real
Grok call with requested model `grok-4.5`; that command did not emit the
provider's resolved route. This is compatibility evidence, not a model ranking.

All runtime data, PostgreSQL files, OpenClaw and Grok state, caches, transcripts,
and raw evidence remained on the external volume. The run used no privileged
command and no private NewAPI gateway. System-home Go build and module caches
were both zero bytes after shutdown.

## User-Visible Trajectory

The browser trajectory used a household service appointment because it has a
durable fact, a temporary distractor, a correction, an unrelated conversation,
and an unambiguous deletion probe:

```text
tell OpenClaw about Saturday 10:00 and the front-desk arrival instruction
-> include a temporary lunch suggestion in the same message
-> complete one real Grok turn
-> form one reviewable appointment candidate, not the lunch suggestion
-> inspect it with /vermory memories
-> accept it with /vermory accept

reset the host transcript
-> ask for the appointment
-> receive Saturday 10:00 and the front-desk instruction from fresh Vermory context

open an unrelated OpenClaw session
-> ask the same question
-> receive UNKNOWN

correct the memory through /vermory correct
-> reset the host transcript again
-> receive Sunday 14:00 and never present Saturday as current

forget the current memory through /vermory forget
-> reset again
-> exact probe returns UNKNOWN
-> a separate fresh paraphrased probe recovers none of the deleted fact
```

The first formation output contained exactly one candidate under
`household.home-network-service-appointment`. The lunch suggestion produced no
candidate. No memory became active from the model output itself; activation,
correction, and forgetting all occurred through explicit `/vermory` commands in
the official UI.

## Why Recall Was Attributed To Vermory

Each recall boundary used OpenClaw `/reset`. Gateway `chat.history` proved that
the canonical session key remained `agent:main:main`, its backing session ID
changed, and the new transcript contained only the reset exchange before the
question. It contained neither appointment value nor the arrival instruction.

The first fresh transcript then answered:

```text
Saturday at 10:00; the technician must contact the front desk before arriving.
```

After correction, another fresh transcript answered:

```text
Sunday at 14:00; the technician must contact the front desk before arriving.
```

Because Grok ran with provider session mode `none`, ambient memory and web
search disabled, and no pre-reset host message in the supplied transcript, the
appointment could only have arrived through the current Vermory delivery.

## Isolation, Correction, And Deletion

The unrelated OpenClaw session resolved to a different conversation continuity
and answered `UNKNOWN`. The main and unrelated sessions remained the only two
conversation continuities. PostgreSQL also retained the platform's one thin
`global_defaults` continuity, making three continuity rows in total; that row
contained no appointment memory and was not removed to manufacture a lower
count.

Correction left one `superseded` Saturday memory and one active Sunday
replacement. Forgetting changed the replacement to `deleted`. The exact
post-delete probe answered `UNKNOWN`. The independent paraphrased probe did not
obey the requested fixed phrase, but it disclosed neither day/time nor the
front-desk instruction. That wording miss is retained; the deletion gate is the
absence of fact recovery, not forced model phrasing.

Final PostgreSQL authority recorded:

| Assertion | Result |
|---|---:|
| Conversation continuities | 2 |
| Thin Global Defaults continuities | 1 |
| Completed conversation turns | 6 |
| Observations | 16 |
| Deliveries | 6 |
| Formation schedules | 2 |
| Active memories | 0 |
| Superseded memories | 1 |
| Deleted memories | 1 |
| Current lexical projection rows | 0 |
| Current 1,536-dimension vector rows | 0 |
| Current 2,560-dimension vector rows | 0 |
| Old markers in the latest two fresh deliveries | false |

Deleting the governed memory did not erase prior OpenClaw transcripts. It
removed the fact from current Vermory delivery and current projections, which
is the platform's declared deletion boundary.

## Reload, Restart, And Replay

A clean browser reload recovered the active backing transcript with the same
session ID and produced zero console errors or warnings. Vermory and the
OpenClaw Gateway were then stopped while PostgreSQL remained running. Chrome
recorded six expected connection-refused errors and six reconnect warnings
during this deliberate outage, automatically reconnected after restart, and
recovered the same transcript and deleted lifecycle outcome.

After restart, `/vermory memories` displayed:

```text
No pending candidates or active memories for this OpenClaw session.
```

A completed real browser operation was submitted again to
`/v1/client-operations/prepare`. PostgreSQL returned its original terminal
receipt with `status=completed` and `replayed=true`; conversation-turn,
observation, delivery, formation-schedule, and governed-memory counts were
unchanged. A final clean reload again produced zero errors and zero warnings.

## Evidence Integrity

The private external run retains 49 checksum-indexed raw evidence files. The
authoritative report SHA-256 is
`9fc02f1bab766f384166ed333b410f6590f823599d47b6f679421e2a6e365fda`; the raw
inventory SHA-256 is
`4777bcfd52c847f00f7f3f89618232cc488bac58cd9f447754c37be7c3ece6fd`.
Known runtime and Grok authentication values matched zero raw evidence files.
After shutdown, the copied Vermory credentials, database password, OpenClaw
Gateway/device credentials, and Grok auth file were removed. The public
snapshot contains no local path, endpoint, credential, database URL, or proxy
configuration.

## Retained Failures

Rejected and precursor attempts were not rewritten as passes:

1. Direct invocation of the Grok runtime installer failed before executable handling was corrected.
2. A copied Homebrew launcher failed strict code-signature validation.
3. Copying the Node shim omitted its platform package, while OpenClaw runtime inspection had also changed shape.
4. One complete precursor was rejected because the harness was still changing during the run.
5. The first exact formation invocation carried a duplicate `--verbatim` flag and failed.
6. A precursor model answer contained an irrelevant alphanumeric string.
7. The first automated new-session send failed to dispatch an input event and created no turn.
8. The browser tool could not write a screenshot to the canonical external evidence location.
9. The accepted paraphrased deletion probe missed the requested fixed wording but leaked none of the deleted fact.

## Claim Boundary

W38 qualifies this exact official OpenClaw 2026.6.11, Chrome, Grok CLI, Vermory
plugin, and PostgreSQL trajectory. It does not qualify every OpenClaw release,
browser, messaging channel, remote topology, automatic governance policy, or
formation-quality distribution. It does not rank Grok against another model,
and it does not treat OpenClaw transcripts, browser state, or Grok private state
as memory authority.
