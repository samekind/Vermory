# Memory Eligibility And Retention Runtime

This runbook records the normalized W19 real-client evidence. Raw provider
responses, authenticated client state, full traces, and database ledgers remain
outside Git on the Mac mini under:

```text
$HOME/Library/Application Support/Vermory/evidence/w19/w19-real-client-20260717
```

The committed record contains only bounded semantic output, non-secret runtime
identity, hard-gate results, and artifact hashes. It does not contain API keys,
gateway tokens, OAuth material, full OpenClaw configuration, vectors, or raw
provider request bodies.

## Runtime Identity

| Component | Accepted runtime |
|---|---|
| Vermory | `0.1.0-alpha.1`, revision `f8696b96a2792c11299e6ce735877d8e7e1dbd58-dirty` |
| Vermory binary SHA-256 | `624875f615c7963836e2f5532b9bf066746a335c4e23aa544572948a77413f36` |
| Vermory Codex replay | exact revision `1da8318b8b90f555d2d0012ac0b5513aa77c0ee0` |
| Codex replay binary SHA-256 | `46050e2a3e45ade21fd2ce82dbf8378ffad0f390d62146cff67475cda5146ad0` |
| PostgreSQL | `18`, schema `18`, Unix socket under `/tmp` |
| W19 database | `vermory_w19_real_20260717` |
| Grok CLI | `0.2.101 (5bc4b5dfadcf)` |
| Grok binary SHA-256 | `8431538dbd99379240f558b48b779c651d668b06d793c87311ad532c4395a4e2` |
| Grok model | `grok-4.5` |
| Codex CLI | official `0.144.3`, isolated ephemeral runtime |
| Codex model transport | direct DuoJie Responses API, `gpt-5.5` |
| OpenClaw | `2026.6.11`, loopback Gateway with Vermory plugin `0.1.0` |

The Grok wrapper used an isolated runtime home and the existing authenticated
CLI state. Cross-session memory, plan mode, subagents, and web search were
disabled. The accepted Codex replay used the official Codex CLI with a separate
isolated configuration and a direct DuoJie Responses transport. Neither client
used the Mac mini NewAPI route. The model route is compatibility evidence, not
a model ranking or promotion decision.

## Formal Qualification

The accepted formal profile is `w19-formal-2e97280-20260717-v1` at exact
implementation revision `2e9728043cc2c4f39e296fd2612c23299c5a541f`.
PostgreSQL `18.3` with pgvector `0.8.5` created 10,000 governed memories across
four tenants and twenty continuities. All `320 / 320` scoped queries completed;
all 4,000 current facts were returned; scheduled, expired, archived, deleted,
Global Default, and cross-scope misuse were zero. All sixteen hard gates passed.

The profile bound the real Grok Web Chat, Grok MCP, and official Codex MCP hashes
below, then performed one direct SiliconFlow `BAAI/bge-m3` projection/query probe
at 1,024 dimensions and exactly two requests. The normalized report contains
only provider response hashes, not raw bodies or vectors.

Report hashes:

| Artifact | SHA-256 |
|---|---|
| JSON | `a61ddbcd2f5c49a7843ad206b694b38b13027739574e68abc54a487e45b47e0d` |
| Markdown | `c5a2d0f6798470488b1c6e15338a6efc331cd6f4ea29947c8d2a2459a9d43005` |

With the provider credential unset and invalid PostgreSQL paths, offline replay
validated the report before database or network startup and preserved both
hashes. See
[Memory Eligibility And Retention Evidence](../evidence/2026-07-16-memory-eligibility-retention.md).

## G01: Task-Local Language Override

The real Web Chat service ran against the W19 database with the authenticated
Grok CLI provider.

The task-local request returned an English table-facing response. A later,
unrelated thread returned Chinese and explicitly retained the Chinese global
default. PostgreSQL inspection after both turns showed exactly one active,
current `reply_language` default:

```text
Default user-facing replies to Chinese unless the active task explicitly
requests another language.
```

The task-local English instruction did not create, replace, or mutate a Global
Default. Both model-facing deliveries omitted lifecycle fields, memory IDs,
retrieval metadata, and unrelated thread history.

Selected evidence:

| Artifact | SHA-256 |
|---|---|
| initial provider failure | `2f5950960ccc22579fa67aeccdb98ac75173b344b107ce15072b782be07a7db8` |
| successful English task | `4a8de5cb8b11a4d29608e16ff6251b83cc250f7861a0888c2c1aabc3272e81d5` |
| later unrelated Chinese task | `543093982ab76ca9cebbbb2378b07a132254ab7eada886ccc7c2b16b2152dd41` |
| post-run Global Default inspection | `8445aa6a69fff58b0083302fa64d4641bb13501a3be645d31976cbeda5df56e2` |

## C02: Bounded Conversation Fact

The real trajectory confirmed two independent facts in one housing-search
continuity: a durable monthly budget and a time-bounded viewing appointment.
Before the boundary, Grok received both. At the exact database-clock boundary,
the new delivery retained the budget and suppressed:

- the expired viewing origin;
- the sibling assistant answer that had repeated the viewing;
- the earlier provider answer produced before the boundary;
- lifecycle and validity metadata.

The inspection surface still retained the appointment as authorized history
with `lifecycle_status=active` and `effective_state=expired`. Expiry therefore
changed current eligibility without silently archiving or deleting the fact.

The five persisted delivery gates were all true:

```text
durable budget present
expired viewing origin absent
sibling assistant repetition absent
pre-boundary answer absent
internal metadata absent
```

This real replay found a production defect: recent history filtered the expired
user observation but could still include the assistant observation from the
same turn. The runtime now evaluates recent conversation history with the same
request-level `eligibility_as_of` snapshot and suppresses assistant history
derived from an ineligible origin or delivery. The regression contract is
`TestExpiredConfirmedUserObservationSuppressesSiblingAssistantHistory`.

Selected evidence:

| Artifact | SHA-256 |
|---|---|
| pre-boundary real turn | `392bfdd154b37ea42c4ca339b7b65860451727c89015329df637f59db1643fd4` |
| exact-boundary corrected turn | `9d2d8fbf3dfccd12bc643bcd104b762b651e7c34de2ba825c17d766df1e4e7df` |
| exact-boundary hard gates | `58daaf2625869eab44508fecfcfccc9b51a06fd0a3eb95d4bc25bd22252fe350` |
| post-boundary inspection | `1ec49ae5424457940de3b5c8b9b3d50ec241fc57b7cf1e478b95e4bc758e052b` |

## W03: Real Grok MCP Workspace Control

The disposable W03 workspace was already bound to continuity
`8e9badce-4972-49be-83e3-cfe0f53276f8`. Its authority state before the client
run contained:

- one expired temporary workaround;
- one current durable verification and security fact;
- one deleted, redacted synthetic secret.

The isolated Grok runtime registered one temporary stdio MCP server. `grok mcp
doctor` reported one healthy server, protocol `2025-06-18`, and exactly two
tools: `prepare_context` and `commit_observation`.

Grok then executed this real chain:

```text
prepare_context (w19-w03-grok-prepare-1)
-> receive only the current durable workspace constraint
-> create GROK_CURRENT_CONSTRAINTS.md
-> run go test -p 1 -count=1 ./...
-> commit_observation (w19-w03-grok-commit-1)
-> retain the agent result as proposed
```

The bounded artifact was:

```markdown
# Current Constraints

## Verification
- Run: `go test -p 1 -count=1 ./...`

## Security
- `.env` files must never be committed.
```

An independent replay returned `ok example.com/vermory/w03`. The artifact did
not contain the expired workaround, the deleted secret, memory IDs, validity
fields, lifecycle fields, or other internal metadata. PostgreSQL recorded one
delivery and one `agent_result` write-back on the same confirmed continuity.
The write-back remained `proposed` and did not become current memory. After
evidence capture, the temporary MCP registration was removed and the disposable
workspace was restored to a clean Git state.

All normalized Grok MCP gates passed:

```text
database: 12 / 12 true
client:    5 / 5 true
artifact:  6 / 6 true
verification: independent go test passed
governance: proposed write-back did not become current
```

Selected evidence:

| Artifact | SHA-256 |
|---|---|
| artifact snapshot | `ab97a28882afbcbb7bc7b7e78ec1f50b46494fe23c654ea15878cd1f44417a95` |
| exported Grok transcript | `cb5041363fd58c4cdeb606e5cdc454e2a3b85ab52b1c358846c52afed3f9d886` |
| normalized runtime gates | `bbaf5adbc21dc5b6e53692d49ab9d0742b4d3b80c29f51557d06181a0c377416` |
| local trace archive | `c9520c401a9e5f21f261f8527d4541a82b5450658b7ea6b1637e0ec83ad124b6` |

## W03: Official Codex MCP Boundary Replay

Official Codex CLI `0.144.3` ran with an isolated ephemeral configuration,
direct DuoJie `gpt-5.5`, and one temporary SSH-stdio MCP registration pointing
to the exact Codex-replay Vermory binary above. MCP negotiation returned
protocol `2025-06-18` and exactly two tools: `prepare_context` and
`commit_observation`.

The prompt did not state the expected workspace facts. It required Codex to use
the returned semantic context as its only authority, preserve exact technical
strings, create one bounded artifact, verify it locally, and write the result
back with the delivery receipt.

Before the boundary, the real client completed:

```text
prepare_context
-> receive the current temporary workaround
-> receive the durable verification command
-> receive the durable security constraint
-> create workspace-before-boundary.md
-> verify every delivered constraint and reject credential syntax
-> commit_observation
-> retain the result as proposed
```

The bounded artifact was:

```markdown
# Workspace Boundary

- Use VERMORY_CACHE_DISABLED=1 only while the temporary workaround is current.
- .env files and secret-bearing local configuration must never be committed.
- The durable verification command is go test -p 1 -count=1 ./....
```

