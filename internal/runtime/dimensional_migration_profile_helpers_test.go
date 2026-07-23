package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type dimensionalFixtureDataset struct {
	Prefix                string
	ContinuitiesPerTenant int
	Tenants               []string
	Records               map[string][]dimensionalFixtureRecord
}

type dimensionalFixtureRecord struct {
	TenantID     string
	RepoRoot     string
	ContinuityID string
	MemoryID     string
	MemoryKey    string
	Content      string
}

type dimensionalFixtureEmbedder struct {
	dimensions int
	calls      atomic.Int64
}

func (e *dimensionalFixtureEmbedder) Embed(_ context.Context, content string) ([]float32, error) {
	e.calls.Add(1)
	digest := sha256.Sum256([]byte(content))
	vector := make([]float32, e.dimensions)
	for index := 0; index < len(digest)/4 && index < len(vector); index++ {
		value := binary.BigEndian.Uint32(digest[index*4 : index*4+4])
		vector[index] = float32(value%1000000) / 1000000
	}
	return vector, nil
}

type dimensionalBlockingEmbedder struct {
	delegate Embedder
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
	entered  atomic.Int64
}

func (e *dimensionalBlockingEmbedder) Embed(ctx context.Context, content string) ([]float32, error) {
	e.entered.Add(1)
	e.once.Do(func() {
		close(e.started)
		select {
		case <-e.release:
		case <-ctx.Done():
		}
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return e.delegate.Embed(ctx, content)
}

func seedDimensionalFixture(t *testing.T, store *Store, prefix string, tenants, continuities, recordsPerContinuity int) dimensionalFixtureDataset {
	t.Helper()
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		t.Fatal("dimensional fixture prefix is required")
	}
	dataset := dimensionalFixtureDataset{
		Prefix: prefix, ContinuitiesPerTenant: continuities,
		Tenants: make([]string, tenants), Records: make(map[string][]dimensionalFixtureRecord, tenants),
	}
	for tenantIndex := 0; tenantIndex < tenants; tenantIndex++ {
		tenantID := fmt.Sprintf("%s-tenant-%02d", prefix, tenantIndex)
		dataset.Tenants[tenantIndex] = tenantID
		governance := NewGovernanceService(store, tenantID)
		for continuityIndex := 0; continuityIndex < continuities; continuityIndex++ {
			repoRoot := fmt.Sprintf("/fixtures/%s/%02d/%02d", prefix, tenantIndex, continuityIndex)
			resolution, err := governance.ConfirmWorkspace(context.Background(), repoRoot)
			if err != nil {
				t.Fatal(err)
			}
			for recordIndex := 0; recordIndex < recordsPerContinuity; recordIndex++ {
				memoryKey := fmt.Sprintf("w17.mini.%02d.%02d.%03d", tenantIndex, continuityIndex, recordIndex)
				content := fmt.Sprintf(
					"W17 marker T%02d-C%02d-R%03d uses endpoint /v1/w17/%02d/%02d/%03d and retry budget %d ms.",
					tenantIndex, continuityIndex, recordIndex,
					tenantIndex, continuityIndex, recordIndex, 300+recordIndex,
				)
				receipt, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
					OperationID: fmt.Sprintf("%s-seed-%02d-%02d-%03d", prefix, tenantIndex, continuityIndex, recordIndex),
					MemoryKey:   memoryKey, Content: content,
					SourceRef: fmt.Sprintf("fixture:%s:%02d:%02d:%03d", prefix, tenantIndex, continuityIndex, recordIndex),
				})
				if err != nil {
					t.Fatal(err)
				}
				dataset.Records[tenantID] = append(dataset.Records[tenantID], dimensionalFixtureRecord{
					TenantID: tenantID, RepoRoot: repoRoot, ContinuityID: resolution.ContinuityID,
					MemoryID: receipt.Memory.MemoryID, MemoryKey: memoryKey, Content: content,
				})
			}
		}
	}
	return dataset
}

