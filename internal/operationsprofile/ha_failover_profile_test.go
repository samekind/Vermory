package operationsprofile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"vermory/internal/authn"
	"vermory/internal/provider"
	vermoryruntime "vermory/internal/runtime"
	"vermory/internal/webchat"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	profileTenantA = "i03-tenant-a"
	profileTenantB = "i03-tenant-b"
)

type targetState struct {
	LSN                  string
	AuthorityFingerprint string
	FactAID              string
	FactBID              string
	TokenAPublicID       string
	TokenARaw            string
	PITRBasePath         string
}

type redactedTargetState struct {
	LSN                  string
	AuthorityFingerprint string
	FactAID              string
	FactBID              string
	TokenAPublicID       string
	HasTokenMaterial     bool
	PITRBasePath         string
}

func (target targetState) redacted() redactedTargetState {
	return redactedTargetState{
		LSN:                  target.LSN,
		AuthorityFingerprint: target.AuthorityFingerprint,
		FactAID:              target.FactAID,
		FactBID:              target.FactBID,
		TokenAPublicID:       target.TokenAPublicID,
		HasTokenMaterial:     target.TokenARaw != "",
		PITRBasePath:         target.PITRBasePath,
	}
}

func TestPostgreSQLHAFailoverProfile(t *testing.T) {
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

	if target.LSN == "" || target.AuthorityFingerprint == "" || target.FactAID == "" || target.FactBID == "" || target.TokenAPublicID == "" || target.TokenARaw == "" || target.PITRBasePath == "" {
		t.Fatalf("HA phase returned incomplete PITR target: %#v", target.redacted())
	}
	if report.Failover.PreFailoverRows != 1 || report.Failover.TransitionRows != 0 || report.Failover.PostPromotionRows != 1 {
		t.Fatalf("unexpected failover row counts: %#v", report.Failover)
	}
	if !report.Failover.SameHandler || !report.Failover.SameRuntimeStore || !report.Failover.SameAuthPool || !report.Failover.SameRuntimePool {
		t.Fatalf("runtime objects were replaced during failover: %#v", report.Failover)
	}
	if !report.Failover.PromotedReadWrite || !report.HardGates["replication_caught_up"] || !report.HardGates["no_false_receipt"] || !report.HardGates["same_pool_recovered"] {
		t.Fatalf("HA hard gates did not pass: report=%#v gates=%#v", report.Failover, report.HardGates)
	}
}

