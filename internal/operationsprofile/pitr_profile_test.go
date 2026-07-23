package operationsprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"vermory/internal/authn"
	"vermory/internal/provider"
	vermoryruntime "vermory/internal/runtime"
	"vermory/internal/webchat"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgreSQLPITRProfile(t *testing.T) {
	config, err := loadProfileConfigFromEnv(os.Getenv)
	if errors.Is(err, errProfileDisabled) {
		t.Skip("VERMORY_HA_PITR_PROFILE=1 is required")
	}
	if err != nil {
		t.Fatal(err)
	}
	harness := newClusterHarness(t, config)
	report := newProfileReport(t, config)
	target := runHAFailoverPhase(t, harness, &report)
	runPITRPhase(t, harness, target, &report)
	assertCompletedPITRReport(t, report)
}

func TestPostgreSQLHAPITRProfile(t *testing.T) {
	config, err := loadProfileConfigFromEnv(os.Getenv)
	if errors.Is(err, errProfileDisabled) {
		t.Skip("VERMORY_HA_PITR_PROFILE=1 is required")
	}
	if err != nil {
		t.Fatal(err)
	}
	if replayCompleteReportIfPresent(t, config) {
		return
	}
	harness := newClusterHarness(t, config)
	report := newProfileReport(t, config)
	target := runHAFailoverPhase(t, harness, &report)
	runPITRPhase(t, harness, target, &report)
	assertCompletedPITRReport(t, report)
	writeProfileReport(t, config, report)
}

