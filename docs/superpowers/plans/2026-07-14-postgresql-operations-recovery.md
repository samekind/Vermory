# PostgreSQL Operations And Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verify migration replay, native PostgreSQL backup/restore, projection recovery, database failure behavior, and Linux amd64/arm64 release execution for Vermory's authenticated runtime.

**Architecture:** Keep PostgreSQL authoritative and use native `pg_dump`/`pg_restore` for backup portability. Add deterministic runtime tests and an operator runbook rather than a second backup service; use cross-compiled static Go artifacts plus an available Linux runtime for architecture evidence.

**Tech Stack:** Go 1.26, PostgreSQL 18 test runtime with PostgreSQL 16+ SQL, goose, pgx v5, `pg_dump`, `pg_restore`, Colima/Lima when available, Linux ELF binaries.

## Global Constraints

- `serve` never runs migrations.
- Admin and runtime PostgreSQL identities remain separate.
- PostgreSQL is the only authoritative state.
- Raw token secrets never enter PostgreSQL, backup artifacts, evidence, or Git.
- Search projections are disposable and rebuildable.
- Database-mutating tests run serially with `-p 1`.
- No Gemini CLI and no Mac mini NewAPI route.
- Do not delete non-dedicated user data or stop unrelated services.

---

### Task 1: Freeze Operations Reality Case And Deterministic Contracts

**Files:**
- Create: `reality/cases/I02-postgresql-operations-recovery/manifest.json`
- Create: `reality/cases/I02-postgresql-operations-recovery/events.jsonl`
- Create: `reality/cases/I02-postgresql-operations-recovery/fixtures/recovery-contract.md`
- Create: `reality/cases/I02-postgresql-operations-recovery/fixture-lock.json`
- Modify: `internal/reality/validate_test.go`
- Modify: `internal/reality/experiment0.go`
- Modify: `internal/reality/experiment0_test.go`

**Interfaces:**
- Consumes: existing reality case schema and current I01 authenticated case.
- Produces: immutable migration, dump/restore, projection-loss, outage, and Linux portability pressures.

- [x] **Step 1: Write the failing I02 case validation**

Require pressures named `migration_replay`, `backup_restore`, `projection_rebuild`, `runtime_role_reprovision`, `database_outage`, `linux_amd64`, and `linux_arm64`; require no credential-shaped fixture values.

- [x] **Step 2: Verify RED**

```bash
go test -count=1 ./internal/reality -run 'Test.*I02'
```

Expected: FAIL because I02 and its fixture lock do not exist.

- [x] **Step 3: Freeze hashes and validate**

```bash
go test -count=1 ./internal/reality
git diff --check
```

- [x] **Step 4: Commit**

```bash
git add reality/cases/I02-postgresql-operations-recovery internal/reality
git commit -m "test: freeze PostgreSQL operations recovery case"
```

### Task 2: Add Runtime Recovery Acceptance

**Files:**
- Create: `internal/runtime/operations_acceptance_test.go`
- Modify: `internal/runtime/postgres_store.go`

**Interfaces:**
- Consumes: schema 9, `OpenStoreWithOptions`, token lifecycle, `RebuildProjection`, and runtime role grants.
- Produces: deterministic migration replay, authoritative fingerprint, projection rebuild, and failure-recovery assertions.

- [x] **Step 1: Write failing recovery assertions**

Cover:

```text
Migrate -> Migrate is idempotent at version 9
seed active/superseded/deleted memories and a token
delete memory_search_documents -> rebuild projection -> active recall returns the same eligible set
deleted content remains absent after rebuild
runtime role validation survives reopen
database connection failure returns no successful receipt
new connection after recovery can authenticate and query its tenant
```

- [x] **Step 2: Verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestOperationsRecovery' -v
```

- [x] **Step 3: Implement only required recovery hooks**

Do not add a custom backup format. Keep `Migrate`, `ResetForTest`, and projection rebuild explicit; add only an internal fingerprint helper if tests need stable comparison.

- [x] **Step 4: Verify GREEN and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime
git diff --check
git add internal/runtime
git commit -m "test: prove runtime recovery invariants"
```

### Task 3: Execute Native Backup/Restore And Write Runbook

