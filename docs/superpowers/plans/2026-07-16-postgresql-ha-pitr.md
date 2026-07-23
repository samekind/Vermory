# PostgreSQL HA And PITR Qualification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Qualify Vermory against PostgreSQL 18 streaming-replication failover and exact-LSN point-in-time recovery while preserving authority, RLS, failure receipts, projection rebuild, and post-recovery credential governance.

**Architecture:** Freeze one synthetic operations trajectory, then add an opt-in external-package profile harness that creates only dedicated PostgreSQL clusters and exercises the existing runtime through a multi-host `pgxpool`. PostgreSQL remains the only authority; process orchestration stays test-owned, and committed evidence contains only non-secret reports and snapshots.

**Tech Stack:** Go 1.25.7+, PostgreSQL 18.4 native tools, pgx v5/pgxpool, goose migrations, native streaming replication, WAL archiving, `pg_basebackup`, Ed25519-independent deterministic evidence, GitHub Actions protected delivery.

## Global Constraints

- Use branch `agent/grok-cli-runtime`; never implement this work on `main`.
- Do not use subagents unless a later task becomes genuinely independent.
- Create and stop only dedicated clusters under `VERMORY_HA_PITR_ROOT`.
- Require `VERMORY_HA_PITR_PROFILE=1` before any test starts PostgreSQL processes.
- Require PostgreSQL server and client tools to report major version 18.
- Never stop, reconfigure, copy, or inspect the user's existing PostgreSQL data directory.
- Keep PostgreSQL authoritative; projections remain disposable.
- Do not print or commit raw API tokens, passwords, connection strings, process logs, or data-directory contents.
- Keep W15 scores and the production lexical default unchanged.
- Treat PITR-restored deletions and token revocations as historical state that requires quarantine and re-governance, not as current production truth.
- Keep the overall Vermory goal active after W16.

---

### Task 1: Freeze The I03 HA/PITR Reality Case

**Files:**
- Create: `reality/cases/I03-postgresql-ha-pitr/manifest.json`
- Create: `reality/cases/I03-postgresql-ha-pitr/events.jsonl`
- Create: `reality/cases/I03-postgresql-ha-pitr/fixtures/ha-pitr-contract.md`
- Create: `reality/cases/I03-postgresql-ha-pitr/fixture-lock.json`
- Modify: `internal/reality/validate_test.go`
- Modify: `internal/reality/experiment0.go`
- Modify: `internal/reality/experiment0_test.go`

**Interfaces:**
- Consumes: the version-1 reality case schema and `FreezeCase` fixture-lock contract.
- Produces: a byte-frozen `I03-postgresql-ha-pitr` case with explicit HA, PITR, historical-state, and credential-governance pressures.

- [x] **Step 1: Write the failing I03 validation test**

Append a test beside the I02 check:

```go
func TestI03PostgreSQLHAPITRCaseIsFrozen(t *testing.T) {
    c, err := LoadCase("../../reality/cases/I03-postgresql-ha-pitr")
    if err != nil {
        t.Fatal(err)
    }
    if got := ValidateCase(c); len(got) != 0 {
        t.Fatalf("expected valid I03 case, got violations: %#v", got)
    }
    requireStrings(t, c.Manifest.Pressures,
        "physical_streaming_replication",
        "primary_immediate_failure",
        "standby_promotion",
        "multi_host_runtime_reconnect",
        "wal_archive",
        "pitr_target_lsn",
        "historical_deletion_resurrection",
        "historical_token_resurrection",
        "post_recovery_projection_rebuild",
        "post_recovery_credential_governance",
    )
    for _, source := range c.Manifest.Sources {
        data, err := os.ReadFile(filepath.Join("../../reality/cases/I03-postgresql-ha-pitr", source.FixturePath))
        if err != nil {
            t.Fatal(err)
        }
        lower := strings.ToLower(string(data))
        for _, forbidden := range []string{"vmt_", "sk-", "password=", "postgresql://", "/users/", "/volumes/"} {
            if strings.Contains(lower, forbidden) {
                t.Fatalf("fixture %s contains forbidden value %q", source.FixturePath, forbidden)
            }
        }
    }
}
```

- [x] **Step 2: Verify RED**

```bash
go test -count=1 ./internal/reality -run 'TestI03PostgreSQLHAPITRCaseIsFrozen' -v
```