func applyDimensionalFixtureTail(
	t *testing.T,
	store *Store,
	dataset *dimensionalFixtureDataset,
	revisionsPerTenant int,
	deletesPerTenant int,
	newFactsPerTenant int,
) {
	t.Helper()
	for tenantIndex, tenantID := range dataset.Tenants {
		governance := NewGovernanceService(store, tenantID)
		records := dataset.Records[tenantID]
		if revisionsPerTenant+deletesPerTenant > len(records) {
			t.Fatalf("dimensional tail exceeds tenant records: revisions=%d deletes=%d records=%d", revisionsPerTenant, deletesPerTenant, len(records))
		}
		for index := 0; index < revisionsPerTenant; index++ {
			record := records[index]
			content := record.Content + " Revision 2 is current."
			receipt, err := governance.ReviseSource(context.Background(), record.RepoRoot, record.MemoryID, GovernanceWriteRequest{
				OperationID: fmt.Sprintf("%s-revise-%02d-%02d", dataset.Prefix, tenantIndex, index),
				MemoryKey:   record.MemoryKey, Content: content,
				SourceRef: fmt.Sprintf("fixture:%s:revision:%02d:%02d", dataset.Prefix, tenantIndex, index),
			})
			if err != nil {
				t.Fatal(err)
			}
			record.MemoryID = receipt.Memory.MemoryID
			record.Content = content
			records[index] = record
		}
		deleteEnd := revisionsPerTenant + deletesPerTenant
		for index := revisionsPerTenant; index < deleteEnd; index++ {
			record := records[index]
			if _, err := governance.Forget(
				context.Background(), record.RepoRoot, record.MemoryID,
				fmt.Sprintf("%s-delete-%02d-%02d", dataset.Prefix, tenantIndex, index),
			); err != nil {
				t.Fatal(err)
			}
		}
		records = append(records[:revisionsPerTenant], records[deleteEnd:]...)
		for newIndex := 0; newIndex < newFactsPerTenant; newIndex++ {
			continuityIndex := newIndex % dataset.ContinuitiesPerTenant
			repoRoot := fmt.Sprintf("/fixtures/%s/%02d/%02d", dataset.Prefix, tenantIndex, continuityIndex)
			continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)
			memoryKey := fmt.Sprintf("w17.%s.%02d.new.%04d", dataset.Prefix, tenantIndex, newIndex)
			content := fmt.Sprintf("W17 new marker T%02d-N%02d is active after migration start.", tenantIndex, newIndex)
			receipt, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
				OperationID: fmt.Sprintf("%s-new-%02d-%02d", dataset.Prefix, tenantIndex, newIndex),
				MemoryKey:   memoryKey, Content: content,
				SourceRef: fmt.Sprintf("fixture:%s:new:%02d:%02d", dataset.Prefix, tenantIndex, newIndex),
			})
			if err != nil {
				t.Fatal(err)
			}
			records = append(records, dimensionalFixtureRecord{
				TenantID: tenantID, RepoRoot: repoRoot, ContinuityID: continuityID,
				MemoryID: receipt.Memory.MemoryID, MemoryKey: memoryKey, Content: content,
			})
		}
		dataset.Records[tenantID] = records
	}
}

