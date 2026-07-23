# Conversation Formation Loop Qualification

Date: 2026-07-18

## Accepted Run

| Field | Value |
|---|---|
| Run ID | `w21-conversation-formation-20260718` |
| Frozen case | `F01-conversation-formation-loop` |
| Final implementation revision | `67b14b16cd78a2af96de052b7a8470980d6921c9` |
| F01 fixture lock SHA-256 | `28c928024dc0d683b1617f22aac0bf3c48d095742be9dae82ab0e39d17dc8256` |
| Vermory schema | `19` |
| OpenClaw | `2026.6.11 / e085fa1` |
| Conversation model | `grok-cli/grok-4.5` |
| Formation endpoint | `https://api.siliconflow.cn/v1` |
| Formation model | `deepseek-ai/DeepSeek-V4-Flash` |
| Normalized snapshot | [`snapshots/2026-07-18-conversation-formation-loop.json`](snapshots/2026-07-18-conversation-formation-loop.json) |
| Snapshot SHA-256 | `eacb74652c2830c97a40a70d1d0a1f37906be6fb1dfff7ae654aa2aa74e29db6` |
| Qualification gates | `18 / 18 PASS` |

The accepted trajectory uses a real OpenClaw client, a real Grok conversation
turn, and a direct SiliconFlow `DeepSeek-V4-Flash` formation call. Mac mini
NewAPI is not used. The models are compatibility targets for this trajectory,
not contestants in a model ranking.

Raw client state, model artifacts, PostgreSQL inspection, failure logs, and
the complete checksum inventory remain outside Git in the user-owned Mac mini
evidence root. The repository contains the frozen public case, implementation,
automated qualification, this normalized report, and a credential-free JSON
snapshot.

## User-Visible Trajectory

The run exercises the complete conversation write-back loop:

```text
OpenClaw user turns
-> same-conversation observations
-> reviewable model-formed candidates
-> explicit accept or reject
-> corrected governed memory
-> later OpenClaw answer
-> explicit deletion
-> deletion-safe later answers
```

Session A supplied three initial user observations through the ordinary
OpenClaw plugin lifecycle:

1. plumbing inspection on Friday at 15:30;
2. technician must check in with the concierge;
3. a synthetic temporary access code, followed by rain and lunch chatter.

Vermory formed candidates only from persisted user observations in the frozen
Session A manifest. Assistant answers were never eligible evidence.

## Real Provider Formation

The direct provider path exposed and retained three materially different
attempts:

| Attempt | Result | Time | Meaning |
|---|---:|---:|---|
| default model mode | `provider_timeout` | 90 s | the preview model did not return headers before the bounded timeout |
| non-thinking, schema omitted by adapter | `invalid_provider_output` | 10 s | JSON arrived, but `occurrence` had the wrong type and transient chatter was proposed |
| non-thinking, required schema included | `completed` | 14 s | strict JSON parsed, exact evidence validated, and rain/lunch produced no candidate |

A separate minimal direct probe returned HTTP `200` from
`DeepSeek-V4-Flash` in 3 seconds. This isolated the first failure from network,
credential, and model-availability failures. The accepted fix added an
explicit optional non-thinking request control and ensured the
OpenAI-compatible adapter actually includes the required JSON schema in the
model prompt. Default requests remain unchanged unless the operator selects
the option.

The accepted model response contained four reviewable candidates:

| Candidate | Governance result |
|---|---|
| Friday appointment | accepted |
| concierge check-in | accepted |
| temporary access code | accepted, then later forgotten |
| keep the code until visit details are final | rejected as a lifecycle instruction rather than a standalone durable fact |

The model did not activate anything. Three facts became active only after
explicit operator acceptance. The extra lifecycle candidate is preserved as a
rejected model-quality result rather than being removed from the evidence.

## Correction And Temporary Instruction

A later real OpenClaw user turn stated:

```text
Correction: the building moved the inspection to Saturday at 10:00.
Friday at 15:30 is obsolete.
```

The direct formation model returned one exact-evidence `update` in 6 seconds.
Accepting it produced the following authoritative lifecycle:

| Fact | State |
|---|---|
| Friday at 15:30 | `superseded` |
| Saturday at 10:00 | `active` |
| concierge check-in | `active` |

Another real turn asked for English only for that turn. Formation returned
`abstained` with zero items. Active Global Defaults remained `0` before and
after the turn.

The first wording, `current visit time`, was ambiguous in the presence of the
OpenClaw timestamp. Grok answered the wall-clock time even though Vermory had
correctly injected Saturday at 10:00. That failed answer is retained. The final
natural task removed the ambiguity:

```text
What time is the plumbing inspection scheduled for, and what must the
technician do on arrival?
```

The real OpenClaw/Grok answer was:

```text
The plumbing inspection is scheduled for Saturday at 10:00. On arrival, the
technician must check in with the concierge.
```

The final delivery and answer contained Saturday at 10:00 and concierge. They
contained no Friday at 15:30, access code, rain, or lunch.

## Deletion