Expected: FAIL because `I03-postgresql-ha-pitr` does not exist.

- [x] **Step 3: Create the frozen case**

The manifest must use:

```json
{
  "version": 1,
  "id": "I03-postgresql-ha-pitr",
  "title": "PostgreSQL streaming failover and exact-LSN historical recovery",
  "evidence_level": "public",
  "continuity_lines": ["security"],
  "pressures": [
    "physical_streaming_replication",
    "primary_immediate_failure",
    "standby_promotion",
    "multi_host_runtime_reconnect",
    "wal_archive",
    "pitr_target_lsn",
    "historical_deletion_resurrection",
    "historical_token_resurrection",
    "post_recovery_projection_rebuild",
    "post_recovery_credential_governance"
  ]
}
```

The fixture and events must freeze the T0-T4 timeline from the design, require
one transition failure with no receipt, and explicitly forbid treating a PITR
target as automatically reconciled current state.

- [x] **Step 4: Freeze fixtures and register the hypothesis signal**

Run the existing freeze command:

```bash
go run ./cmd/vermory reality-freeze \
  --case-dir reality/cases/I03-postgresql-ha-pitr
```

Add I03 to the Experiment 0 `H-014` operations signal without changing other
case counts or converting it to sealed evidence.

- [x] **Step 5: Verify GREEN and commit**

```bash
go test -count=1 ./internal/reality
git diff --check
git add reality/cases/I03-postgresql-ha-pitr internal/reality
git commit -m "test: freeze PostgreSQL HA and PITR case"
```

---

### Task 2: Add Deterministic HA/PITR Report Contracts

**Files:**
- Create: `internal/operationsprofile/report.go`
- Create: `internal/operationsprofile/report_test.go`

**Interfaces:**
- Produces: `operationsprofile.Report`, `operationsprofile.Checkpoint`, `operationsprofile.WriteCheckpoint(root string, checkpoint Checkpoint) error`, `operationsprofile.WriteReport(root string, report Report) (ArtifactPaths, bool, error)`, and `operationsprofile.InventoryDigest(entries []ArchiveEntry) string`.
- Consumes: only standard-library JSON, hashing, sorting, file, and time packages.

- [x] **Step 1: Write failing report and replay tests**

Use this public shape:

```go
type Report struct {
    Version                int               `json:"version"`
    RunID                  string            `json:"run_id"`
    ImplementationRevision string            `json:"implementation_revision"`
    RequestFingerprint     string            `json:"request_fingerprint"`
    PostgreSQLVersion      string            `json:"postgresql_version"`
    StartedAt              time.Time         `json:"started_at"`
    CompletedAt            time.Time         `json:"completed_at"`
    Topology               TopologyReport    `json:"topology"`
    Failover               FailoverReport    `json:"failover"`
    PITR                   PITRReport        `json:"pitr"`
    Security               SecurityReport    `json:"security"`
    HardGates              map[string]bool   `json:"hard_gates"`
    Failures               []FailureRecord   `json:"failures"`
    NonClaims              []string          `json:"non_claims"`
}
```

Tests must require:

```go
func TestWriteReportCreatesDeterministicArtifactsAndReplays(t *testing.T)
func TestWriteReportRejectsConflictingRunIDReuse(t *testing.T)
func TestWriteCheckpointIsAtomicAndRejectsFingerprintDrift(t *testing.T)
func TestInventoryDigestSortsPathsAndHashesBytes(t *testing.T)
func TestValidateReportRejectsSecretShapedFieldsAndFailedHardGate(t *testing.T)
```

The Markdown report must contain `# PostgreSQL HA And PITR Qualification`, the
run ID, implementation revision, target LSN, failover duration, PITR duration,
every failed attempt, hard-gate status, and the historical-state quarantine
warning.

- [x] **Step 2: Verify RED**

```bash
go test -count=1 ./internal/operationsprofile -run 'TestWriteReport|TestInventoryDigest|TestValidateReport' -v
```

Expected: FAIL because `internal/operationsprofile` does not exist.

- [x] **Step 3: Implement atomic deterministic report writing**

`WriteReport` must write through a temporary file followed by `os.Rename`, sort
map-derived output before Markdown generation, calculate a canonical request
fingerprint excluding timestamps, and return `replayed=true` only when existing
JSON is byte-equivalent to the new report after canonical encoding.

