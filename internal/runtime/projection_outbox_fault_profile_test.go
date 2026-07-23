package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vermory/internal/memorybackend"
)

type projectionOutboxFaultCase struct {
	Version              string   `json:"version"`
	ID                   string   `json:"id"`
	TenantID             string   `json:"tenant_id"`
	RealProviderTenantID string   `json:"real_provider_tenant_id"`
	RecordCount          int      `json:"record_count"`
	WorkerBatchSize      int      `json:"worker_batch_size"`
	ProfileID            string   `json:"profile_id"`
	HardGates            []string `json:"hard_gates"`
}

func TestProjectionOutboxFaultCaseIsFrozen(t *testing.T) {
	manifest := loadProjectionOutboxFaultCase(t)
	if manifest.Version != "1" || manifest.ID != "W11-projection-outbox-fault-profile" {
		t.Fatalf("unexpected W11 identity: %#v", manifest)
	}
	if manifest.RecordCount != 1000 || manifest.WorkerBatchSize != 128 {
		t.Fatalf("unexpected W11 load profile: %#v", manifest)
	}
	if manifest.ProfileID != ProductionRetrievalProfileID || len(manifest.HardGates) != 8 {
		t.Fatalf("unexpected W11 retrieval contract: %#v", manifest)
	}
}

