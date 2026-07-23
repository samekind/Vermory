# Local Operator Workspace Governance

This guide runs the local trusted-governance path for a workspace continuity.
It is deliberately separate from MCP normal flow: an AI client can retrieve
governed context and write a proposed result, but it cannot confirm a
workspace, promote its own result, replace a fact, or delete a fact.

Use a dedicated disposable PostgreSQL database and synthetic facts. Do not put
real source text, credentials, personal paths, or chat history in this guide,
its command history, or Git artifacts.

## Build

From the repository root:

```bash
go build -o ./bin/vermory ./cmd/vermory
```

The operator commands write one JSON receipt to stdout. Errors and diagnostics
are written to stderr.

## Confirm And Inspect

Confirming a root is explicit. Repeating confirmation for the same normalized
root returns its existing continuity instead of creating another one.

```bash
./bin/vermory workspace inspect \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03

./bin/vermory workspace confirm \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03
```

The first command returns `{"status":"needs_confirmation",...}` and does
not create a continuity. The second returns `{"status":"resolved",...}`
with a `continuity_id`.

Confirmation only binds this exact root. A rename, worktree, mirror, or new
path is not inferred to be the same workspace; explicit rebind is a separate
bridge capability and is not part of this slice.

## Record, Propose, Revise, Correct, And Forget

Every mutation requires an operator-selected `operation_id`. Reusing the same
ID for a retry is idempotent. Keep the `memory_id` from each JSON receipt: it
is the only accepted target for source revision, user correction, or deletion.

```bash
./bin/vermory memory add-source \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id w03-source-v1 \
  --source-ref fixture:W03:release-notes-v1 \
  --content 'Use checkout_eta_v1 for the staged checkout release.'

./bin/vermory memory inspect \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03
```

Copy the active v1 `memory_id` from the inspect response into the trusted
source revision:

```bash
./bin/vermory memory revise-source \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id w03-source-v2 \
  --memory-id '<v1-memory-id>' \
  --source-ref fixture:W03:release-notes-v2 \
  --content 'Use checkout_eta_v2 for the staged checkout release.'
```

The source revision atomically supersedes only the named active fact and keeps
the replacement as a `source_update`. It does not use content similarity to
choose a target or replace other facts from the source. Use `memory correct`
instead when the authority is an explicit user correction rather than a new
trusted source version. Both operations require a named active target.

When a trusted ingestor has a stable fact key but should not activate source
changes automatically, record the current source with `--key` and submit the
new revision as a candidate:

```bash
./bin/vermory memory add-source \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id signing-v1 \
  --key release.signing.mode \
  --source-ref repo:deploy/production.yaml@sha-old \
  --content 'Production releases use a macOS keychain certificate.'

./bin/vermory memory propose-source \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id signing-candidate-v2 \
  --key release.signing.mode \
  --source-ref repo:deploy/production.yaml@sha-new \
  --content 'Production releases use GitHub Actions OIDC keyless signing.'
```

`propose-source` resolves only active facts with the same key in the same
tenant and confirmed workspace. Zero matches creates a new candidate, one
different match creates a replacement candidate, identical content records an
unchanged observation, and multiple active matches abstain. A proposed or
rejected candidate is visible to `memory inspect` but absent from normal search
and MCP context.

Review the returned `candidate_memory_id`, then make one explicit decision:

```bash
./bin/vermory memory accept-candidate \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id signing-accept-v2 \
  --memory-id '<candidate-memory-id>'

./bin/vermory memory reject-candidate \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id signing-reject-v2 \
  --memory-id '<candidate-memory-id>'
```

Acceptance atomically supersedes the still-current keyed target and activates
the candidate. Rejection preserves the candidate as audit history and changes
no active fact. This path relies on a trusted stable key; it does not extract
keys or infer general semantic conflicts from arbitrary documents.

Copy the returned v2 `memory_id` into the forget operation:

```bash
./bin/vermory memory forget \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id w03-forget-v2 \
  --memory-id '<v2-memory-id>'
```

`memory forget` intentionally accepts no free-text reason or content flag.
Its bounded observation is `Operator requested deletion.` so a deletion
request cannot repeat a sensitive fact in a new audit observation. The target
memory, its origin observation, and its lexical projection are redacted or
removed by the same authority transaction.

## Connect A Local Client

MCP remains a two-tool normal-flow server. For a temporary Grok CLI replay,
register a user-local server against this dedicated database:

```bash
ATTACHMENT="$(./bin/vermory workspace-attachment \
  --cwd "$PWD" \
  --filesystem-namespace workstation-alpha \
  --format base64)"

grok mcp add vermory-w03 -- "$(pwd)/bin/vermory" mcp-stdio \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --workspace-attachment "$ATTACHMENT"

grok mcp doctor vermory-w03 --json
```

A qualifying real-client replay must use the exact confirmed root, call
`prepare_context`, complete a narrow repository task, preserve its artifact
and verification output, then call `commit_observation` with the delivery
receipt. Its write-back must remain `proposed`. Preserve a redacted delivery
and observation ledger under ignored `artifacts/runtime/W03/`.

The Go tests in this repository prove the command and lifecycle contract, but
a qualifying client replay must still show the client tool calls and the
corresponding PostgreSQL ledger. A successful Codex replay is recorded in
[Codex MCP Real-Client Evidence](../evidence/2026-07-14-codex-mcp-real-client.md);
a keyed source-candidate rejection/acceptance and real Grok replay are recorded
in [Source Conflict Candidate Runtime Evidence](../evidence/2026-07-14-source-conflict-candidate-runtime.md);
a failed client attempt remains failure evidence and must not be replaced by a
scripted pass or a Grok result.

After a temporary replay, remove the user-local server if it is no longer
needed:

```bash
grok mcp remove vermory-w03
```