`WriteCheckpoint` stores one JSON file per phase under
`<root>/checkpoints/<phase>.json`. A checkpoint contains run ID, request
fingerprint, phase, terminal status, non-secret measurements, and failures.
Rewriting the same phase with a different request fingerprint or terminal
payload is rejected. Intermediate checkpoints support diagnosis; only a
matching complete final report permits zero-process run-level replay.

Reject any report string containing:

```text
postgresql://
password=
vmt_
sk-
authorization:
```

Reject a report with a false hard gate. Preserve `Failures` even when a retry
later succeeds.

- [x] **Step 4: Verify GREEN and commit**

```bash
go test -count=1 ./internal/operationsprofile
go test -race -count=1 ./internal/operationsprofile
git diff --check
git add internal/operationsprofile
git commit -m "feat: add HA and PITR evidence reports"
```

---

### Task 3: Build The Dedicated PostgreSQL 18 Cluster Harness

**Files:**
- Create: `internal/operationsprofile/postgres_cluster_test.go`
- Create: `internal/operationsprofile/postgres_cluster_helpers_test.go`

**Interfaces:**
- Produces test-only `clusterHarness`, `postgresCluster`, and `profileConfig` helpers.
- Consumes `VERMORY_HA_PITR_PROFILE`, `VERMORY_POSTGRES_BIN_DIR`, and `VERMORY_HA_PITR_ROOT`.

- [x] **Step 1: Write failing harness boundary tests**

```go
func TestLoadProfileConfigRequiresExplicitOptIn(t *testing.T)
func TestLoadProfileConfigRequiresPostgreSQL18Tools(t *testing.T)
func TestClusterPathsRemainInsideDedicatedRoot(t *testing.T)
func TestRenderPrimaryConfigUsesLoopbackAndDedicatedArchive(t *testing.T)
func TestRenderStandbyAndPITRConfigsContainExactRecoveryBoundary(t *testing.T)
```

The test config loader returns `errProfileDisabled` unless the opt-in value is
exactly `1`. It rejects `/`, an existing PostgreSQL service data directory, a
root containing symlink escapes, or a bin directory whose `postgres --version`
or `pg_basebackup --version` is not 18.x.

- [x] **Step 2: Verify RED**

```bash
go test -count=1 ./internal/operationsprofile -run 'TestLoadProfileConfig|TestClusterPaths|TestRender' -v
```

- [x] **Step 3: Implement minimal cluster lifecycle helpers**

Define:

```go
type profileConfig struct {
    Enabled bool
    BinDir  string
    Root    string
    RunID   string
}

type postgresCluster struct {
    Name       string
    DataDir    string
    LogPath    string
    Port       int
    ProcessUp  bool
}

type clusterHarness struct {
    Config     profileConfig
    Primary    postgresCluster
    Standby    postgresCluster
    PITRBase   string
    PITR       postgresCluster
    WALArchive string
}
```

Implement wrappers for `initdb`, `pg_ctl start/stop/promote`,
`pg_basebackup`, `psql`, `pg_controldata`, free-port allocation, bounded SQL
polling, path containment, and cleanup. Every `exec.CommandContext` error must
return command name, exit code, and a redacted tail of output without DSNs.

Every cluster must use loopback TCP only with `unix_socket_directories=''` so
the profile does not depend on platform Unix socket path limits. Primary
configuration must also include `wal_level`,
`max_wal_senders`, `max_replication_slots`, `hot_standby`, `archive_mode`, and
an archive command targeting the dedicated archive directory. The helper must
use `pg_ctl stop -m immediate` only for the dedicated primary.

- [x] **Step 4: Run a cluster-only smoke profile**

```bash
VERMORY_HA_PITR_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR=/opt/homebrew/opt/postgresql@18/bin \
VERMORY_HA_PITR_ROOT=/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr/w16-harness-smoke \
go test -count=1 ./internal/operationsprofile -run 'TestPostgreSQLClusterHarnessSmoke' -v
```

Expected: primary starts, a streamed standby catches up, both system
identifiers match, and cleanup leaves no listener on the allocated ports.

- [x] **Step 5: Commit**

```bash
go test -count=1 ./internal/operationsprofile
git diff --check
git add internal/operationsprofile
git commit -m "test: add dedicated PostgreSQL replication harness"
```

