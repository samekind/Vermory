# Automatic Conversation Formation And Review Qualification

Date: 2026-07-18

## Accepted Run

| Field | Value |
|---|---|
| Run ID | `w22-f02-20260718-b6c6154` |
| Frozen case | `F02-automatic-conversation-review` |
| Fixture lock SHA-256 | `e5f5762885b2372742412f7f9499f49de08c7a854d59dae4babb4875a79c8231` |
| Implementation revision | `6038cece6c9c949e5006de1f0145c11b431f2899` |
| Vermory schema | `20` |
| OpenClaw | `2026.6.11 / e085fa1` |
| OpenClaw conversation model | `grok-cli/grok-4.5` |
| Formation provider / model | `grok-cli / grok-4.5` |
| Hermes | `v0.18.2 / 0f102fa4dc04b7dfdab048169aaaa640d09d7523` |
| Hermes conversation provider / model | direct SiliconFlow / `deepseek-ai/DeepSeek-V4-Flash` |
| Vermory binary SHA-256 | `2a33536c73d2ea8da6500badb76a7471043514d733fe560611ecd8d179c9968f` |
| OpenClaw package SHA-256 | `26f1d729999f3fb16eb3626bea1864a065ba8867e84510bc66c89ea39dbc0ca0` |
| Qualification gates | `23 / 23 PASS` |

The accepted trajectory ran on the user-owned Mac mini with isolated user
paths, a dedicated PostgreSQL database, a restricted PostgreSQL login role,
an authenticated Vermory API on `127.0.0.1:8792`, an isolated OpenClaw gateway
on `127.0.0.1:18790`, and a fixed-tenant formation worker. Stable Vermory,
OpenClaw, and Hermes services were not replaced. No `sudo` or Mac mini NewAPI
route was used.

Raw client state, service logs, database inspection, package output, and the
complete checksum inventory remain outside Git under:

```text
~/Library/Application Support/Vermory/evidence/
  w22-automatic-conversation-review/w22-f02-20260718-b6c6154
```

The repository retains the frozen public case, implementation, automated
tests, this normalized report, and a credential-free JSON snapshot.

## User-Visible Trajectory

W22 turns the W21 operator-triggered formation path into an asynchronous
conversation loop:

```text
completed OpenClaw or Hermes turn
-> durable same-continuity schedule
-> fixed-tenant provider worker
-> review inbox with exact user evidence
-> explicit accept, reject, correct, or forget
-> later client turn receives only accepted current memory
```

The formation provider never activates, rejects, corrects, deletes, or
promotes memory. Ordinary client credentials can complete lifecycle calls but
cannot open or mutate the review inbox. OpenClaw governance uses a separate
operator credential and a direct command that does not register a model tool.
Hermes exposes no governance tool at all.

## Initial OpenClaw Formation

A real OpenClaw/Grok turn supplied three synthetic thesis-submission facts:

- current upload bundle: `thesis-defense-v7.zip`;
- faculty portal deadline: Tuesday at 18:00;
- faculty coordinator office: B-412.

The visible turn completed in 20 seconds. Formation did not run inside the
completion request. The durable schedule later reached:

```text
state=idle requested=1 processed=1 last_status=completed
```

The worker formed exactly three proposed candidates:

| Key | Proposed content | Exact source evidence |
|---|---|---|
| `thesis-defense-upload-bundle` | current bundle is `thesis-defense-v7.zip` | exact user sentence containing the filename |
| `faculty-portal-deadline` | deadline is Tuesday at 18:00 | exact user sentence containing the deadline |
| `faculty-coordinator-office` | office is B-412 | exact user sentence containing the office |

Before review, all three were `proposed`; none was active. The review response
contained the candidate key, proposed content, exact source quote, decision,
and short reference. It contained no provider prompt, provider output, raw
artifact, fingerprint, or formation reason.

## Direct OpenClaw Governance

The accepted review path used OpenClaw gateway `chat.send`, which dispatches
registered slash commands before model execution. `openclaw agent --message`
was rejected as command evidence because that CLI path forwards slash text to
the model instead of the direct-command dispatcher.

The user-facing command sequence was:

```text
/vermory memories
/vermory accept ba664a8b
/vermory accept e1351c9e
/vermory reject ffdb5ff7
```

The direct command responses were:

```text
Accepted [ba664a8b] thesis-defense-upload-bundle.
Accepted [e1351c9e] faculty-portal-deadline.
Rejected [ffdb5ff7] faculty-coordinator-office.
```

The plugin runtime reported `status=loaded`, command `vermory`, and zero model
tools. References were valid only inside the current OpenClaw session cache.

## Correction And Current Recall

A second real OpenClaw turn stated:

```text
The faculty portal moved the deadline to Wednesday at 12:00.
Tuesday at 18:00 is obsolete.
```

The worker formed one exact-evidence `update` candidate:

```text
faculty-portal-deadline -> Wednesday at 12:00
```

It remained proposed until `/vermory accept 82789dcc`. Acceptance produced the
authoritative lifecycle:

| Fact | State |
|---|---|
| Tuesday at 18:00 | `superseded` |
| Wednesday at 12:00 | `active` |
| B-412 office | `rejected` |
| `thesis-defense-v7.zip` | `active` |

A fresh real OpenClaw/Grok turn then answered:

```text
Current thesis upload bundle: thesis-defense-v7.zip
Faculty portal deadline: Wednesday at 12:00
```

The answer contained neither Tuesday nor B-412.

## Forgetting

The current deadline was forgotten through `/vermory forget 82789dcc`.
Post-operation authority contained:

| Key | State |
|---|---|
| old Tuesday deadline | `superseded` |
| current Wednesday deadline | `deleted` |
| office | `rejected` |
| upload bundle | `active` |

The served operator inspection returned exactly one active memory: the upload
bundle. A later real OpenClaw/Grok turn, asked only for the current bundle,
returned exactly:

```text
thesis-defense-v7.zip
```

It contained no Wednesday, Tuesday, or B-412 text. This W22 run qualifies the
served active-memory and later-delivery behavior. It does not claim to rewrite
OpenClaw's own transcript history; W21 separately qualifies broader Vermory
redaction surfaces.

## Hermes Isolation Control

An official Hermes `v0.18.2` process ran from a fresh W22 `HERMES_HOME` with
the current Vermory provider. Its model call used direct SiliconFlow
`DeepSeek-V4-Flash`; the credential was read at runtime from the Mac login
Keychain by a user LaunchAgent and was never written to command arguments,
configuration, evidence, or logs.

The first C-204 wording was retained as an honest `abstained` formation result
because it did not clearly establish durable future use. A resumed turn in the
same Hermes session added ongoing rehearsal-logistics semantics. The worker
then formed one proposed candidate:

```text
rehearsal.printer_room -> Current rehearsal printer room is C-204.
```

The operator inbox results were:

| Scope | Pending candidates |
|---|---:|
| exact Hermes continuity | 1, containing C-204 |
| thesis OpenClaw continuity | 0 |

The exact Hermes source quote was `the current rehearsal printer room is
C-204`. The OpenClaw inbox contained no Hermes content. The installed Hermes
provider reported `available`, returned an empty tool schema, and raised
`NotImplementedError` for a governance tool call.

## Authentication Boundary

The Mac mini database used a dedicated `LOGIN NOSUPERUSER NOCREATEDB
NOCREATEROLE NOINHERIT NOBYPASSRLS` runtime role. Runtime connection
validation reported `row_security=on`. RLS was enabled on:

- `conversation_formation_schedules`;
- `source_formation_runs`;
- `source_formation_items`;
- `governed_memories`.

The OpenClaw lifecycle used a client token. Review and governance used a
separate operator token. A client-token request to list candidates returned
HTTP `403`.

## Worker Restart, Fail-Open, And Replay

Only the W22 formation worker was stopped. With the worker unavailable, a real
OpenClaw/Grok turn completed in 8 seconds and persisted a durable schedule:

```text
before restart: pending | requested=25 | processed=0
```

The answer remained visible; chat did not wait for or fail with the formation
provider. Restarting the restricted worker recovered the schedule and formed:

```text
defense-rehearsal.current-slide-deck
-> defense-rehearsal-v3.pdf
```

The terminal schedule was:

```text
after restart: idle | requested=25 | processed=25 | last_status=completed
```

Replaying the exact completed operation returned `replayed=true`. After three
seconds, schedule state, requested and processed cursors, attempt count, last
run ID, and formation-run count were unchanged. No second provider call or
candidate batch was created.

## Automated Verification

Fresh W22 qualification passed:

```text
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w22_final_test_0718?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w22_final_race_0718?host=/tmp' \
  go test -race -p 1 -count=1 \
    ./internal/runtime \
    ./internal/webchat \
    ./internal/operatorcli \
    ./internal/authn \
    ./cmd/vermory

go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
git diff --check
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
```

OpenClaw passed `48 / 48` tests. Hermes passed `6 / 6` tests. The full Go and
PostgreSQL suite, race targets, vet, module drift, Reality Program, RLS,
replay, concurrency, deletion, reset, packaging, and cross-client controls
remained green.

## Failure Ledger

The external evidence root retains every failed or rejected path, including:

- the original empty F02 JSONL event;
- missing running-window content fingerprint;
- incorrect optional composite-FK delete action;
- stale Reality Program case count;
- missing explicit tenant-context calls on exported Store methods;
- Apple rsync `--protect-args` incompatibility and the corrected staging move;
- non-interactive Node PATH and Corepack cache/version failures;
- rejected shared `node_modules` ownership and missing offline tarball;
- OpenClaw device scope-upgrade and slash-command routing mistakes;
- the observation-sequence polling assumption;
- the first honest Hermes formation abstention;
- the rejected `pg_tables.forcerowsecurity` evidence query;
- the first protected CI failure after fixture EOF normalization changed two
  frozen bytes without regenerating the manifest and lock;
- the first artifact metadata verifier passed `-trimpath=true` to `grep`
  without `--`, so the pattern was parsed as an option;
- the first package verifier inherited W21's twelve-entry OpenClaw inventory
  even though W22 intentionally adds `governance.js` and `governance.d.ts`;
- two PR-body orchestration cleanup failures that happened outside product
  execution and did not alter the accepted artifact or runtime evidence;
- non-fatal package-only OpenClaw channel setup warnings.

The accepted path did not delete failed evidence or reinterpret it as success.

## Integrity And Privacy

The normalized Mac mini evidence set contained 29 scanned text files and
18,271 bytes before the checksum manifest was added. Credential-pattern hits
were `0`. The binary package archive was excluded from text scanning and was
verified separately by SHA-256 and inventory.

The checksum manifest passed full verification. Its SHA-256 is:

```text
9acaae45d041fb8ed4f4755a79a3edaf0719bddb60e4e8d32a3c99b52c6508f1
```

The OpenClaw package contains fourteen files: six JavaScript files, six TypeScript
declarations, `openclaw.plugin.json`, and `package.json`. It contains no tests,
dependency tree, environment file, token, or user state.

## Protected Delivery

The first protected run, `29618465089` / job `88008485535`, failed after two
F02 fixture files were normalized without regenerating the frozen manifest and
lock. The scenario semantics were unchanged; the fixtures were refrozen under
lock SHA-256
`e5f5762885b2372742412f7f9499f49de08c7a854d59dae4babb4875a79c8231`.
The failure remains in the public and Mac mini ledgers.

Protected CI then passed on exact source head
`3e1f7b9c9a0e6eb5de902db96ec0c28218f54bcb`:

| Field | Value |
|---|---|
| Run / job | `29618745068` / `88009318831` |
| Conclusion | `SUCCESS` in `5m4s` |
| Artifact | `8421515213` |
| Artifact name | `vermory-pr-snapshot-53f16117762ab05cf26f56f33524b102604f5637` |
| Artifact bytes | `21,753,983` |
| API and streamed ZIP SHA-256 | `6b4600fa4c7b5c59d94fd2f73f9f84f5e030c9e99bd554291ef0f3b854cf2772` |
| Synthetic merge | `53f16117762ab05cf26f56f33524b102604f5637` |
| Merge verification | `verified=true`, `reason=valid` |
| Synthetic merge second parent | exact source head `3e1f7b9c9a0e6eb5de902db96ec0c28218f54bcb` |

The independently streamed artifact remained on the Mac mini. Verification
proved:

- all four release checksums and exact four-file Go archive layouts;
- expected `GOOS` / `GOARCH`, `CGO_ENABLED=0`, `-trimpath=true`,
  `vcs.modified=false`, and the synthetic merge revision;
- static Linux amd64 and arm64 binaries;
- real Darwin arm64 execution of `version` and
  `conversation-formation-worker --help`;
- OpenClaw package SHA-256
  `dc3f2489b3a3260739d0675d75c238c25dbdf83cb2b35e75e72baa063f220c63`
  with the exact fourteen-entry inventory;
- Hermes package SHA-256
  `c06b4112161c07be1dc4e2f8b858e28801a5c7345a315fefbb57c92313d9c7b9`,
  an exact six-file inventory, and a passing sidecar;
- zero package credential-pattern, sensitive-name, or local-runtime-path
  hits.

The protected-delivery checksum manifest covers `43` files. Its SHA-256 is:

```text
9146537930fc97894fe342c5fd20ada7ac4118eb26a5442bd794cf91ba588e92
```

The accepted protected evidence remains under:

```text
~/Library/Application Support/Vermory/evidence/
  w22-automatic-conversation-review/
  w22-f02-20260718-b6c6154/
  protected-delivery-3e1f7b9-29618745068
```

## Scope

This run qualifies automatic scheduling, review-safe evidence, explicit
governance, corrected current recall, forgetting, worker restart, completion
replay, role separation, and OpenClaw/Hermes isolation for the executed F02
trajectory. It does not rank Grok against DeepSeek, prove that every future
conversation produces ideal memory candidates, or claim deletion of external
clients' own transcript stores.
