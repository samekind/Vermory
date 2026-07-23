# Three-Client Conversation Bridge Real-Client Qualification

Date: 2026-07-22

Case: `B03-three-client-conversation-bridge`

Status: `30 / 30 PASS`, `client-qualified`

The machine-readable result is retained in the
[B03 real-client snapshot](snapshots/2026-07-22-three-client-conversation-bridge-real-clients.json).
Its `hard_gates.results` object names all 30 gates individually; the repository
policy rejects a missing, additional, or non-passing gate rather than trusting
the summary count alone.

## Accepted Run

| Field | Value |
|---|---|
| Run ID | `b03-20260722-82609f0` |
| Implementation revision | `82609f095c32db1935d27ecb569557847a78c337` |
| Vermory version | `0.0.0-SNAPSHOT-82609f0` |
| Host | Mac mini, macOS `26.5.1`, ARM64 |
| PostgreSQL / schema | `18.3 / 23` |
| Web client | Google Chrome `150` |
| Hermes | official `v0.18.2`, local revision `0f102fa4` |
| OpenClaw | official `2026.6.11 (e085fa1)` |
| Model execution | Codex CLI `0.144.3`, `main`, `gpt-5.6-terra` |
| Service scope | loopback user LaunchAgents, no `sudo` |

The accepted runtime used the exact Darwin ARM64 release from the signed
pull-request snapshot. PostgreSQL, Vermory Web Chat, the official OpenClaw
Gateway, and the official Hermes installation all ran on the Mac mini. A
temporary loopback relay on the workstation invoked the authenticated Codex
CLI and was forwarded to Mac mini loopback only for model execution. It stored
no Vermory data or provider credential and is not a product provider. Mac mini
NewAPI was not used.

The model is a compatibility target for this client trajectory, not a ranking
result.

## User-Visible Trajectory

The real Chrome application performed this sequence:

```text
Web Chat release matter
-> send raw-only chatter and receive RAW_ONLY_ACK
-> leave raw chatter unremembered
-> send the current bundle and receive BUNDLE_ACK
-> click Remember only for the bundle observation

Web Chat unrelated matter
-> send UNRELATED_RELEASE_MATTER and receive UNRELATED_ACK
-> leave the unrelated observation unremembered
```

The release continuity contained one active governed memory. The unrelated
continuity contained zero memories.

Before any bridge existed, a fresh official Hermes session and the OpenClaw
Gateway agent path both returned `NO_LINKED_BUNDLE`. Their Vermory deliveries
contained zero context bytes even though the source Web Chat memory already
existed.

The operator then linked the exact Web Chat continuity to the exact Hermes
session. Replaying the same normalized operation returned the same bridge ID
with `replayed=true`. Resuming that Hermes session produced:

```text
vermory-v8.tgz
```

The operator separately linked the exact OpenClaw session key. A fresh turn
through OpenClaw's Gateway OpenAI-compatible HTTP agent path produced:

```text
vermory-v8.tgz
```

An unlinked second Hermes session and an unlinked second OpenClaw session both
returned `NO_LINKED_BUNDLE`.

## Delivery Evidence

PostgreSQL recorded 11 completed external-client turns. The two linked turns
were the only deliveries containing `vermory-v8.tgz`, and each context body
was 88 bytes. Every external delivery had both of these assertions:

```text
contains WEB_CHAT_RAW_ONLY: false
contains UNRELATED_RELEASE_MATTER: false
```

This is stronger than checking the final answers alone: the stored model input
packet itself contained the governed bundle only for the two explicitly linked
continuities. The raw Web Chat transcript and unrelated matter were never
delivered to Hermes or OpenClaw.

## Reversal

Both bridges were explicitly reversed. Each append-only event sequence is:

```text
created -> reversed
```

Fresh deterministic prepares then returned zero context bytes for both target
continuities. The official Hermes client and OpenClaw Gateway each performed a
further real-model turn and returned `NO_LINKED_BUNDLE`. PostgreSQL recorded
zero non-empty external deliveries after the reversal timestamp.

Reversal did not delete either side's own history. After reversal, the primary
Hermes session retained 8 observations, the primary OpenClaw session retained
10 observations, and both unrelated sessions retained 2 observations. The Web
Chat source memory remained active. This is the intended contract: stop future
governed sharing without rewriting client-owned transcripts or deleting the
source.

## Database Assertions

| Assertion | Result |
|---|---:|
| External turns / completed | `11 / 11` |
| Linked bundle deliveries | `2` |
| External raw-marker deliveries | `0` |
| External unrelated-marker deliveries | `0` |
| Non-empty post-reversal deliveries | `0` |
| Source active memories containing the bundle | `1` |
| Bridge operations / events | `2 / 4` |
| Active / reversed conversation links | `0 / 2` |
| Retained server-side failed turns | `3` |

The full private database audit SHA-256 is
`e3ef379f2b95df6a987c4b35d957ed47b757753d1278ec4c49dc8e08dc6ef42e`.
The delivery audit SHA-256 is
`60ac284d35b2986cc95c6279b2345714eaec8529ddbdb10113bdf05d50e4ec61`.

## Real Model Accounting

The accepted path made 11 model turns: 3 Web Chat turns, 4 Hermes turns, and 4
OpenClaw Gateway turns. Two additional requests were relay health smokes. One
earlier official OpenClaw CLI attempt encountered a device-scope upgrade and
fell back to its embedded agent runtime. Its answer and Vermory observations
were retained, but it was rejected as B03 Gateway evidence and replaced by the
accepted Gateway HTTP agent path.

The relay log contains 14 successful requests and zero model-call failures.
Its SHA-256 is
`a13d8a7d88a65302b98be30126f8b1666d7d32da621bd529a899bfde911bea20`.

## Protected Input And Integrity

The accepted binary came from GitHub Actions run `29903259886`. Both protected
jobs succeeded, and signed artifact `8522876048` was verified before runtime
use. Selected payload hashes are:

| Payload | SHA-256 |
|---|---|
| Darwin ARM64 release | `fbeff2afce1d9f9068def55f869478008cc0de2b6205b941757a6885cc408163` |
| Vermory binary | `9bec0940ad33c3baf12d03cd0301d2d75aa3f46266a12daf3538d56741b00900` |
| OpenClaw package | `46ba8fec85ac83b09af5177dde0e9cacd35c25475e537cfa8d8f31f4a61f4b14` |
| Hermes package | `501457db314e9c1221450ea4d92a8c3709c8d9f9a8a357b803e5675a96834b42` |
| Hermes provider source | `9b2dbda2c33bc728515318f9e7522623dcdf4cd38eae7f23b5f29b39720afdbb` |

Raw transcripts, provider bodies, credentials, database URLs, private host
references, and the temporary relay source remain outside Git.

## Preserved Failures

The dedicated database still contains the three original provider failures:
the duplicate Grok wrapper flag, unavailable Grok login, and the direct
SiliconFlow insufficient-balance response. They were not deleted or relabeled.

Two later browser attempts failed before reaching the relay because the
temporary cross-host forwarding connection closed. They created no false
server receipt. A transient user LaunchAgent bootstrap retry and the rejected
OpenClaw embedded fallback are also retained in the private run ledger.

## Claim Boundary

This run qualifies one same-run Chrome Web Chat, official Hermes, and official
OpenClaw Gateway trajectory with explicit links, idempotent replay, bounded
delivery, unrelated-session isolation, reversal, and source/history retention.

It does not claim that matching names auto-link clients, that bridge reversal
deletes external transcripts, that the temporary relay is a production
provider, that every OpenClaw channel or Hermes gateway mode is qualified, or
that one model is superior to another.