---

### Task 4: Prove Multi-Host Runtime Failover

**Files:**
- Create: `internal/operationsprofile/ha_failover_profile_test.go`
- Modify: `internal/operationsprofile/report.go`
- Modify: `internal/operationsprofile/report_test.go`

**Interfaces:**
- Consumes: Task 3 cluster harness, `runtime.OpenStore`, `runtime.OpenStoreWithOptions`, `authn.GrantRuntimeRole`, `authn.IssueToken`, `webchat.NewAuthenticatedHandler`, and `provider.Mock`.
- Produces: `runHAFailoverPhase(t *testing.T, harness *clusterHarness, report *Report) targetState`, the `FailoverReport` section, and hard gates for replay catch-up, no false receipt, same-pool reconnect, RLS, and exact-once persistence.

- [x] **Step 1: Write the failing failover profile assertions**

The profile must build one handler and retain it:

```go
runtimeStore, err := runtime.OpenStoreWithOptions(ctx, multiHostRuntimeURL, runtime.StoreOptions{EnforceTenantContext: true})
if err != nil { t.Fatal(err) }
authPool, err := pgxpool.New(ctx, multiHostRuntimeURL)
if err != nil { t.Fatal(err) }
handler := webchat.NewAuthenticatedHandler(
    runtimeStore,
    provider.Mock{Output: "accepted after failover"},
    "profile-mock",
    authn.NewPostgresAuthenticator(authPool),
)
```

Freeze these operation IDs:

```text
i03-pre-failover
i03-transition-failure
i03-post-promotion
```

The test must assert the handler, runtime store, and auth pool pointers remain
the same before and after promotion.

- [x] **Step 2: Verify RED**

```bash
VERMORY_HA_PITR_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR=/opt/homebrew/opt/postgresql@18/bin \
VERMORY_HA_PITR_ROOT=/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr/w16-ha-red \
go test -count=1 ./internal/operationsprofile -run 'TestPostgreSQLHAFailoverProfile' -v
```

Expected: FAIL before the profile orchestration exists.

- [x] **Step 3: Implement the HA trajectory**

The test must:

1. initialize primary and migrate to schema 15;
2. create a local replication role and restricted runtime role;
3. seed fact A, token A, and one revoked control token;
4. create PITR base backup and streaming standby;
5. commit fact B and authenticated turn `i03-pre-failover`;
6. wait for standby replay LSN to reach the primary flush LSN;
7. issue the post-target token used for HA;
8. force a WAL switch and wait for archive visibility;
9. stop primary with immediate mode;
10. call the unchanged handler with `i03-transition-failure` and require a
    bounded error with zero receipt/rows;
11. promote standby and wait for read-write state;
12. retry through the same handler until `i03-post-promotion` succeeds;
13. require pre-failover and post-promotion rows exactly once, transition rows
    zero, token controls correct, RLS policies present, and cross-tenant access
    absent.

Write atomic checkpoints after `cluster_initialized`,
`replication_caught_up`, and `standby_promoted`.

Keep the reusable phase entry point exact:

```go
func runHAFailoverPhase(t *testing.T, harness *clusterHarness, report *Report) targetState
```

It returns the T2 target LSN, authority fingerprint, fact IDs, token-A public
ID/raw value held only in process memory, and the completed PITR base path.

- [x] **Step 4: Verify GREEN and commit**

```bash
VERMORY_HA_PITR_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR=/opt/homebrew/opt/postgresql@18/bin \
VERMORY_HA_PITR_ROOT=/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr/w16-ha-green \
go test -count=1 ./internal/operationsprofile -run 'TestPostgreSQLHAFailoverProfile' -v
git diff --check
git add internal/operationsprofile
git commit -m "test: prove PostgreSQL standby failover"
```

---

### Task 5: Prove Exact-LSN PITR And Re-Governance

**Files:**
- Create: `internal/operationsprofile/pitr_profile_test.go`
- Modify: `internal/operationsprofile/report.go`
- Modify: `internal/operationsprofile/report_test.go`

**Interfaces:**
- Consumes: the Task 3 base-backup and WAL helpers, the T2 LSN/fingerprint captured by the profile, and existing governance, projection, token, authentication, and RLS APIs.
- Produces: `runPITRPhase(t *testing.T, harness *clusterHarness, target targetState, report *Report)`, `TestPostgreSQLHAPITRProfile`, the `PITRReport` and `SecurityReport` sections, and hard gates for target-state equivalence and quarantine exit.

