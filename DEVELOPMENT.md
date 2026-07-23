# Vermory Development Guide

## Toolchain

The protected CI currently uses:

- Go `1.25.7` from `go.mod`;
- PostgreSQL `18` with the pinned pgvector service image;
- Node.js `24` and pnpm `11.12.0` for OpenClaw;
- uv `0.11.28` and Python `3.12` for Hermes tests;
- GoReleaser `2.17.0` for snapshots;
- Cosign `3.0.6` for keyless release-manifest signing.

Use a compatible local toolchain, but treat CI as the clean-environment
authority. Do not commit local virtual environments, package stores, generated
artifacts, provider output, or credentials.

## First Checkout

```bash
go mod download
pnpm -C integrations/openclaw install --frozen-lockfile
UV_PROJECT_ENVIRONMENT=/tmp/vermory-hermes \
  uv sync --project integrations/hermes --locked --python 3.12
```

Do not create a Hermes `.venv` inside the repository. Do not put real provider
keys in `.env` files that may be copied into evidence.

## Test Levels

Run the narrowest affected tests while developing. Before opening or updating
a pull request, run the repository gates below.

```bash
bash scripts/repository-policy.sh
bash -n scripts/release-manifest.sh
go test -p 1 -count=1 ./...
go test -count=1 -race ./internal/reality
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
PYTHONDONTWRITEBYTECODE=1 \
UV_PROJECT_ENVIRONMENT=/tmp/vermory-hermes-test \
  uv run --project integrations/hermes --locked --python 3.12 \
  python -m unittest discover -s integrations/hermes/tests -v
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
git diff --check
```

Database-backed tests use an isolated database through
`VERMORY_TEST_DATABASE_URL`. Never point tests at a production or shared
personal database.

```bash
export VERMORY_TEST_DATABASE_URL='postgresql://postgres:postgres@127.0.0.1:5432/vermory_test?sslmode=disable'
go test -p 1 -count=1 ./...
```

The full protected runtime race set is defined in
[`.github/workflows/ci.yml`](.github/workflows/ci.yml). Keep this guide and the
workflow aligned when the gate changes.

## Reality-First Change Flow

Use the following path when a change claims a new product capability or changes
a hard boundary:

```text
real failure or authorized trajectory
-> freeze source, expected behavior, and forbidden behavior
-> add deterministic checks and relevant simple baselines
-> implement the smallest end-to-end mechanism
-> run through a real client when the claim is client-facing
-> retain failures and negative controls
-> publish evidence with explicit non-claims
```

Small refactors, typo fixes, and test-only maintenance do not need a new reality
case, but they still must not weaken an existing case or inflate an evidence
claim.

## Database And Migration Rules

- PostgreSQL remains the only native semantic authority.
- Migrations are forward, reviewable, and safe to replay on a fresh database.
- Every schema change includes tests for migration order and affected lifecycle
  behavior.
- Projection data must remain rebuildable from authority.
- A destructive migration requires an ADR, a backup/restore plan, and explicit
  maintainer approval.
- Never reuse production credentials or include database dumps with private
  content in a public issue or pull request.

## Provider And Client Rules

- Providers and clients are compatibility targets, not a ranking exercise.
- Model output may propose information but cannot grant authority to itself.
- Real-client evidence names the exact client, model, version, permissions, and
  failure mode.
- A failed Grok, Codex, cursor-agent, OpenClaw, or Hermes run remains a failure;
  another client may add separate evidence but may not silently replace it.
- Gemini CLI is retired and is not an active client target.
- Public qualification must not route through a private NewAPI gateway or store
  provider secrets in artifacts.

## Branches And Commits

- Branch from the current protected `main` unless continuing an accepted branch.
- Prefer `type/short-description` names such as `feat/workspace-rebind` or
  `docs/collaboration-policy`.
- Keep commits reviewable and use concise imperative subjects such as
  `feat:`, `fix:`, `test:`, `docs:`, `ci:`, or `chore:`.
- Do not amend or rewrite commits another contributor may already be reviewing.
- Do not mix unrelated cleanup into a capability or evidence change.

## Release Boundary

Pull requests build signed, expiring snapshots. They do not publish releases.
Only an explicit `v*` tag may create a draft GitHub Release, and publication
requires a maintainer decision after protected checks and artifact verification.

See [Repository Workflow](docs/collaboration/repository-workflow.md) for review
and merge policy.