func TestProjectionOutboxFaultProfile(t *testing.T) {
	if os.Getenv("VERMORY_OUTBOX_FAULT_PROFILE") != "1" {
		t.Skip("VERMORY_OUTBOX_FAULT_PROFILE=1 is required")
	}
	apiKey := strings.TrimSpace(os.Getenv("VERMORY_LIVE_EMBEDDING_API_KEY"))
	if apiKey == "" {
		t.Skip("VERMORY_LIVE_EMBEDDING_API_KEY is required")
	}
	manifest := loadProjectionOutboxFaultCase(t)
	cluster := startDisposablePostgres18(t)
	defer cluster.stop(t, "fast")

	ctx := context.Background()
	store, err := OpenStore(ctx, cluster.databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, manifest.TenantID, "/fixtures/w11/outbox")
	if err != nil {
		t.Fatal(err)
	}

	seedProjectionBacklog(t, store, manifest, continuityID)
	initialStatus, err := store.RetrievalProjectionStatus(ctx, manifest.TenantID, manifest.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if initialStatus.LastEventID != 0 || initialStatus.LatestEventID != int64(manifest.RecordCount) || initialStatus.Lag != int64(manifest.RecordCount) {
		t.Fatalf("initial backlog mismatch: %#v", initialStatus)
	}

	profile := productionRetrievalProfile(t)
	normalEmbedder := &projectionTestEmbedder{vector: testVector1024(0.25)}
	worker := mustProjectionWorker(t, store, normalEmbedder, manifest.TenantID, manifest.WorkerBatchSize)
	firstBatch, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if firstBatch.Processed != manifest.WorkerBatchSize || firstBatch.Lag != int64(manifest.RecordCount-manifest.WorkerBatchSize) {
		t.Fatalf("bounded first batch mismatch: %#v", firstBatch)
	}

	failingWorker := mustProjectionWorker(t, store, &projectionTestEmbedder{
		err: errors.New("provider unavailable"),
	}, manifest.TenantID, manifest.WorkerBatchSize)
	failed, err := failingWorker.RunOnce(ctx)
	if err == nil || failed.FailureCode != "embedding_unavailable" || failed.Processed != 0 {
		t.Fatalf("provider failure was not retained: result=%#v err=%v", failed, err)
	}
	afterFailure, err := store.RetrievalProjectionStatus(ctx, manifest.TenantID, manifest.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.LastEventID != firstBatch.LastEventID || afterFailure.Lag != firstBatch.Lag {
		t.Fatalf("provider failure advanced the cursor: before=%#v after=%#v", firstBatch, afterFailure)
	}

	runProjectionUntilCurrent(t, worker)
	if got := projectionVectorCount(t, store, manifest.TenantID, manifest.ProfileID); got != int64(manifest.RecordCount) {
		t.Fatalf("initial catch-up vector count=%d, want %d", got, manifest.RecordCount)
	}
	rewindProjectionCursor(t, store, manifest.TenantID, manifest.ProfileID)
	runProjectionUntilCurrent(t, worker)
	if got := projectionVectorCount(t, store, manifest.TenantID, manifest.ProfileID); got != int64(manifest.RecordCount) {
		t.Fatalf("duplicate replay changed vector count=%d, want %d", got, manifest.RecordCount)
	}

	governance := NewGovernanceService(store, manifest.TenantID)
	restartMemory, err := governance.AddSource(ctx, "/fixtures/w11/outbox", GovernanceWriteRequest{
		OperationID: "w11-restart-source", MemoryKey: "outbox.restart.fact",
		Content: "Restart recovery marker is W11-RESTART-7319.", SourceRef: "fixture:w11-restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	restartBlocking := &projectionTestEmbedder{
		vector: testVector1024(0.5), started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	restartWorker := mustProjectionWorker(t, store, restartBlocking, manifest.TenantID, 1)
	restartDone := make(chan error, 1)
	go func() {
		_, runErr := restartWorker.RunOnce(ctx)
		restartDone <- runErr
	}()
	waitForProjectionEmbedding(t, restartBlocking.started)
	cluster.stop(t, "immediate")
	close(restartBlocking.release)
	select {
	case runErr := <-restartDone:
		if runErr == nil {
			t.Fatal("worker unexpectedly committed while PostgreSQL was stopped")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not return after PostgreSQL stop")
	}
	cluster.start(t)
	waitForStoreRecovery(t, store)
	if got := projectionMemoryVectorCount(t, store, manifest.TenantID, manifest.ProfileID, restartMemory.Memory.MemoryID); got != 0 {
		t.Fatalf("restart failure committed a partial vector: %d", got)
	}
	runProjectionUntilCurrent(t, worker)
	if got := projectionMemoryVectorCount(t, store, manifest.TenantID, manifest.ProfileID, restartMemory.Memory.MemoryID); got != 1 {
		t.Fatalf("restart recovery did not project the pending fact: %d", got)
	}

	deletedMemory, err := governance.AddSource(ctx, "/fixtures/w11/outbox", GovernanceWriteRequest{
		OperationID: "w11-delete-source", MemoryKey: "outbox.deleted.fact",
		Content: "Deletion race marker is W11-DELETE-8842.", SourceRef: "fixture:w11-delete",
	})
	if err != nil {
		t.Fatal(err)
	}
	deleteBlocking := &projectionTestEmbedder{
		vector: testVector1024(0.75), started: make(chan struct{}, 1), release: make(chan struct{}),
	}
	deleteWorker := mustProjectionWorker(t, store, deleteBlocking, manifest.TenantID, 1)
	deleteDone := make(chan error, 1)
	go func() {
		_, runErr := deleteWorker.RunOnce(ctx)
		deleteDone <- runErr
	}()
	waitForProjectionEmbedding(t, deleteBlocking.started)
	if _, err := governance.Forget(ctx, "/fixtures/w11/outbox", deletedMemory.Memory.MemoryID, "w11-delete-race"); err != nil {
		t.Fatal(err)
	}
	close(deleteBlocking.release)
	if err := <-deleteDone; err == nil || !strings.Contains(err.Error(), "authority_changed") {
		t.Fatalf("late embedding did not lose to deletion: %v", err)
	}
	runProjectionUntilCurrent(t, worker)
	if got := projectionMemoryVectorCount(t, store, manifest.TenantID, manifest.ProfileID, deletedMemory.Memory.MemoryID); got != 0 {
		t.Fatalf("deleted memory was resurrected by retry: %d", got)
	}

	realContinuityID, err := store.ConfirmWorkspaceBinding(ctx, manifest.RealProviderTenantID, "/fixtures/w11/real-provider")
	if err != nil {
		t.Fatal(err)
	}
	realGovernance := NewGovernanceService(store, manifest.RealProviderTenantID)
	realMemory, err := realGovernance.AddSource(ctx, "/fixtures/w11/real-provider", GovernanceWriteRequest{
		OperationID: "w11-real-provider-source", MemoryKey: "outbox.real.fact",
		Content: "The real provider recovery code is W11-SILICON-4407.", SourceRef: "fixture:w11-real-provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	realBase, err := memorybackend.NewOpenAIEmbedder(
		profile.BaseURL, apiKey, profile.Model, profile.Dimensions, &http.Client{Timeout: 2 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	realEmbedder := &faultCountingEmbedder{Embedder: realBase}
	realWorker, err := NewProjectionWorker(store, realEmbedder, ProjectionWorkerOptions{
		TenantID: manifest.RealProviderTenantID, Profile: profile, BatchSize: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	runProjectionUntilCurrent(t, realWorker)
	coordinator, err := NewRetrievalCoordinator(store, realEmbedder, profile)
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := coordinator.Retrieve(ctx, RetrievalRequest{
		OperationID: "w11-real-provider-query", TenantID: manifest.RealProviderTenantID,
		ContinuityIDs: []string{realContinuityID}, Query: "Which recovery code came from the real provider fact?",
		Limit: 3, Mode: RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsMemoryID(retrieved.Memories, realMemory.Memory.MemoryID) || realEmbedder.Count() != 2 {
		t.Fatalf("real provider recovery mismatch: memories=%#v requests=%d", retrieved.Memories, realEmbedder.Count())
	}

	finalStatus, err := store.RetrievalProjectionStatus(ctx, manifest.TenantID, manifest.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if finalStatus.Lag != 0 || finalStatus.Status != "idle" {
		t.Fatalf("final outbox status is not current: %#v", finalStatus)
	}
	payload, _ := json.Marshal(map[string]any{
		"records":                           manifest.RecordCount,
		"initial_backlog":                   initialStatus.Lag,
		"first_batch_processed":             firstBatch.Processed,
		"provider_failure_cursor_unchanged": true,
		"duplicate_replay_vector_count":     manifest.RecordCount,
		"postgres_restart_during_embedding": true,
		"same_pool_recovered":               true,
		"concurrent_delete_won":             true,
		"final_lag":                         finalStatus.Lag,
		"real_provider_requests":            realEmbedder.Count(),
	})
	t.Logf("projection outbox fault evidence=%s", payload)
}

type disposablePostgres18 struct {
	binDir      string
	dataDir     string
	socketDir   string
	logPath     string
	port        int
	databaseURL string
	running     bool
}

func startDisposablePostgres18(t *testing.T) *disposablePostgres18 {
	t.Helper()
	binDir := strings.TrimSpace(os.Getenv("VERMORY_POSTGRES18_BIN"))
	if binDir == "" {
		binDir = "/opt/homebrew/opt/postgresql@18/bin"
	}
	for _, name := range []string{"initdb", "pg_ctl", "createdb"} {
		if _, err := os.Stat(filepath.Join(binDir, name)); err != nil {
			t.Skipf("PostgreSQL 18 binary %s is required: %v", name, err)
		}
	}
	root, err := os.MkdirTemp("/tmp", "vermory-w11-pg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	cluster := &disposablePostgres18{
		binDir: binDir, dataDir: filepath.Join(root, "data"), socketDir: filepath.Join(root, "socket"),
		logPath: filepath.Join(root, "postgres.log"), port: freePostgresPort(t),
	}
	if err := os.MkdirAll(cluster.socketDir, 0o700); err != nil {
		t.Fatal(err)
	}
	username := strings.TrimSpace(os.Getenv("USER"))
	if username == "" {
		username = "postgres"
	}
	runPostgresCommand(t, filepath.Join(binDir, "initdb"),
		"--no-locale", "--encoding=UTF8", "--auth=trust", "--username="+username, cluster.dataDir)
	cluster.start(t)
	runPostgresCommand(t, filepath.Join(binDir, "createdb"),
		"-h", cluster.socketDir, "-p", fmt.Sprint(cluster.port), "vermory_outbox_fault")
	cluster.databaseURL = fmt.Sprintf("postgresql:///vermory_outbox_fault?host=%s&port=%d", url.QueryEscape(cluster.socketDir), cluster.port)
	return cluster
}

func (cluster *disposablePostgres18) start(t *testing.T) {
	t.Helper()
	if cluster.running {
		return
	}
	options := fmt.Sprintf("-k %s -h 127.0.0.1 -p %d", cluster.socketDir, cluster.port)
	runPostgresCommand(t, filepath.Join(cluster.binDir, "pg_ctl"),
		"-D", cluster.dataDir, "-l", cluster.logPath, "-o", options, "-w", "start")
	cluster.running = true
}

func (cluster *disposablePostgres18) stop(t *testing.T, mode string) {
	t.Helper()
	if !cluster.running {
		return
	}
	runPostgresCommand(t, filepath.Join(cluster.binDir, "pg_ctl"),
		"-D", cluster.dataDir, "-m", mode, "-w", "stop")
	cluster.running = false
}

func runPostgresCommand(t *testing.T, command string, args ...string) {
	t.Helper()
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", filepath.Base(command), err, output)
	}
}

func freePostgresPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func seedProjectionBacklog(t *testing.T, store *Store, manifest projectionOutboxFaultCase, continuityID string) {
	t.Helper()
	ctx, err := withTenantContext(context.Background(), manifest.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	for batchStart := 0; batchStart < manifest.RecordCount; batchStart += 100 {
		batchEnd := batchStart + 100
		if batchEnd > manifest.RecordCount {
			batchEnd = manifest.RecordCount
		}
		tx, err := store.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for index := batchStart; index < batchEnd; index++ {
			request := CommitObservationRequest{
				OperationID: fmt.Sprintf("w11-backlog-%04d", index), Kind: ObservationKindSourceUpdate,
				Content:   fmt.Sprintf("Outbox backlog fact %04d uses marker W11-%04d.", index, index),
				SourceRef: fmt.Sprintf("fixture:w11:%04d", index), MemoryKey: fmt.Sprintf("outbox.fact.%04d", index),
			}
			observation, err := commitObservationTx(ctx, tx, manifest.TenantID, continuityID, request)
			if err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
			if _, err := governObservationTx(ctx, tx, manifest.TenantID, continuityID, observation.ObservationID, request); err != nil {
				tx.Rollback(ctx)
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func productionRetrievalProfile(t *testing.T) RetrievalProfile {
	t.Helper()
	spec, ok := SupportedRetrievalProfile(ProductionRetrievalProfileID)
	if !ok {
		t.Fatal("production retrieval profile is not registered")
	}
	return RetrievalProfile{
		ID: spec.ID, BaseURL: spec.BaseURL, Model: spec.Model,
		Dimensions: spec.Dimensions, ProjectionClass: spec.ProjectionClass,
	}
}

func runProjectionUntilCurrent(t *testing.T, worker *ProjectionWorker) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		result, err := worker.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.Lag == 0 {
			return
		}
	}
	t.Fatal("projection worker did not reach zero lag")
}

func rewindProjectionCursor(t *testing.T, store *Store, tenantID, profileID string) {
	t.Helper()
	ctx, err := withTenantContext(context.Background(), tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
UPDATE memory_projection_cursors
SET last_event_id = 0, status = 'idle', last_error_code = '', updated_at = now()
WHERE tenant_id = $1 AND profile_id = $2`, tenantID, profileID); err != nil {
		t.Fatal(err)
	}
}

func projectionVectorCount(t *testing.T, store *Store, tenantID, profileID string) int64 {
	t.Helper()
	var count int64
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2`, tenantID, profileID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func projectionMemoryVectorCount(t *testing.T, store *Store, tenantID, profileID, memoryID string) int64 {
	t.Helper()
	var count int64
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2 AND memory_id = $3::uuid`, tenantID, profileID, memoryID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func waitForProjectionEmbedding(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("projection worker did not reach embedding")
	}
}

func waitForStoreRecovery(t *testing.T, store *Store) {
	t.Helper()
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		err = store.pool.Ping(context.Background())
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("runtime pool did not recover after PostgreSQL restart: %v", err)
}

func containsMemoryID(memories []Memory, want string) bool {
	for _, memory := range memories {
		if memory.ID == want {
			return true
		}
	}
	return false
}

type faultCountingEmbedder struct {
	Embedder
	requests atomic.Int64
}

func (embedder *faultCountingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embedder.requests.Add(1)
	return embedder.Embedder.Embed(ctx, text)
}

func (embedder *faultCountingEmbedder) Count() int64 {
	return embedder.requests.Load()
}

func loadProjectionOutboxFaultCase(t *testing.T) projectionOutboxFaultCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W11-projection-outbox-fault-profile", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest projectionOutboxFaultCase
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