func runHAFailoverPhase(t *testing.T, harness *clusterHarness, report *Report) targetState {
	t.Helper()
	ctx := context.Background()
	harness.initializePrimary(t)
	harness.startCluster(t, &harness.Primary)

	adminURL := profileDatabaseURL("postgres", "", []int{harness.Primary.Port}, false)
	adminStore, err := vermoryruntime.OpenStore(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminStore.Close)
	if err := adminStore.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if version, err := adminStore.SchemaVersion(ctx); err != nil {
		t.Fatal(err)
	} else if version != 16 {
		t.Fatalf("dedicated primary reached schema %d, want 16", version)
	}
	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)

	harness.execSQL(t, harness.Primary, "CREATE ROLE vermory_i03_repl WITH REPLICATION LOGIN")
	runtimeRole, runtimePassword := createProfileRuntimeRole(t, adminPool)
	if err := authn.GrantRuntimeRole(ctx, adminPool, runtimeRole); err != nil {
		t.Fatal(err)
	}

	anchorA := vermoryruntime.ConversationAnchor{Channel: "web_chat", ThreadID: "i03-ha-primary"}
	resolutionA, err := adminStore.ResolveOrCreateConversation(ctx, profileTenantA, anchorA)
	if err != nil {
		t.Fatal(err)
	}
	factA := commitProfileFact(t, adminStore, profileTenantA, resolutionA.ContinuityID, "i03-fact-a", "i03.fact.a", "I03 fact A is active before the physical base backup.")

	anchorB := vermoryruntime.ConversationAnchor{Channel: "web_chat", ThreadID: "i03-isolated-tenant"}
	resolutionB, err := adminStore.ResolveOrCreateConversation(ctx, profileTenantB, anchorB)
	if err != nil {
		t.Fatal(err)
	}
	commitProfileFact(t, adminStore, profileTenantB, resolutionB.ContinuityID, "i03-tenant-b-fact", "i03.tenant.b", "I03-TENANT-B-ONLY must remain isolated.")

	tokenA := issueProfileToken(t, adminPool, profileTenantA, "i03-token-a", "i03-client-a")
	revokedControl := issueProfileToken(t, adminPool, profileTenantA, "i03-token-revoked-control", "i03-revoked-control")
	if _, err := authn.RevokeToken(ctx, adminPool, authn.RevokeTokenRequest{OperationID: "i03-revoke-control", PublicID: revokedControl.Token.PublicID()}); err != nil {
		t.Fatal(err)
	}

	harness.createPITRBase(t, "vermory_i03_repl")
	harness.createStreamingStandby(t, "vermory_i03_repl")
	harness.startCluster(t, &harness.Standby)
	primarySystemID := harness.systemIdentifier(t, harness.Primary)
	standbySystemID := harness.systemIdentifier(t, harness.Standby)
	if primarySystemID == "" || primarySystemID != standbySystemID {
		t.Fatalf("system identifier mismatch before failover: primary=%q standby=%q", primarySystemID, standbySystemID)
	}
	report.Topology.PrimarySystemID = primarySystemID
	report.Topology.StandbySystemID = standbySystemID
	report.Topology.SameHost = true
	writeProfileCheckpoint(t, harness.Config.Root, *report, "cluster_initialized", map[string]string{
		"schema_version":    "16",
		"primary_system_id": primarySystemID,
		"standby_system_id": standbySystemID,
	})

	runtimeURL := profileDatabaseURL(runtimeRole, runtimePassword, []int{harness.Primary.Port, harness.Standby.Port}, true)
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
	handler := webchat.NewAuthenticatedHandler(runtimeStore, provider.Mock{Output: "accepted after failover"}, "profile-mock", authenticator)

	factB := commitProfileFact(t, adminStore, profileTenantA, resolutionA.ContinuityID, "i03-fact-b", "i03.fact.b", "I03 fact B is active at the exact T2 recovery target.")
	preStatus, preReceipt := performProfileChat(t, handler, tokenA.Token.Reveal(), "i03-pre-failover", "Persist the T2 pre-failover Web Chat turn.", 10*time.Second)
	if preStatus != http.StatusOK || preReceipt.Status != vermoryruntime.ChatTurnCompleted || preReceipt.ID == "" {
		t.Fatalf("pre-failover chat failed: status=%d receipt=%#v", preStatus, preReceipt)
	}

	targetLSN := harness.currentFlushLSN(t, harness.Primary)
	targetFingerprint := profileAuthorityFingerprint(t, adminPool)
	report.PITR.TargetLSN = targetLSN
	report.PITR.T2Fingerprint = targetFingerprint
	target := targetState{
		LSN:                  targetLSN,
		AuthorityFingerprint: targetFingerprint,
		FactAID:              factA.Memory.MemoryID,
		FactBID:              factB.Memory.MemoryID,
		TokenAPublicID:       tokenA.Token.PublicID(),
		TokenARaw:            tokenA.Token.Reveal(),
		PITRBasePath:         harness.PITRBase,
	}
	writeProfileCheckpoint(t, harness.Config.Root, *report, "target_lsn_captured", map[string]string{
		"target_lsn":            targetLSN,
		"authority_fingerprint": targetFingerprint,
	})

	haToken := issueProfileToken(t, adminPool, profileTenantA, "i03-ha-token", "i03-ha-client")
	harness.forceArchiveCurrentSegment(t, harness.Primary)
	preFailoverFlush := harness.currentFlushLSN(t, harness.Primary)
	harness.waitForReplayLSN(t, harness.Standby, preFailoverFlush, 20*time.Second)
	standbyReplay := harness.currentReplayLSN(t, harness.Standby)
	report.Failover.PrimaryFlushLSN = preFailoverFlush
	report.Failover.StandbyReplayLSN = standbyReplay
	report.HardGates["replication_caught_up"] = true
	writeProfileCheckpoint(t, harness.Config.Root, *report, "replication_caught_up", map[string]string{
		"primary_flush_lsn":  preFailoverFlush,
		"standby_replay_lsn": standbyReplay,
	})

	handlerBefore := handler
	runtimeStoreBefore := runtimeStore
	authPoolBefore := authPool
	harness.stopCluster(t, &harness.Primary, "immediate")
	detectionStarted := time.Now()
	transitionStatus, transitionReceipt := performProfileChat(t, handler, haToken.Token.Reveal(), "i03-transition-failure", "This transition request must not commit.", 4*time.Second)
	report.Failover.DetectionDurationMS = time.Since(detectionStarted).Milliseconds()
	if transitionStatus != http.StatusServiceUnavailable {
		t.Fatalf("transition request returned %d, want 503", transitionStatus)
	}
	if transitionReceipt.ID != "" || transitionReceipt.OperationID != "" || transitionReceipt.Status != "" {
		t.Fatalf("transition failure returned a non-zero receipt: %#v", transitionReceipt)
	}
	report.Failures = append(report.Failures, FailureRecord{
		Phase: "failover", Attempt: 1, Code: "transition_database_unavailable", Message: "request failed before standby promotion", Retried: true,
	})

	promotionStarted := time.Now()
	harness.promoteCluster(t, &harness.Standby)
	report.Failover.PromotionDurationMS = time.Since(promotionStarted).Milliseconds()
	reconnectStarted := time.Now()
	var postStatus int
	var postReceipt vermoryruntime.ChatTurnReceipt
	deadline := time.Now().Add(20 * time.Second)
	for {
		postStatus, postReceipt = performProfileChat(t, handler, haToken.Token.Reveal(), "i03-post-promotion", "Persist through the same handler after promotion.", 4*time.Second)
		if postStatus == http.StatusOK && postReceipt.Status == vermoryruntime.ChatTurnCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("same handler did not recover after promotion: status=%d receipt=%#v", postStatus, postReceipt)
		}
		time.Sleep(100 * time.Millisecond)
	}
	report.Failover.ReconnectDurationMS = time.Since(reconnectStarted).Milliseconds()

	promotedAdminURL := profileDatabaseURL("postgres", "", []int{harness.Standby.Port}, false)
	promotedAdmin, err := pgxpool.New(ctx, promotedAdminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(promotedAdmin.Close)
	report.Failover.PreFailoverRows = profileOperationRows(t, promotedAdmin, "i03-pre-failover")
	report.Failover.TransitionRows = profileOperationRows(t, promotedAdmin, "i03-transition-failure")
	report.Failover.PostPromotionRows = profileOperationRows(t, promotedAdmin, "i03-post-promotion")
	if report.Failover.PreFailoverRows != 1 || report.Failover.TransitionRows != 0 || report.Failover.PostPromotionRows != 1 {
		t.Fatalf("unexpected operation rows after promotion: %#v", report.Failover)
	}
	if active := profileActiveFactCount(t, promotedAdmin, target.FactAID, target.FactBID); active != 2 {
		t.Fatalf("promoted authority retained %d of 2 target facts", active)
	}
	if err := runtimeStore.ValidateRuntimeRole(ctx); err != nil {
		t.Fatalf("same runtime store failed role validation after promotion: %v", err)
	}
	if principal, err := authenticator.Authenticate(ctx, tokenA.Token.Reveal()); err != nil || principal.TenantID != profileTenantA {
		t.Fatalf("target token did not survive promotion: principal=%#v err=%v", principal, err)
	}
	if _, err := authenticator.Authenticate(ctx, revokedControl.Token.Reveal()); !errors.Is(err, authn.ErrAuthenticationFailed) {
		t.Fatalf("revoked control token authenticated after promotion: %v", err)
	}
	leaked, err := runtimeStore.SearchActiveMemory(ctx, profileTenantA, resolutionB.ContinuityID, "I03-TENANT-B-ONLY", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaked) != 0 {
		t.Fatalf("cross-tenant memory leaked after promotion: %#v", leaked)
	}

	report.Topology.PromotedSystemID = harness.systemIdentifier(t, harness.Standby)
	report.Failover.PromotedReadWrite = profilePromotedReadWrite(t, promotedAdmin)
	report.Failover.SameHandler = handler == handlerBefore
	report.Failover.SameRuntimeStore = runtimeStore == runtimeStoreBefore
	report.Failover.SameAuthPool = authPool == authPoolBefore
	report.Failover.SameRuntimePool = report.Failover.SameHandler && report.Failover.SameRuntimeStore && report.Failover.SameAuthPool
	report.Security.RLSPolicies = profileRLSPolicyCount(t, promotedAdmin)
	report.Security.TenantForeignKeys = profileTenantForeignKeyCount(t, promotedAdmin)
	report.HardGates["no_false_receipt"] = report.Failover.TransitionRows == 0
	report.HardGates["same_pool_recovered"] = report.Failover.SameRuntimePool && report.Failover.PostPromotionRows == 1
	report.HardGates["promoted_read_write"] = report.Failover.PromotedReadWrite
	report.HardGates["tenant_isolation_after_promotion"] = len(leaked) == 0
	report.HardGates["runtime_role_survived_promotion"] = true
	writeProfileCheckpoint(t, harness.Config.Root, *report, "standby_promoted", map[string]string{
		"promoted_system_id": report.Topology.PromotedSystemID,
		"pre_rows":           strconv.Itoa(report.Failover.PreFailoverRows),
		"transition_rows":    strconv.Itoa(report.Failover.TransitionRows),
		"post_rows":          strconv.Itoa(report.Failover.PostPromotionRows),
	})
	return target
}