The temporary code was forgotten through the served conversation operator
route, not by directly editing authority tables. Vermory then rebuilt the
continuity's disposable lexical projection from current authority.

The first real deletion pass found one implementation defect: a failed
formation run had no item row, so its raw provider audit retained the code even
though successful formation items, observations, turns, deliveries, and search
documents were redacted. The fix now expands redaction through the deleted
candidate's evidence observation and every conversation formation manifest
that contains it. A new forget operation on an already deleted memory can
reapply redaction, which repaired the historical failed run without restoring
the memory.

Final residual counts are all zero:

| Surface | Exact deleted value occurrences |
|---|---:|
| governed memories | 0 |
| observations | 0 |
| conversation answers | 0 |
| delivery history | 0 |
| lexical search documents | 0 |
| formation run provider audit | 0 |
| formation items | 0 |
| OpenClaw isolated workspace/state | 0 |

The failed run remains a failed run with
`failure_code=invalid_provider_output`; only its protected payload and reason
are `[redacted]`.

Three real OpenClaw deletion probes were retained. Exact and paraphrased probes
did not leak the code but stopped at a client-side intent to inspect the
workspace instead of returning a final `unavailable` answer. The related-topic
probe produced no Grok output and hit the 179-second CLI watchdog. These are
client-quality failures, not accepted answer-quality results. Database,
delivery, and external-client state remained free of the deleted value after
all three probes.

## Isolation, Replay, And Concurrency

Real OpenClaw sessions A, B, and C resolved to three different continuity IDs.
Session B contained an unrelated grocery delivery; Session C contained a
thesis rehearsal. Supplying either B or C observation ID to Session A
formation failed before provider execution, and no invalid run was persisted.

A fixture provider then referenced a Session B observation while the frozen
input manifest contained only a Session A observation. The run terminated with
`evidence_observation_outside_manifest` and zero items.

The accepted initial operation was replayed with a dummy key and an unreachable
provider URL. It returned in 0 seconds with `replayed=true`, the original run
ID, one run row, and no duplicate candidates. This proves replay does not need
or call the provider.

Two provider-delay probes exercised atomic concurrency checks on the Mac mini
database:

| Probe | Terminal result | Partial items |
|---|---|---:|
| selected observation changed during provider execution | `input_manifest_changed` | 0 |
| active memory changed during provider execution | `active_snapshot_changed` | 0 |

The test values were restored after each probe. The first harness attempt used
shell command substitution and could not `wait` on the spawned child. The
observation was immediately restored, the orphan process exited, and the
already persisted run was verified as a correct `input_manifest_changed`
failure. The corrected active-snapshot probe used a direct child PID and an
exit trap.

## Fail-Open

A fresh isolated OpenClaw configuration pointed the Vermory plugin at an
unreachable loopback port. OpenClaw still returned:

```text
FAIL_OPEN_OK
```

The plugin emitted a bounded warning. Vermory binding count for that session
remained `0`; no successful persistence receipt was invented.

## Mac Mini Deployment

| Field | Value |
|---|---|
| Host | `mac-mini-of-xin-era.local` |
| Evidence root | `~/Library/Application Support/Vermory/evidence/w21-conversation-formation/5d488f5` |
| Listen | `127.0.0.1:8791` |
| Database | `vermory_w21_f01_5d488f5` |
| Tenant | `w21-f01` |
| Final binary revision | `67b14b16cd78a2af96de052b7a8470980d6921c9` |
| Final binary SHA-256 | `210a0d6f5027e85a5cea7e678ddfd89fc62c84966cb8110fe0fe261c536ed9ab` |
| Service state | loopback listener present, user-owned process |

The binary was built from the local external-disk repository and transferred
directly over SSH. OpenClaw and Hermes user-level installations already present
on the Mac mini were reused. No package manager reinstall or `sudo` was used.
An early unrelated broad `jstarctl` inspection path triggered local
administrator prompts while inspecting a system DNS service; that command was
unnecessary for Vermory deployment and was not used again.

The stable Vermory services on `8787` and `8789`, and the stable OpenClaw
gateway on `18789`, were not replaced. Two turns accidentally sent through the
stable OpenClaw wrapper were snapshotted for recovery and then removed in an
exact transaction before the isolated run continued.

## Automated Verification

Fresh local verification on the final source changes passed:

```text
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 \
    ./internal/runtime ./internal/webchat ./internal/operatorcli ./cmd/vermory

go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
git diff --check
```

The full PostgreSQL suite, W21/F01 lifecycle, provider tests, OpenClaw O01,
Hermes H01, RLS, deletion, projection rebuild, retrieval, recovery, and
operations regressions remain green.

## Protected Delivery

The first protected delivery for the complete W21 source head passed all 23
main CI steps:

| Field | Value |
|---|---|
| Source head | `cb78aed2335f115a85ed422b7ac099535dd6418f` |
| Run / job | `29609943989 / 87981998623` |
| Result / duration | `SUCCESS / 293 seconds` |
| Artifact | `8418419540` / `vermory-pr-snapshot-66c9aa96dfcd3b7d244b74f792ebceab60264c92` |
| Artifact bytes | `21,657,451` |
| Artifact API and transport SHA-256 | `36d4d41ce080d7c7a7316160fd45094d680615ab3a3c592fbddde9d1b081f277` |
| Synthetic merge | `66c9aa96dfcd3b7d244b74f792ebceab60264c92` |
| Merge verification | `verified=true`, `reason=valid` |
| Merge second parent | `cb78aed2335f115a85ed422b7ac099535dd6418f` |

The artifact was streamed to the Mac mini without a persistent local copy. The
first stream was observed at `10,092,544 / 21,657,451` bytes and failed ZIP
validation, so it was rejected. A range resume supplied the remaining
`11,564,907` bytes. The accepted stable ZIP matched both the Artifact API byte
count and digest and passed central-directory validation.

Independent Mac mini verification passed all four Go archive checksums and
layouts. Every binary reported the expected `GOOS` and `GOARCH`,
`CGO_ENABLED=0`, `-trimpath=true`, `vcs.modified=false`, and synthetic-merge
revision. Both Linux binaries were statically linked. The Darwin arm64 binary
executed `version` and `memory form-conversation --help`.

The OpenClaw `0.1.0` package contained exactly 12 files and had SHA-256
`e2833e6a5d5cbaf72af244b7c1650532fd81b0dfcb435c84f2462dd1906c3950`.
The deterministic Hermes `0.1.0` package sidecar passed, the package contained
exactly six files, and its SHA-256 was
`99dd0c2c99cbf5703eec8cbc4a7e84442e2662af8becb81985d8b481ec90dfe6`.
The extracted packages contained zero environment or key files, dependency or
cache directories, credential-shaped values, or bearer-token values.

Protected-delivery evidence is stored under
`~/Library/Application Support/Vermory/evidence/w21-conversation-formation/5d488f5/protected-delivery-cb78aed-29609943989`.
Its checksum manifest covers 45 files and has SHA-256
`39c6204479be5767f4dce2718033a2752c21a08ba4caff22c688c12ceffaf556`.
The structured verification record has SHA-256
`045b5438966c8ef65d1d56c3ead786fecf7624645a6f997954490ed111c6c35b`.

## Integrity And Privacy

The Mac mini raw evidence checksum manifest covers 6,458 files. It was created
before the normalized summary was appended and excludes only the checksum file
itself. The manifest SHA-256 is:

```text
f9e23207106b3dfcb17586f67eba2d870830ee749b5287fdbb989e6c7215fc0a
```

Privacy scan results:

| Check | Count |
|---|---:|
| provider key matches in repository | 0 |
| provider key matches in Mac mini evidence | 0 |
| environment files | 0 |
| direct credential assignments | 0 |
| full-environment dumps | 0 |
| `sudo` / authorization helper processes | 0 |

The provider key was read from the already authorized local task record and
sent over SSH standard input into one remote process. It was never written to
Git, `.env`, OpenClaw configuration, command arguments, evidence logs, or the
Mac mini login Keychain. Noninteractive SSH was unable to access that Keychain,
and the run did not claim otherwise.

## Preserved Failure Ledger

1. the first validator expected `.result.payloads`; real OpenClaw output uses
   top-level `.payloads`;
2. the stable OpenClaw wrapper overrode W21 isolation variables and sent two
   test turns to the stable tenant; the rows were reversibly snapshotted and
   removed;
3. noninteractive SSH could not access the login Keychain;
4. default-thinking `DeepSeek-V4-Flash` formation timed out after 90 seconds;
5. the OpenAI-compatible adapter discarded the required JSON schema;
6. the model proposed a lifecycle instruction as a separate candidate;
7. failed formation audit retained the deleted code until manifest-aware
   redaction and repair forget were implemented;
8. the ambiguous `current visit time` prompt produced a wall-clock answer;
9. exact and paraphrased OpenClaw deletion probes stopped at workspace-search
   intent rather than a final unavailable answer;
10. the related deletion probe hit the 179-second Grok CLI watchdog;
11. the first drift harness could not wait on a child created inside command
    substitution;
12. the model-generated appointment `memory_key` retained a stale temporal
    token after the content was corrected from Friday to Saturday.
13. the first protected artifact stream was truncated and rejected before a
    range-resumed transport matched the Artifact API size, digest, and ZIP
    integrity checks.

No failed attempt was deleted, relabeled as a pass, or used as accepted
answer-quality evidence.

## Non-Claims

- W21 does not rank Grok, DeepSeek, or any other model.
- W21 does not claim every model will form stable keys without review.
- W21 does not claim OpenClaw always returns a good answer when its tool or CLI
  backend stalls.
- W21 deletion covers Vermory authority, audit, delivery, projection, and the
  isolated client state checked in this run; it is not a universal purge of
  every external platform's independent transcript storage.
- W21 qualifies one complete real conversation formation trajectory. It does
  not by itself prove formation quality at corpus scale or complete the
  overall Vermory release, signing, cross-host, and sealed-evaluation goals.
