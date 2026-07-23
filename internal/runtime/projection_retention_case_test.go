package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type projectionRetentionCase struct {
	Version               string   `json:"version"`
	ID                    string   `json:"id"`
	ProfileName           string   `json:"profile_name"`
	TenantCount           int      `json:"tenant_count"`
	ContinuitiesPerTenant int      `json:"continuities_per_tenant"`
	RecordsPerContinuity  int      `json:"records_per_continuity"`
	InitialCurrentFacts   int      `json:"initial_current_facts"`
	AcceleratedEpochs     int      `json:"accelerated_epochs"`
	RevisionCount         int      `json:"revision_count"`
	DeleteCount           int      `json:"delete_count"`
	NewFactCount          int      `json:"new_fact_count"`
	TailEventCount        int      `json:"tail_event_count"`
	TotalGeneratedEvents  int      `json:"total_generated_events"`
	FinalCurrentFacts     int      `json:"final_current_facts"`
	RetainedTailPerTenant int      `json:"retained_tail_per_tenant"`
	QueryClientCount      int      `json:"query_client_count"`
	QueriesPerClient      int      `json:"queries_per_client"`
	WorkerBatchSize       int      `json:"worker_batch_size"`
	SnapshotPageSize      int      `json:"snapshot_page_size"`
	PoolMaxConnections    int      `json:"pool_max_connections"`
	IncumbentProfileID    string   `json:"incumbent_profile_id"`
	DimensionalProfileID  string   `json:"dimensional_profile_id"`
	FutureProfileID       string   `json:"future_profile_id"`
	RealProviderTenantID  string   `json:"real_provider_tenant_id"`
	HardGates             []string `json:"hard_gates"`
}

func TestProjectionRetentionCaseIsFrozen(t *testing.T) {
	manifest := loadProjectionRetentionCase(t)
	if manifest.Version != "1" || manifest.ID != "W18-projection-event-retention-pruning" ||
		manifest.ProfileName != "projection-event-retention-v1" {
		t.Fatalf("unexpected W18 identity: %#v", manifest)
	}
	initial := manifest.TenantCount * manifest.ContinuitiesPerTenant * manifest.RecordsPerContinuity
	tail := manifest.RevisionCount*2 + manifest.DeleteCount + manifest.NewFactCount
	finalCurrent := initial - manifest.DeleteCount + manifest.NewFactCount
	queries := manifest.QueryClientCount * manifest.QueriesPerClient
	if initial != 20000 || manifest.InitialCurrentFacts != initial {
		t.Fatalf("W18 initial arithmetic drifted: initial=%d manifest=%d", initial, manifest.InitialCurrentFacts)
	}
	if tail != 153600 || manifest.TailEventCount != tail ||
		manifest.TotalGeneratedEvents != initial+tail || manifest.TotalGeneratedEvents != 173600 {
		t.Fatalf("W18 event arithmetic drifted: tail=%d total=%d", tail, manifest.TotalGeneratedEvents)
	}
	if finalCurrent != 20000 || manifest.FinalCurrentFacts != finalCurrent ||
		manifest.AcceleratedEpochs != 24 || manifest.RetainedTailPerTenant != 1000 {
		t.Fatalf("W18 final or retention arithmetic drifted: %#v", manifest)
	}
	if queries != 320 || len(manifest.HardGates) != 14 {
		t.Fatalf("W18 query or hard-gate contract drifted: queries=%d gates=%d", queries, len(manifest.HardGates))
	}
	if manifest.IncumbentProfileID != ProductionRetrievalProfileID ||
		manifest.DimensionalProfileID != DimensionalMigrationRetrievalProfileID ||
		manifest.FutureProfileID != MigrationRetrievalProfileID {
		t.Fatalf("W18 profile IDs drifted: %#v", manifest)
	}
	if manifest.WorkerBatchSize != 256 || manifest.SnapshotPageSize != 250 ||
		manifest.PoolMaxConnections != 48 || manifest.RealProviderTenantID != "w18-real-provider-tenant" {
		t.Fatalf("W18 runtime settings drifted: %#v", manifest)
	}
}

func loadProjectionRetentionCase(t *testing.T) projectionRetentionCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W18-projection-event-retention-pruning", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest projectionRetentionCase
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("W18 case contains trailing JSON data: %v", err)
	}
	return manifest
}