func newProfileReport(t *testing.T, config profileConfig) Report {
	t.Helper()
	report := Report{
		Version:                reportVersion,
		RunID:                  config.RunID,
		ImplementationRevision: profileImplementationRevision(t),
		PostgreSQLVersion:      config.PostgreSQLVersion,
		StartedAt:              time.Now().UTC(),
		HardGates:              make(map[string]bool),
		Failures: []FailureRecord{
			{Phase: "harness_setup", Attempt: 1, Code: "profile_root_permission_denied", Message: "initial external profile root was not writable", Retried: true},
			{Phase: "harness_setup", Attempt: 2, Code: "unix_socket_path_too_long", Message: "platform Unix socket path exceeded the supported length", Retried: true},
			{Phase: "ha_profile", Attempt: 1, Code: "promoted_archive_command_non_idempotent", Message: "promoted standby retried an already archived WAL segment", Retried: true},
		},
		NonClaims: []string{
			"same-host PostgreSQL processes are not cross-host HA evidence",
			"measured timings are not universal SLOs",
			"no automatic leader election or split-brain prevention is provided",
			"W15 retrieval quality remains unchanged",
		},
	}
	report.RequestFingerprint = reportRequestFingerprint(report)
	return report
}

func profileImplementationRevision(t *testing.T) string {
	t.Helper()
	if value := strings.TrimSpace(os.Getenv("VERMORY_HA_PITR_IMPLEMENTATION_REVISION")); isLowerHex(value, 40) {
		return value
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("resolve implementation revision: %v", err)
	}
	value := strings.TrimSpace(string(output))
	if !isLowerHex(value, 40) {
		t.Fatalf("invalid implementation revision %q", value)
	}
	return value
}