**Files:**
- Create: `docs/evidence/2026-07-14-postgresql-operations-recovery.md`
- Modify: `docs/integrations/identity-authorization-rls.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: a dedicated PostgreSQL source database, `pg_dump`, `pg_restore`, current runtime binary, and Task 2 assertions.
- Produces: safe restore transcript, source/target fingerprints, token/RLS/projection checks, and operator instructions.

- [x] **Step 1: Prepare dedicated source and target databases**

Use names under `vermory_ops_i02_*` only. Create separate admin/runtime roles, apply migrations through `database migrate`, and never use the repository test database as the restore target.

- [x] **Step 2: Capture custom dump**

```bash
pg_dump --format=custom --no-owner --no-acl --file=/tmp/vermory-ops-i02.dump "$SOURCE_ADMIN_DATABASE_URL"
```

Do not print the DSN or dump contents. Record only command status, byte size, and SHA-256.

- [x] **Step 3: Restore into empty target and reprovision runtime role**

```bash
createdb --maintenance-db="$TARGET_ADMIN_MAINTENANCE_URL" "$TARGET_DATABASE_NAME"
pg_restore --exit-on-error --no-owner --no-acl --dbname="$TARGET_ADMIN_DATABASE_URL" /tmp/vermory-ops-i02.dump
./bin/vermory database grant-runtime --database-url "$TARGET_ADMIN_DATABASE_URL" --role vermory_ops_i02_runtime
```

- [x] **Step 4: Run source/target deterministic comparisons**

Compare migration version, authoritative row fingerprints, token status counts, RLS policy inventory, role attributes, projection rebuild counts, exact active recall, deleted-fact absence, and cross-tenant FK rejection.

- [x] **Step 5: Document backup sensitivity and removal boundary**

State that token digests are sensitive authentication material, raw secrets are unrecoverable, roles may require separate cluster bootstrap, and uninstall does not delete the database implicitly.

- [x] **Step 6: Commit evidence**

```bash
git diff --check
git add docs/evidence/2026-07-14-postgresql-operations-recovery.md docs/integrations/identity-authorization-rls.md README.md README.zh-CN.md
git commit -m "docs: record PostgreSQL recovery evidence"
```

### Task 4: Linux amd64/arm64 Portability Evidence

**Files:**
- Create: `docs/evidence/2026-07-14-linux-runtime-portability.md`
- Create: `artifacts/release/2026-07-14/manifest.json`

**Interfaces:**
- Consumes: `cmd/vermory`, current Go module, available Linux runtime, and `serve` configuration validation.
- Produces: architecture-specific ELF artifacts and startup evidence without credentials.

- [x] **Step 1: Build both artifacts**

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o artifacts/release/2026-07-14/vermory-linux-amd64 ./cmd/vermory
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o artifacts/release/2026-07-14/vermory-linux-arm64 ./cmd/vermory
file artifacts/release/2026-07-14/vermory-linux-amd64 artifacts/release/2026-07-14/vermory-linux-arm64
```

- [x] **Step 2: Run Linux startup probes**

Run `--help` and unsafe `serve` configuration probes on a real Linux runtime. If Colima/Lima emulation is used, record architecture and emulation explicitly.

- [x] **Step 3: Write checksummed manifest and evidence**

The manifest records only paths, architecture, size, SHA-256, Go version, kernel/runtime architecture, and exit status.

- [x] **Step 4: Commit**

```bash
git diff --check
git add docs/evidence/2026-07-14-linux-runtime-portability.md artifacts/release/2026-07-14/manifest.json
git commit -m "test: verify Linux runtime portability"
```

### Task 5: Operations Release Gate And Draft PR Update

**Files:**
- Modify: `docs/superpowers/plans/2026-07-14-postgresql-operations-recovery.md`
- Modify: `docs/evidence/2026-07-14-postgresql-operations-recovery.md`
- Modify: `docs/evidence/2026-07-14-linux-runtime-portability.md`

- [x] **Step 1: Run operations release verification**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/authn ./internal/runtime ./internal/webchat ./cmd/vermory
go vet ./...
git diff --exit-code -- go.mod go.sum
git diff --check
```

- [x] **Step 2: Mark only freshly verified checklist items**

Review the operations evidence for credentials, ambiguous claims, source/target mismatch, and missing failure statuses before marking completion.

- [x] **Step 3: Push and update Draft PR**

Push `agent/grok-cli-runtime`, preserve Draft state, and add the operations evidence and explicit remaining benchmark/sealed-client boundary to PR 1.