After an explicit operator validity update moved the same workaround past its
half-open eligibility boundary, a fresh Codex session completed the same MCP
lifecycle. Its delivery and artifact retained only the two durable constraints:

```markdown
# Current Durable Constraints

- The durable verification command is `go test -p 1 -count=1 ./...`.
- .env files and secret-bearing local configuration must never be committed.
```

The second client also executed
`GOCACHE=<disposable-workspace-cache> go test -p 1 -count=1 ./...`; the test
passed. The cache was evidence-only and is not part of the committed artifact.

PostgreSQL assertions for each accepted run showed exactly one delivery, the
same confirmed continuity, one `agent_result` observation, the approved
artifact source reference, and one proposed memory. Before the boundary the
delivery contained all three eligible constraints. After the boundary it
contained the durable verification and security constraints but not the
workaround. Neither delivery, artifact, final message, nor event result
contained the forgotten synthetic secret. Both write-backs remained proposed.

All normalized Codex gates passed:

```text
MCP protocol and tool surface: 3 / 3 true
before-boundary database/artifact gates: 12 / 12 true
after-boundary database/artifact gates: 12 / 12 true
governance: 2 proposed write-backs, 0 automatic promotion
forgetting: deleted synthetic secret absent from both deliveries
```

Selected evidence:

| Artifact | SHA-256 |
|---|---|
| before-boundary artifact | `4543b0538f7007ab698bbdb8669e7439db7636003271d8586af13a4ca0e486e8` |
| before-boundary Codex event stream | `ea3ab1de2b461f542d699a299842756465f5a7c40fb22b18005170960e8ceed4` |
| before-boundary final message | `4ccabbba0762fe9bfe58258f20bd4076d2776cd0d240d1b3ebe74b23f0fb386c` |
| after-boundary artifact | `9c543f5ac961d741be974677a77e0d3b82521bfde5e5b1fddf06ffac6d6d407f` |
| after-boundary Codex event stream | `e0b96d1485965f14773953e69344dc642c744dd9d2145c402fb9fff00d5f7220` |
| after-boundary final message | `6f3026096c467d0f568f3b7311fc859eac3f21ebd45437b56aafdd646ef842f4` |
| ordered before/after event-stream aggregate | `1570aa87f321e0d9f4785cc14abab0bfa684af2b490d36f59d90335dbce88cfc` |
| ordered before/after artifact aggregate | `067031a5ae9c73b86a3f409236a851d1abeaa5a240b3a8b1045bdcf3c936cc1f` |

The raw bounded evidence and checksum manifest are stored with the other W19
real-client evidence on the Mac mini, outside Git.

## Preserved Failure Ledger

Failures remain chronological and are not replaced by the later successful
artifacts:

1. The first G01 provider call passed `--verbatim` through both the wrapper and
   provider, and Grok rejected the duplicate flag. The runtime wrapper was split
   into an environment-only provider wrapper and a strict one-turn OpenClaw
   wrapper before retrying.
2. The first C02 validity command generated an invalid RFC3339 timestamp. The
   CLI rejected it before a database write; the retry used the PostgreSQL clock.
3. The first exact-boundary C02 replay exposed stale sibling assistant history.
   A RED regression reproduced it, the shared snapshot filter was fixed, and the
   full real turn was rerun.
4. A nonexistent `memory rebuild-projection` CLI command was attempted during
   W03 setup and rejected without changing authority. W19 lexical forget already
   updates authority and projection transactionally.
5. The first official Codex attempt stopped at the external ChatGPT
   usage-limit gate before a successful MCP call. Its event stream is retained
   as failed client evidence.
6. The initial SSH-stdio registration did not preserve quoting around the
   PostgreSQL socket query parameter. The remote shell rejected the command;
   an independent MCP handshake exposed the transport defect before retrying.
7. The first direct-provider Codex run reached the model but its idle FRP SSH
   transport closed before `prepare_context`. Three tool attempts failed. The
   registration was corrected with bounded SSH keepalive options, then a fresh
   session completed both tools.
8. The accepted before-boundary run had one non-mutating discovery command
   return no matches before the artifact existed. The later deterministic
   artifact checks passed.
9. The accepted after-boundary run had one concurrent exact-text check observe
   the pre-final artifact state. The same check was repeated after the write and
   passed, followed by a successful Go test.
10. Two evidence-only helper attempts failed after the successful Grok run: one
   shell-quoted SQL gate command and one non-login-shell verification that could
   not resolve `go`. Neither touched product authority. The corrected SQL and
   absolute Go-path replay produced the accepted gates above.

## Deterministic Client Gates

The real-client evidence is complemented by deterministic acceptance:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 \
  ./internal/runtime ./internal/webchat ./internal/mcpserver

pnpm -C integrations/openclaw test
pnpm -C integrations/openclaw typecheck
pnpm -C integrations/openclaw build
pnpm -C integrations/openclaw pack --dry-run
```

These tests prove the deterministic lifecycle and transport contracts. The
separate event streams, artifacts, and PostgreSQL ledgers prove that the
official Codex client also completed the real before/after-boundary trajectory.
