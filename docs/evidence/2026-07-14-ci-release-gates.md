# CI Release Gates Evidence

Date: 2026-07-14

Implementation revision: `dc78bd68748374dda313d436b4e9d739bc0fed47`

## Scope

This evidence upgrades the pull-request workflow from a fast source-only Go
check to automatic gates for the runtime surfaces already claimed by the
repository. It does not create a public release or claim final release
readiness.

Before this change, GitHub Actions did not set
`VERMORY_TEST_DATABASE_URL`. PostgreSQL-dependent workspace, conversation,
identity, RLS, recovery, MCP, and operator tests therefore followed their
documented skip path. The workflow also did not install or test the OpenClaw
package and did not build the release binary.

## Enforced Pull-Request Gates

The single `test` job now uses:

```text
Ubuntu GitHub-hosted runner
PostgreSQL service image: pgvector/pgvector:pg18@sha256:12a379b47ad65289572ea0756efc11b7c241a6662833e8af7038cd3b73d647e0
Database: vermory_test
Go version: go.mod (1.25.7)
Node.js: 24
pnpm: 11.12.0
Job timeout: 30 minutes
```

| Gate | Command or mechanism |
|---|---|
| PostgreSQL readiness | Container health check with `pg_isready` |
| Full Go suite with database | `go test -p 1 -count=1 ./...` |
| Runtime race coverage | `go test -race -p 1 -count=1` across authn, runtime, webchat, identity CLI, operator CLI, MCP server, command, provider, memory-backend, and retrieval-ablation packages |
| Reality race coverage | `go test -count=1 -race ./internal/reality` |
| Static analysis | `go vet ./...` |
| Dependency drift | `go mod tidy` followed by zero `go.mod` / `go.sum` diff |
| Release build | `go build -trimpath -o /tmp/vermory ./cmd/vermory` |
| OpenClaw dependency integrity | `pnpm -C integrations/openclaw install --frozen-lockfile` |
| OpenClaw behavior and build | `pnpm -C integrations/openclaw check` |
| OpenClaw publish shape | `pnpm -C integrations/openclaw pack --dry-run` |
| Patch hygiene | `git diff --check` |

Package-level Go execution is intentionally serial because the integration
tests share one dedicated PostgreSQL database. This avoids turning database
test races into nondeterministic CI noise while preserving goroutine race
detection inside each tested package.

## Local Verification

The workflow syntax passed `actionlint 1.7.7`. The same full database suite,
runtime race package set, OpenClaw 43-test check, typecheck, build, package
dry-run, `go vet`, tidy check, release build, and diff check passed locally
before push.

## Remote Verification

GitHub Actions run
[`29299572273`](https://github.com/jstar0/Vermory/actions/runs/29299572273)
executed the workflow from revision `dc78bd6`.

```text
job: test
conclusion: success
duration: 2m35s
main steps completed: 16/16
```

The remote job reported success for container initialization, PostgreSQL-backed
tests, runtime race tests, reality race tests, vet, module verification,
release build, OpenClaw install/check/package, and clean-diff verification.
This is stronger evidence than the previous CI result because the database URL
was present and the PostgreSQL service was healthy before tests began.

## W08 Pgvector Gate Correction

W08 added database-backed native pgvector retrieval and outage tests. The first
remote run at revision `465f045587ae59f69fb9bf1a2ab97342d42a71db` used the
plain `postgres:18` service image. GitHub Actions run
[`29334199266`](https://github.com/jstar0/Vermory/actions/runs/29334199266), job
`87089262583`, failed because that image did not provide the `vector`
extension. The two native retrieval tests failed rather than being skipped:

```text
TestRunUsesNativePostgreSQLVectorBackend: failed
TestRunNativeVectorOutageDegradesToLexical: failed
root cause: extension "vector" is not available
```

Revision `51a484bd2e8fd57ce8f2f1d98cc81f03fb855c10` changed the service
to the immutable image:

```text
pgvector/pgvector:pg18@sha256:12a379b47ad65289572ea0756efc11b7c241a6662833e8af7038cd3b73d647e0
```

It also added `internal/memorybackend` and `internal/retrievalablation` to the
remote race set. GitHub Actions run
[`29334651225`](https://github.com/jstar0/Vermory/actions/runs/29334651225), job
`87090757056`, then completed successfully in `3m54s`. All 19 main steps
passed, including the full serial PostgreSQL/pgvector suite, expanded runtime
race tests, reality race, vet, module-drift check, release binary, OpenClaw
install/check/package, four-platform GoReleaser snapshot, artifact upload, and
clean-diff verification.

The successful run uploaded artifact `8311523895`, named
`vermory-pr-snapshot-5dde8dcdd7264888fcd9c4e2b5a9f373927cc56d`, with GitHub
digest
`sha256:238a173f91fa775b62f212251cbea0f67b757d4184a8f406c5be8fefe22f40af`.
This is pre-final delivery evidence; the final checklist head is required to
pass the same protected workflow and produce an independently verified
artifact before W08 delivery closes.

## Main Branch Enforcement

The public repository previously had no branch protection. After the clean
remote run, `main` was configured with this minimal single-maintainer policy:

```json
{
  "required_check": "test",
  "strict": true,
  "pull_request_required": true,
  "required_approving_reviews": 0,
  "required_conversation_resolution": true,
  "enforce_admins": false,
  "allow_force_pushes": false,
  "allow_deletions": false
}
```

The policy requires a branch to be current with `main` and the expanded `test`
job to pass before a normal merge. It does not require a second maintainer's
approval and does not prevent repository administrators from emergency
recovery. After protection was enabled, Draft PR 1 reported `CLEAN` and
`MERGEABLE` with the required `test` check completed successfully.

## Claim Boundary

This result proves that the current PR automatically executes the repository's
database-backed and OpenClaw release gates on a clean Ubuntu runner and that
normal `main` integration is protected by that check. It does not prove
production scale, a published GitHub Release, artifact signing, container
deployment, macOS/Windows portability, external sealed evaluation, or final
open-source release acceptance.