func runPITRPhase(t *testing.T, harness *clusterHarness, target targetState, report *Report) {
	t.Helper()
	ctx := context.Background()
	currentAdminURL := profileDatabaseURL("postgres", "", []int{harness.Standby.Port}, false)
	currentStore, err := vermoryruntime.OpenStore(ctx, currentAdminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(currentStore.Close)
	currentPool, err := pgxpool.New(ctx, currentAdminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(currentPool.Close)
	continuityID := profileMemoryContinuity(t, currentPool, target.FactBID)

	deleted, err := currentStore.CommitGovernedObservation(ctx, profileTenantA, continuityID, vermoryruntime.CommitObservationRequest{
		OperationID:    "i03-delete-fact-b",
		Kind:           vermoryruntime.ObservationKindForgetRequest,
		Content:        "Operator requested deletion after the T2 recovery target.",
		SourceRef:      "memory:" + target.FactBID,
		TargetMemoryID: target.FactBID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Memory.Status != "deleted" {
		t.Fatalf("T3 fact deletion did not complete: %#v", deleted)
	}
	if _, err := authn.RevokeToken(ctx, currentPool, authn.RevokeTokenRequest{OperationID: "i03-revoke-token-a-current", PublicID: target.TokenAPublicID}); err != nil {
		t.Fatal(err)
	}
	factC := commitProfileFact(t, currentStore, profileTenantA, continuityID, "i03-fact-c", "i03.fact.c", "I03 fact C exists only after the T2 recovery target.")
	assertCurrentT3T4State(t, currentPool, target, factC.Memory.MemoryID)
	harness.forceArchiveCurrentSegment(t, harness.Standby)

	recoveryStarted := time.Now()
	copyDirectoryTree(t, target.PITRBasePath, harness.PITR.DataDir)
	config, err := renderPITRConfig(harness.PITR.Port, harness.WALArchive, target.LSN)
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(harness.PITR.DataDir, "postgresql.conf"), config)
	if err := os.Remove(filepath.Join(harness.PITR.DataDir, "standby.signal")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harness.PITR.DataDir, "recovery.signal"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	harness.startCluster(t, &harness.PITR)
	report.PITR.DurationMS = time.Since(recoveryStarted).Milliseconds()
	report.Topology.RestoredSystemID = harness.systemIdentifier(t, harness.PITR)
	report.PITR.RestoredReplayLSN = harness.execSQL(t, harness.PITR, "SELECT pg_last_wal_replay_lsn()")
	if !profileReplayReachedTarget(t, harness, target.LSN) {
		t.Fatalf("PITR replay LSN %s did not reach target %s", report.PITR.RestoredReplayLSN, target.LSN)
	}

	restoredAdminURL := profileDatabaseURL("postgres", "", []int{harness.PITR.Port}, false)
	restoredStore, err := vermoryruntime.OpenStore(ctx, restoredAdminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restoredStore.Close)
	restoredPool, err := pgxpool.New(ctx, restoredAdminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restoredPool.Close)
	restoredFingerprint := profileAuthorityFingerprint(t, restoredPool)
	report.PITR.RestoredFingerprint = restoredFingerprint
	if restoredFingerprint != target.AuthorityFingerprint {
		t.Fatalf("restored authority differs from T2: restored=%s target=%s", restoredFingerprint, target.AuthorityFingerprint)
	}
	assertRestoredT2State(t, restoredPool, target, factC.Memory.MemoryID)
	report.PITR.HistoricalStateRestored = true
	report.HardGates["target_state_equal"] = true
	report.HardGates["t3_t4_excluded"] = true
	writeProfileCheckpoint(t, harness.Config.Root, *report, "pitr_recovered", map[string]string{
		"target_lsn":           target.LSN,
		"restored_replay_lsn":  report.PITR.RestoredReplayLSN,
		"restored_fingerprint": restoredFingerprint,
	})

	for _, tenantID := range []string{profileTenantA, profileTenantB} {
		for _, profileID := range []string{
			vermoryruntime.ProductionRetrievalProfileID,
			vermoryruntime.MigrationRetrievalProfileID,
			vermoryruntime.DimensionalMigrationRetrievalProfileID,
		} {
			if err := restoredStore.ResetVectorProjection(ctx, tenantID, profileID); err != nil {
				t.Fatal(err)
			}
		}
	}
	rebuilt, err := restoredStore.RebuildAllProjections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt < 3 {
		t.Fatalf("projection rebuild restored only %d active memories", rebuilt)
	}
	assertRestoredSearch(t, restoredStore, restoredPool, continuityID, target, factC.Memory.MemoryID)
	report.PITR.ProjectionRebuilt = true
	report.HardGates["projection_rebuilt"] = true
	writeProfileCheckpoint(t, harness.Config.Root, *report, "projection_rebuilt", map[string]string{
		"active_search_documents": int64String(rebuilt),
	})

	const runtimeRole = "vermory_i03_runtime"
	const runtimePassword = "vermory-i03-test-only"
	runtimeURL := profileDatabaseURL(runtimeRole, runtimePassword, []int{harness.PITR.Port}, true)
	runtimeStore, err := vermoryruntime.OpenStoreWithOptions(ctx, runtimeURL, vermoryruntime.StoreOptions{EnforceTenantContext: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimeStore.Close)
	if err := runtimeStore.ValidateRuntimeRole(ctx); err != nil {
		t.Fatal(err)
	}
	authPool, err := pgxpool.New(ctx, runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authPool.Close)
	authenticator := authn.NewPostgresAuthenticator(authPool)
	if principal, err := authenticator.Authenticate(ctx, target.TokenARaw); err != nil || principal.TenantID != profileTenantA {
		t.Fatalf("historically active token was not restored: principal=%#v err=%v", principal, err)
	}
	report.Security.HistoricalTokenInitiallyActive = true
	if _, err := authn.RevokeToken(ctx, restoredPool, authn.RevokeTokenRequest{OperationID: "i03-pitr-revoke-token-a", PublicID: target.TokenAPublicID}); err != nil {
		t.Fatal(err)
	}
	report.Security.HistoricalTokenRevoked = true
	restoredHandler := webchat.NewAuthenticatedHandler(runtimeStore, provider.Mock{Output: "accepted after PITR re-governance"}, "profile-mock", authenticator)
	oldStatus, oldReceipt := performProfileChat(t, restoredHandler, target.TokenARaw, "i03-pitr-old-token-request", "The restored historical token must be rejected.", 5*time.Second)
	if oldStatus != http.StatusUnauthorized || oldReceipt.ID != "" || oldReceipt.Status != "" {
		t.Fatalf("historical token was not rejected: status=%d receipt=%#v", oldStatus, oldReceipt)
	}
	report.Security.HistoricalTokenRejected = true
	newToken := issueProfileTokenWithRole(t, restoredPool, profileTenantA, "i03-pitr-new-token", "i03-recovery-operator", authn.RoleOperator)
	newStatus, newReceipt := performProfileChat(t, restoredHandler, newToken.Token.Reveal(), "i03-pitr-new-token-request", "Resume service only after credential re-governance.", 10*time.Second)
	if newStatus != http.StatusOK || newReceipt.Status != vermoryruntime.ChatTurnCompleted || newReceipt.ID == "" {
		t.Fatalf("new token did not restore authenticated service: status=%d receipt=%#v", newStatus, newReceipt)
	}
	report.Security.NewTokenAccepted = true
	report.Security.CurrentStateReconciled = true
	report.Security.RLSPolicies = profileRLSPolicyCount(t, restoredPool)
	report.Security.TenantForeignKeys = profileTenantForeignKeyCount(t, restoredPool)
	report.HardGates["credentials_regoverned"] = true
	report.HardGates["historical_token_quarantined"] = true
	report.HardGates["restored_runtime_role_valid"] = true
	writeProfileCheckpoint(t, harness.Config.Root, *report, "credentials_regoverned", map[string]string{
		"historical_token_rejected": "true",
		"new_token_accepted":        "true",
	})

	entries, bytes := profileArchiveInventory(t, harness.WALArchive)
	report.PITR.BaseBackupBytes = profileTreeBytes(t, harness.PITRBase)
	report.PITR.WALArchiveBytes = bytes
	report.PITR.WALInventorySHA256 = InventoryDigest(entries)
	if report.PITR.BaseBackupBytes <= 0 || report.PITR.WALArchiveBytes <= 0 || !isLowerHex(report.PITR.WALInventorySHA256, 64) {
		t.Fatalf("physical backup inventory is incomplete: %#v", report.PITR)
	}
	report.HardGates["wal_inventory_complete"] = true
	report.HardGates["pitr_target_reached"] = true
	report.CompletedAt = time.Now().UTC()
}

func assertCompletedPITRReport(t *testing.T, report Report) {
	t.Helper()
	if report.PITR.TargetLSN == "" || report.PITR.RestoredReplayLSN == "" || report.PITR.T2Fingerprint == "" || report.PITR.T2Fingerprint != report.PITR.RestoredFingerprint {
		t.Fatalf("PITR target evidence is incomplete: %#v", report.PITR)
	}
	if !report.PITR.HistoricalStateRestored || !report.PITR.ProjectionRebuilt {
		t.Fatalf("PITR lifecycle gates did not pass: %#v", report.PITR)
	}
	if !report.Security.HistoricalTokenInitiallyActive || !report.Security.HistoricalTokenRevoked || !report.Security.HistoricalTokenRejected || !report.Security.NewTokenAccepted || !report.Security.CurrentStateReconciled {
		t.Fatalf("credential re-governance is incomplete: %#v", report.Security)
	}
	for _, gate := range []string{"target_state_equal", "t3_t4_excluded", "projection_rebuilt", "credentials_regoverned"} {
		if !report.HardGates[gate] {
			t.Fatalf("hard gate %q did not pass: %#v", gate, report.HardGates)
		}
	}
	if err := ValidateReport(report); err != nil {
		t.Fatalf("completed PITR report is invalid: %v", err)
	}
}

func replayCompleteReportIfPresent(t *testing.T, config profileConfig) bool {
	t.Helper()
	path := filepath.Join(config.Root, "artifacts", config.RunID, "report.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode completed report replay: %v", err)
	}
	expected := newProfileReport(t, config)
	if report.RequestFingerprint != expected.RequestFingerprint {
		t.Fatalf("completed report fingerprint conflicts with this request: got=%s want=%s", report.RequestFingerprint, expected.RequestFingerprint)
	}
	if err := ValidateReport(report); err != nil {
		t.Fatalf("completed report replay is invalid: %v", err)
	}
	t.Logf("replayed completed HA/PITR report %s", path)
	return true
}

func writeProfileReport(t *testing.T, config profileConfig, report Report) {
	t.Helper()
	paths, replayed, err := WriteReport(filepath.Join(config.Root, "artifacts"), report)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first combined profile report write was unexpectedly replayed")
	}
	t.Logf("HA/PITR report JSON=%s Markdown=%s", paths.JSON, paths.Markdown)
}

func assertCurrentT3T4State(t *testing.T, pool *pgxpool.Pool, target targetState, factCID string) {
	t.Helper()
	status, content := profileMemoryState(t, pool, target.FactBID)
	if status != "deleted" || content != "[redacted]" {
		t.Fatalf("current primary did not retain T3 deletion: status=%s content=%q", status, content)
	}
	status, _ = profileMemoryState(t, pool, factCID)
	if status != "active" {
		t.Fatalf("current primary did not retain T4 fact C: status=%s", status)
	}
	inspection, err := authn.InspectToken(context.Background(), pool, target.TokenAPublicID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Status != authn.TokenStatusRevoked {
		t.Fatalf("current primary token A status=%s, want revoked", inspection.Status)
	}
}

func assertRestoredT2State(t *testing.T, pool *pgxpool.Pool, target targetState, factCID string) {
	t.Helper()
	for _, memoryID := range []string{target.FactAID, target.FactBID} {
		status, content := profileMemoryState(t, pool, memoryID)
		if status != "active" || content == "[redacted]" {
			t.Fatalf("target fact %s was not restored active: status=%s content=%q", memoryID, status, content)
		}
	}
	if profileMemoryRows(t, pool, factCID) != 0 {
		t.Fatalf("post-target fact C %s appeared in PITR authority", factCID)
	}
	inspection, err := authn.InspectToken(context.Background(), pool, target.TokenAPublicID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Status != authn.TokenStatusActive {
		t.Fatalf("historical token status=%s, want active at T2", inspection.Status)
	}
	if profileObservationRows(t, pool, "i03-delete-fact-b") != 0 || profileTokenRevokeRows(t, pool, "i03-revoke-token-a-current") != 0 {
		t.Fatal("post-target deletion or revocation operation appeared in PITR authority")
	}
}

func assertRestoredSearch(t *testing.T, store *vermoryruntime.Store, pool *pgxpool.Pool, continuityID string, target targetState, factCID string) {
	t.Helper()
	exact, err := store.SearchActiveMemory(context.Background(), profileTenantA, continuityID, "I03 fact B is active at the exact T2 recovery target.", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !profileSearchContains(exact, target.FactBID) {
		t.Fatalf("exact restored fact B search missed target: %#v", exact)
	}
	paraphrased, err := store.SearchActiveMemory(context.Background(), profileTenantA, continuityID, "exact T2 recovery target active fact", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !profileSearchContains(paraphrased, target.FactBID) {
		t.Fatalf("paraphrased restored fact B search missed target: %#v", paraphrased)
	}
	if profileSearchDocumentRows(t, pool, factCID) != 0 {
		t.Fatal("post-target fact C became searchable after projection rebuild")
	}
}

func profileSearchContains(memories []vermoryruntime.Memory, memoryID string) bool {
	for _, memory := range memories {
		if memory.ID == memoryID {
			return true
		}
	}
	return false
}

func profileReplayReachedTarget(t *testing.T, harness *clusterHarness, targetLSN string) bool {
	t.Helper()
	return harness.execSQL(t, harness.PITR, "SELECT COALESCE(pg_last_wal_replay_lsn() >= '"+targetLSN+"'::pg_lsn, false)") == "t"
}

func profileMemoryContinuity(t *testing.T, pool *pgxpool.Pool, memoryID string) string {
	t.Helper()
	var continuityID string
	if err := pool.QueryRow(context.Background(), `SELECT continuity_id::text FROM governed_memories WHERE id = $1::uuid`, memoryID).Scan(&continuityID); err != nil {
		t.Fatal(err)
	}
	return continuityID
}

func profileMemoryState(t *testing.T, pool *pgxpool.Pool, memoryID string) (string, string) {
	t.Helper()
	var status, content string
	if err := pool.QueryRow(context.Background(), `SELECT lifecycle_status, content FROM governed_memories WHERE id = $1::uuid`, memoryID).Scan(&status, &content); err != nil {
		t.Fatal(err)
	}
	return status, content
}

func profileMemoryRows(t *testing.T, pool *pgxpool.Pool, memoryID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM governed_memories WHERE id = $1::uuid`, memoryID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func profileSearchDocumentRows(t *testing.T, pool *pgxpool.Pool, memoryID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_search_documents WHERE memory_id = $1::uuid`, memoryID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func profileObservationRows(t *testing.T, pool *pgxpool.Pool, operationID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM observations WHERE operation_id = $1`, operationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func profileTokenRevokeRows(t *testing.T, pool *pgxpool.Pool, operationID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM vermory_auth.api_tokens WHERE revoke_operation_id = $1`, operationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func issueProfileTokenWithRole(t *testing.T, pool *pgxpool.Pool, tenantID, operationID, subjectID string, role authn.Role) authn.IssueTokenReceipt {
	t.Helper()
	receipt, err := authn.IssueToken(context.Background(), pool, authn.IssueTokenRequest{
		OperationID: operationID,
		TenantID:    tenantID,
		SubjectID:   subjectID,
		Role:        role,
		ExpiresAt:   time.Now().UTC().Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Token.Reveal() == "" || receipt.Token.PublicID() == "" {
		t.Fatal("issued profile token omitted raw material")
	}
	return receipt
}

func copyDirectoryTree(t *testing.T, source, target string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, destination)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputErr := input.Close()
		if copyErr != nil {
			output.Close()
			return copyErr
		}
		if inputErr != nil {
			output.Close()
			return inputErr
		}
		return output.Close()
	}); err != nil {
		t.Fatalf("copy PITR base: %v", err)
	}
}

func profileTreeBytes(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return total
}

func profileArchiveInventory(t *testing.T, root string) ([]ArchiveEntry, int64) {
	t.Helper()
	entries := make([]ArchiveEntry, 0)
	var total int64
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, err := io.Copy(hash, file)
		file.Close()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, ArchiveEntry{Path: filepath.ToSlash(relative), Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))})
		total += size
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, total
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}