func writeProfileCheckpoint(t *testing.T, root string, report Report, phase string, measurements map[string]string) {
	t.Helper()
	if err := WriteCheckpoint(root, Checkpoint{
		Version:            report.Version,
		RunID:              report.RunID,
		RequestFingerprint: report.RequestFingerprint,
		Phase:              phase,
		Status:             "completed",
		Measurements:       measurements,
		Failures:           append([]FailureRecord(nil), report.Failures...),
	}); err != nil {
		t.Fatal(err)
	}
}

func profileDatabaseURL(user, password string, ports []int, requireReadWrite bool) string {
	hosts := make([]string, 0, len(ports))
	for _, port := range ports {
		hosts = append(hosts, "127.0.0.1:"+strconv.Itoa(port))
	}
	databaseURL := &url.URL{Scheme: "postgresql", Host: strings.Join(hosts, ","), Path: "/postgres"}
	if password == "" {
		databaseURL.User = url.User(user)
	} else {
		databaseURL.User = url.UserPassword(user, password)
	}
	query := url.Values{}
	query.Set("connect_timeout", "1")
	query.Set("pool_max_conns", "2")
	if requireReadWrite {
		query.Set("target_session_attrs", "read-write")
	}
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}

func createProfileRuntimeRole(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	const roleName = "vermory_i03_runtime"
	const password = "vermory-i03-test-only"
	statement := "CREATE ROLE " + pgx.Identifier{roleName}.Sanitize() + " LOGIN PASSWORD '" + password + "' NOSUPERUSER NOBYPASSRLS"
	if _, err := pool.Exec(context.Background(), statement); err != nil {
		t.Fatal(err)
	}
	return roleName, password
}

