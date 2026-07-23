package runtime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type dimensionalMigrationCase struct {
	Version               string `json:"version"`
	ID                    string `json:"id"`
	ProfileName           string `json:"profile_name"`
	TenantCount           int    `json:"tenant_count"`
	ContinuitiesPerTenant int    `json:"continuities_per_tenant"`
	RecordsPerContinuity  int    `json:"records_per_continuity"`
	InitialActiveCount    int    `json:"initial_active_count"`
	RevisionCount         int    `json:"revision_count"`
	DeleteCount           int    `json:"delete_count"`
	NewFactCount          int    `json:"new_fact_count"`
	TailEventCount        int    `json:"tail_event_count"`
	QueryClientCount      int    `json:"query_client_count"`
	QueriesPerClient      int    `json:"queries_per_client"`
	SnapshotPageSize      int    `json:"snapshot_page_size"`
	WorkerBatchSize       int    `json:"worker_batch_size"`
	PoolMaxConnections    int    `json:"pool_max_connections"`
	IncumbentProfileID    string `json:"incumbent_profile_id"`
	CandidateProfileID    string `json:"candidate_profile_id"`
	RealProviderTenantID  string `json:"real_provider_tenant_id"`
	ReferenceHardware     struct {
		OS         string `json:"os"`
		CPU        string `json:"cpu"`
		MemoryGiB  int    `json:"memory_gib"`
		Storage    string `json:"storage"`
		PostgreSQL string `json:"postgresql"`
		PGVector   string `json:"pgvector"`
	} `json:"reference_hardware"`
	CalibratedLimits struct {
		AuthoritySeedSeconds     int `json:"authority_seed_seconds"`
		IncumbentSnapshotSeconds int `json:"incumbent_snapshot_seconds"`
		CandidateSnapshotSeconds int `json:"candidate_snapshot_seconds"`
		WriterSeconds            int `json:"writer_seconds"`
		TailCatchupSeconds       int `json:"tail_catchup_seconds"`
		RestartRecoverySeconds   int `json:"restart_recovery_seconds"`
		QueryP95MS               int `json:"query_p95_ms"`
		QueryP99MS               int `json:"query_p99_ms"`
		DatabaseSizeGiB          int `json:"database_size_gib"`
	} `json:"calibrated_limits"`
	HardGates []string `json:"hard_gates"`
}

func TestDimensionalMigrationCaseIsFrozen(t *testing.T) {
	manifest := loadDimensionalMigrationCase(t)
	if manifest.Version != "1" || manifest.ID != "W17-active-backlog-dimensional-migration" ||
		manifest.ProfileName != "active-backlog-dimension-v1" {
		t.Fatalf("unexpected W17 identity: %#v", manifest)
	}
	initial := manifest.TenantCount * manifest.ContinuitiesPerTenant * manifest.RecordsPerContinuity
	tail := manifest.RevisionCount*2 + manifest.DeleteCount + manifest.NewFactCount
	finalActive := initial - manifest.DeleteCount + manifest.NewFactCount
	if initial != 20000 || manifest.InitialActiveCount != initial {
		t.Fatalf("W17 initial arithmetic drifted: initial=%d manifest=%d", initial, manifest.InitialActiveCount)
	}
	if tail != 5000 || manifest.TailEventCount != tail || finalActive != 20000 {
		t.Fatalf("W17 tail arithmetic drifted: tail=%d manifest=%d final=%d", tail, manifest.TailEventCount, finalActive)
	}
	if manifest.QueryClientCount*manifest.QueriesPerClient != 320 || len(manifest.HardGates) != 12 {
		t.Fatalf("W17 query or hard-gate contract drifted: %#v", manifest)
	}
	if manifest.IncumbentProfileID != ProductionRetrievalProfileID ||
		manifest.CandidateProfileID != "siliconflow-qwen3-embedding-4b-2560-v3" {
		t.Fatalf("W17 profile IDs drifted: %#v", manifest)
	}
}

func loadDimensionalMigrationCase(t *testing.T) dimensionalMigrationCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W17-active-backlog-dimensional-migration", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest dimensionalMigrationCase
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