func newDimensionalWorker(t *testing.T, store *Store, tenantID string, profile RetrievalProfile, embedder Embedder, snapshotPageSize, batchSize int) *ProjectionWorker {
	t.Helper()
	worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
		TenantID: tenantID, Profile: profile, SnapshotPageSize: snapshotPageSize, BatchSize: batchSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func drainDimensionalProjection(t *testing.T, worker *ProjectionWorker) ProjectionStatus {
	t.Helper()
	for attempt := 0; attempt < 1000; attempt++ {
		result, err := worker.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("drain dimensional projection: result=%#v err=%v", result, err)
		}
		if result.Lag == 0 {
			status, err := worker.store.RetrievalProjectionStatus(context.Background(), worker.options.TenantID, worker.options.Profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			return status
		}
	}
	t.Fatal("dimensional projection did not reach zero lag")
	return ProjectionStatus{}
}

func dimensionalVectorIDs(t *testing.T, store *Store, tenantID string, class ProjectionClass) []string {
	t.Helper()
	query := `
SELECT memory_id::text
FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2
ORDER BY memory_id`
	profileID := ProductionRetrievalProfileID
	if class == ProjectionClass2560 {
		query = `
SELECT memory_id::text
FROM memory_vector_documents_2560
WHERE tenant_id = $1 AND profile_id = $2
ORDER BY memory_id`
		profileID = DimensionalMigrationRetrievalProfileID
	}
	rows, err := store.pool.Query(context.Background(), query, tenantID, profileID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func governedActiveIDs(t *testing.T, store *Store, tenantID string) []string {
	t.Helper()
	rows, err := store.pool.Query(context.Background(), `
SELECT id::text
FROM governed_memories
WHERE tenant_id = $1 AND memory_kind = 'fact' AND lifecycle_status = 'active' AND content <> '[redacted]'
ORDER BY id`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func sameStringSet(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type dimensionalPostgres18 struct {
	binDir      string
	root        string
	dataDir     string
	logPath     string
	port        int
	databaseURL string
	version     string
	running     bool
}

func TestDimensionalPostgresVersionParsesVendorSuffix(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
		want   string
	}{
		{name: "upstream", output: "postgres (PostgreSQL) 18.4", want: "18.4"},
		{name: "homebrew", output: "postgres (PostgreSQL) 18.4 (Homebrew)", want: "18.4"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := dimensionalPostgresVersion(test.output)
			if err != nil || got != test.want {
				t.Fatalf("dimensionalPostgresVersion(%q)=%q, %v; want %q", test.output, got, err, test.want)
			}
		})
	}
	if _, err := dimensionalPostgresVersion("postgres version unavailable"); err == nil {
		t.Fatal("version output without a numeric version was accepted")
	}
}

func dimensionalPostgresVersion(output string) (string, error) {
	for _, field := range strings.Fields(strings.TrimSpace(output)) {
		field = strings.Trim(field, "()")
		if len(field) > 0 && field[0] >= '0' && field[0] <= '9' && strings.Contains(field, ".") {
			return field, nil
		}
	}
	return "", fmt.Errorf("PostgreSQL version output has no numeric version")
}

func startDimensionalPostgres18(t *testing.T, root, binDir string) *dimensionalPostgres18 {
	t.Helper()
	root = filepath.Clean(strings.TrimSpace(root))
	binDir = filepath.Clean(strings.TrimSpace(binDir))
	if !filepath.IsAbs(root) || root == string(filepath.Separator) {
		t.Fatalf("dimensional PostgreSQL root must be a dedicated absolute path: %q", root)
	}
	if !filepath.IsAbs(binDir) {
		t.Fatalf("PostgreSQL bin directory must be absolute: %q", binDir)
	}
	if _, err := os.Stat(root); err == nil {
		t.Fatalf("dimensional PostgreSQL root already exists; use a new run ID: %s", root)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, name := range []string{"postgres", "initdb", "pg_ctl", "createdb"} {
		info, err := os.Stat(filepath.Join(binDir, name))
		if err != nil || info.IsDir() {
			t.Fatalf("PostgreSQL 18 binary %s is required in %s", name, binDir)
		}
	}
	versionOutput, err := exec.Command(filepath.Join(binDir, "postgres"), "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	version, err := dimensionalPostgresVersion(string(versionOutput))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, "18.") {
		t.Fatalf("PostgreSQL 18 is required, got %s", version)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	cluster := &dimensionalPostgres18{
		binDir: binDir, root: root, dataDir: filepath.Join(root, "data"),
		logPath: filepath.Join(root, "postgres.log"), port: freeDimensionalPostgresPort(t), version: version,
	}
	username := strings.TrimSpace(os.Getenv("USER"))
	if username == "" {
		username = "postgres"
	}
	runDimensionalPostgresCommand(t, filepath.Join(binDir, "initdb"),
		"--no-locale", "--encoding=UTF8", "--auth=trust", "--username="+username, cluster.dataDir)
	cluster.start(t)
	runDimensionalPostgresCommand(t, filepath.Join(binDir, "createdb"),
		"-h", "127.0.0.1", "-p", fmt.Sprint(cluster.port), "vermory_w17")
	cluster.databaseURL = fmt.Sprintf(
		"postgresql://127.0.0.1:%d/vermory_w17?sslmode=disable&connect_timeout=2",
		cluster.port,
	)
	t.Cleanup(func() {
		if cluster.running {
			cluster.stop(t, "fast")
		}
	})
	return cluster
}

func (cluster *dimensionalPostgres18) start(t *testing.T) {
	t.Helper()
	if cluster.running {
		return
	}
	options := fmt.Sprintf("-h 127.0.0.1 -p %d -c unix_socket_directories=''", cluster.port)
	runDimensionalPostgresCommand(t, filepath.Join(cluster.binDir, "pg_ctl"),
		"-D", cluster.dataDir, "-l", cluster.logPath, "-o", options, "-w", "start")
	cluster.running = true
}

func (cluster *dimensionalPostgres18) stop(t *testing.T, mode string) {
	t.Helper()
	if !cluster.running {
		return
	}
	runDimensionalPostgresCommand(t, filepath.Join(cluster.binDir, "pg_ctl"),
		"-D", cluster.dataDir, "-m", mode, "-w", "stop")
	cluster.running = false
}

func runDimensionalPostgresCommand(t *testing.T, command string, args ...string) {
	t.Helper()
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", filepath.Base(command), err, output)
	}
}

func freeDimensionalPostgresPort(t *testing.T) int {
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

type dimensionalRecordingEmbedder struct {
	delegate Embedder
	calls    atomic.Int64
	mu       sync.Mutex
	hashes   []string
}

func (e *dimensionalRecordingEmbedder) Embed(ctx context.Context, content string) ([]float32, error) {
	vector, err := e.delegate.Embed(ctx, content)
	if err != nil {
		return nil, err
	}
	e.calls.Add(1)
	digest := sha256.Sum256([]byte(retrievalVectorLiteral(vector)))
	e.mu.Lock()
	e.hashes = append(e.hashes, fmt.Sprintf("%x", digest[:]))
	e.mu.Unlock()
	return vector, nil
}

func (e *dimensionalRecordingEmbedder) snapshot() (int, []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return int(e.calls.Load()), append([]string(nil), e.hashes...)
}

func waitForDimensionalStoreRecovery(t *testing.T, store *Store, timeout time.Duration) time.Duration {
	t.Helper()
	started := time.Now()
	deadline := started.Add(timeout)
	for {
		queryCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		version, err := store.SchemaVersion(queryCtx)
		cancel()
		if err == nil && version == 16 {
			return time.Since(started)
		}
		if time.Now().After(deadline) {
			t.Fatalf("same dimensional migration pool did not recover: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func dimensionalRunRoot(baseRoot, runID string) (string, error) {
	baseRoot = filepath.Clean(strings.TrimSpace(baseRoot))
	if !filepath.IsAbs(baseRoot) || baseRoot == string(filepath.Separator) {
		return "", fmt.Errorf("dimensional migration root must be a dedicated absolute path")
	}
	if !dimensionalMigrationSafeName.MatchString(runID) {
		return "", fmt.Errorf("dimensional migration run ID is invalid")
	}
	return filepath.Join(baseRoot, runID), nil
}

func dimensionalDatabaseURLHost(databaseURL string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