- [x] **Step 1: Write the failing PITR assertions**

Define the target timeline in the test:

```go
type targetState struct {
    LSN                  string
    AuthorityFingerprint string
    FactAID              string
    FactBID              string
    TokenAPublicID       string
    TokenARaw            string
    PITRBasePath         string
}
```

Assertions must require:

```text
T2: fact A active, fact B active, token A active, fact C absent
T3/T4 current primary: fact B deleted, token A revoked, fact C active
PITR target: exact T2 state restored
quarantine exit: token A revoked again, new token works, old token returns 401
```

- [x] **Step 2: Verify RED**

```bash
VERMORY_HA_PITR_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR=/opt/homebrew/opt/postgresql@18/bin \
VERMORY_HA_PITR_ROOT=/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr/w16-pitr-red \
go test -count=1 ./internal/operationsprofile -run 'TestPostgreSQLPITRProfile' -v
```

- [x] **Step 3: Implement exact target recovery**

The test must copy the completed `pitr-base` backup into a fresh restore data
directory, append:

```text
restore_command = 'cp <dedicated-wal-archive>/%f %p'
recovery_target_lsn = '<captured-T2-LSN>'
recovery_target_inclusive = on
recovery_target_action = promote
```

and create `recovery.signal`. Start the restored cluster on its own port and
wait for `pg_is_in_recovery() = false`.

Before any service acceptance, compare the restored targeted authority rows to
the T2 fingerprint, assert T3/T4 absence, reset all disposable vector/search
projection rows, run `RebuildAllProjections`, and verify exact plus paraphrased
search behavior.

Then revoke token A using a new idempotent operation, verify the old raw token
returns `401`, issue a new operator token, and verify an authenticated request
succeeds. Set `historical_state_restored=true` before this phase and
`current_state_reconciled=true` only after all credential/projection gates pass.

Write atomic checkpoints after `target_lsn_captured`, `pitr_recovered`,
`projection_rebuilt`, and `credentials_regoverned`.

Add the final combined profile without duplicating the HA/PITR implementation:

```go
func TestPostgreSQLHAPITRProfile(t *testing.T) {
    config := loadEnabledProfileConfig(t)
    if replayed := replayCompleteReportIfPresent(t, config); replayed {
        return
    }
    harness := newClusterHarness(t, config)
    report := newProfileReport(config)
    target := runHAFailoverPhase(t, harness, &report)
    runPITRPhase(t, harness, target, &report)
    assertAllHardGates(t, report)
    writeProfileReport(t, config, report)
}
```

`TestPostgreSQLPITRProfile` may call the same phase helpers for focused
development, but the formal evidence command uses the combined test above.

- [x] **Step 4: Verify GREEN and commit**

```bash
VERMORY_HA_PITR_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR=/opt/homebrew/opt/postgresql@18/bin \
VERMORY_HA_PITR_ROOT=/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr/w16-pitr-green \
go test -count=1 ./internal/operationsprofile -run 'TestPostgreSQLPITRProfile' -v
git diff --check
git add internal/operationsprofile
git commit -m "test: prove exact LSN point in time recovery"
```

---

### Task 6: Run The Combined Profile And Commit Evidence

**Files:**
- Create: `docs/evidence/2026-07-16-postgresql-ha-pitr.md`
- Create: `docs/evidence/snapshots/2026-07-16-postgresql-ha-pitr.json`
- Modify: `docs/integrations/identity-authorization-rls.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/evaluation-matrix.md`

**Interfaces:**
- Consumes: Tasks 1-5, implementation revision, dedicated JSData profile root, and report writer.
- Produces: one failure-preserving real profile report, operator runbook updates, and committed non-secret evidence.

- [x] **Step 1: Build the exact evidence binary and run the combined profile**

Use a dedicated root and stable run ID:

```bash
VERMORY_HA_PITR_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR=/opt/homebrew/opt/postgresql@18/bin \
VERMORY_HA_PITR_ROOT=/Volumes/JSData/ComputerScience/Mac/.vermory-ha-pitr/postgresql-ha-pitr-20260716-v1 \
VERMORY_HA_PITR_RUN_ID=postgresql-ha-pitr-20260716-v1 \
VERMORY_HA_PITR_IMPLEMENTATION_REVISION="$(git rev-parse HEAD)" \
go test -count=1 ./internal/operationsprofile -run 'TestPostgreSQLHAPITRProfile' -v
```