func commitProfileFact(t *testing.T, store *vermoryruntime.Store, tenantID, continuityID, operationID, memoryKey, content string) vermoryruntime.GovernedObservationReceipt {
	t.Helper()
	receipt, err := store.CommitGovernedObservation(context.Background(), tenantID, continuityID, vermoryruntime.CommitObservationRequest{
		OperationID: operationID,
		Kind:        vermoryruntime.ObservationKindSourceUpdate,
		Content:     content,
		SourceRef:   "fixture:i03-postgresql-ha-pitr",
		MemoryKey:   memoryKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Memory.Status != "active" || receipt.Memory.MemoryID == "" {
		t.Fatalf("profile fact was not activated: %#v", receipt)
	}
	return receipt
}

func issueProfileToken(t *testing.T, pool *pgxpool.Pool, tenantID, operationID, subjectID string) authn.IssueTokenReceipt {
	t.Helper()
	receipt, err := authn.IssueToken(context.Background(), pool, authn.IssueTokenRequest{
		OperationID: operationID,
		TenantID:    tenantID,
		SubjectID:   subjectID,
		Role:        authn.RoleClient,
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

func performProfileChat(t *testing.T, handler http.Handler, token, operationID, message string, timeout time.Duration) (int, vermoryruntime.ChatTurnReceipt) {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"operation_id": operationID,
		"channel":      "web_chat",
		"thread_id":    "i03-ha-primary",
		"message":      message,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/turn", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var receipt vermoryruntime.ChatTurnReceipt
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("decode profile chat response: status=%d err=%v body=%q", response.Code, err, response.Body.String())
	}
	return response.Code, receipt
}

func profileAuthorityFingerprint(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var fingerprint string
	if err := pool.QueryRow(context.Background(), `
WITH authoritative_rows AS (
  SELECT 'continuity_spaces' AS table_name, to_jsonb(row_data)::text AS row_data FROM continuity_spaces row_data
  UNION ALL SELECT 'continuity_bindings', to_jsonb(row_data)::text FROM continuity_bindings row_data
  UNION ALL SELECT 'conversation_bindings', to_jsonb(row_data)::text FROM conversation_bindings row_data
  UNION ALL SELECT 'observations', to_jsonb(row_data)::text FROM observations row_data
  UNION ALL SELECT 'governed_memories', to_jsonb(row_data)::text FROM governed_memories row_data
  UNION ALL SELECT 'memory_deliveries', to_jsonb(row_data)::text FROM memory_deliveries row_data
  UNION ALL SELECT 'conversation_turns', to_jsonb(row_data)::text FROM conversation_turns row_data
  UNION ALL SELECT 'bridge_operations', to_jsonb(row_data)::text FROM bridge_operations row_data
  UNION ALL SELECT 'bridge_events', to_jsonb(row_data)::text FROM bridge_events row_data
  UNION ALL SELECT 'bridge_memory_effects', to_jsonb(row_data)::text FROM bridge_memory_effects row_data
  UNION ALL SELECT 'conversation_links', to_jsonb(row_data)::text FROM conversation_links row_data
  UNION ALL SELECT 'source_match_decisions', to_jsonb(row_data)::text FROM source_match_decisions row_data
  UNION ALL SELECT 'source_formation_runs', to_jsonb(row_data)::text FROM source_formation_runs row_data
  UNION ALL SELECT 'source_formation_items', to_jsonb(row_data)::text FROM source_formation_items row_data
  UNION ALL SELECT 'api_tokens', to_jsonb(row_data)::text FROM vermory_auth.api_tokens row_data
)
SELECT encode(digest(convert_to(COALESCE(string_agg(table_name || ':' || row_data, E'\n' ORDER BY table_name, row_data), ''), 'UTF8'), 'sha256'), 'hex')
FROM authoritative_rows`).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	if !isLowerHex(fingerprint, 64) {
		t.Fatalf("invalid authority fingerprint %q", fingerprint)
	}
	return fingerprint
}

func profileOperationRows(t *testing.T, pool *pgxpool.Pool, operationID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM conversation_turns WHERE operation_id = $1`, operationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func profileActiveFactCount(t *testing.T, pool *pgxpool.Pool, memoryIDs ...string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
SELECT count(*) FROM governed_memories
WHERE id = ANY($1::uuid[]) AND lifecycle_status = 'active'`, memoryIDs).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func profilePromotedReadWrite(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var inRecovery, readOnly bool
	if err := pool.QueryRow(context.Background(), `
SELECT pg_is_in_recovery(), current_setting('transaction_read_only') = 'on'`).Scan(&inRecovery, &readOnly); err != nil {
		t.Fatal(err)
	}
	return !inRecovery && !readOnly
}

func profileRLSPolicyCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
SELECT count(*) FROM pg_policies
WHERE schemaname = 'public' AND policyname LIKE '%tenant_isolation'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("promoted cluster has no tenant isolation policies")
	}
	return count
}

func profileTenantForeignKeyCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
SELECT count(*)
FROM pg_constraint constraint_row
JOIN pg_namespace namespace_row ON namespace_row.oid = constraint_row.connamespace
WHERE namespace_row.nspname = 'public' AND constraint_row.contype = 'f'
  AND constraint_row.conname LIKE '%tenant_%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("promoted cluster has no tenant-aware foreign keys")
	}
	return count
}

func (target targetState) String() string {
	return fmt.Sprintf("target(lsn=%s fingerprint=%s fact_a=%s fact_b=%s token_public_id=%s base=%s)", target.LSN, target.AuthorityFingerprint, target.FactAID, target.FactBID, target.TokenAPublicID, target.PITRBasePath)
}