Do not rerun to erase a failure. If the run fails, retain its report and assign
a new run ID only after the defect is fixed.

- [x] **Step 2: Audit the raw profile without exposing logs**

Check:

```text
PostgreSQL 18.x
matching primary/standby system identifiers before promotion
standby replay >= pre-failover flush LSN
zero transition-failure rows and receipts
same handler/store/auth-pool identity before and after promotion
target LSN reached
T2 fingerprint matched
T3/T4 excluded from PITR
projection rebuilt from restored authority
historical token rejected after re-governance
new token authenticated
all clusters stopped
```

Only inspect PostgreSQL logs through filtered error/status lines. Do not commit
logs or raw profile directories.

- [x] **Step 3: Write evidence and operator guidance**

The evidence must state:

- same-host processes are not cross-host HA evidence;
- measured timings are not SLOs;
- no automatic leader election or split-brain prevention is provided;
- PITR can restore historically active facts and credentials;
- restored instances remain quarantined until projections and credentials are
  reconciled;
- W15 retrieval quality remains unchanged and unresolved.

The runbook must show multi-host DSN syntax, standby promotion checks, exact-LSN
recovery, projection rebuild, token revocation/reissue, and safe traffic
reenablement order without including real DSNs.

- [x] **Step 4: Verify and commit evidence**

```bash
jq empty docs/evidence/snapshots/2026-07-16-postgresql-ha-pitr.json
go test -count=1 ./internal/reality ./internal/operationsprofile
git diff --check
git add docs/evidence docs/integrations/identity-authorization-rls.md README.md README.zh-CN.md docs/evaluation-matrix.md
git commit -m "docs: record PostgreSQL HA and PITR evidence"
```

---

### Task 7: Protected Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-16-postgresql-ha-pitr.md`
- Modify: Draft PR 1 body

- [x] **Step 1: Run all local gates**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/operationsprofile ./internal/runtime ./internal/authn ./internal/webchat ./cmd/vermory ./internal/reality
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -trimpath ./cmd/vermory
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
go run github.com/goreleaser/goreleaser/v2@v2.17.0 check --config .goreleaser.yaml
go run github.com/goreleaser/goreleaser/v2@v2.17.0 release --snapshot --clean --skip=publish --config .goreleaser.yaml
git diff --check
```

- [x] **Step 2: Commit the checked local-gate state and push the evidence head**

```bash
git add docs/superpowers/plans/2026-07-16-postgresql-ha-pitr.md
git commit -m "docs: record PostgreSQL HA and PITR local gates"
git push origin agent/grok-cli-runtime
```

Append W16 results, failures, non-claims, and evidence paths to Draft PR 1. Keep
the PR Draft and do not create a tag or Release.

- [x] **Step 3: Independently verify evidence-head CI and artifact**

Require:

```text
test=SUCCESS
PR OPEN / Draft / CLEAN / MERGEABLE
GitHub transport ZIP size and SHA-256 match artifact API
all four archive checksums and 4-entry layouts pass
OpenClaw package has exactly 12 entries
Darwin arm64 executes version and the relevant operations help command
GitHub synthetic merge has a valid verification signature
synthetic merge second parent equals the pushed evidence head
zero tags
zero Releases
```

- [x] **Step 4: Close W16 on a final protected checklist head**

Mark all W16 items, commit:

```bash
git add docs/superpowers/plans/2026-07-16-postgresql-ha-pitr.md
git commit -m "docs: close PostgreSQL HA and PITR qualification"
git push origin agent/grok-cli-runtime
```

Require a second protected CI and independent artifact verification. Append
only final immutable run, job, artifact, digest, merge, and PR-state IDs to the
PR body so no third documentation commit is created.

- [x] **Step 5: Keep the overall platform goal active**

W16 closes same-host PostgreSQL streaming failover and exact-LSN PITR only.
The overall goal remains active for genuine external sealed evaluation,
long-duration retention, active-backlog dimensional migration, artifact
signing, cross-host HA evidence, and final release acceptance.
