package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type memoryEligibilityCase struct {
	Version                string   `json:"version"`
	ID                     string   `json:"id"`
	ProfileName            string   `json:"profile_name"`
	TenantCount            int      `json:"tenant_count"`
	ContinuitiesPerTenant  int      `json:"continuities_per_tenant"`
	TotalGovernedFacts     int      `json:"total_governed_facts"`
	CurrentOpenEnded       int      `json:"current_open_ended"`
	Scheduled              int      `json:"scheduled"`
	Expired                int      `json:"expired"`
	Archived               int      `json:"archived"`
	Superseded             int      `json:"superseded"`
	Deleted                int      `json:"deleted"`
	QueryClientCount       int      `json:"query_client_count"`
	QueriesPerClient       int      `json:"queries_per_client"`
	ActiveProfileID        string   `json:"active_profile_id"`
	DirectProviderTenantID string   `json:"direct_provider_tenant_id"`
	RealityCaseIDs         []string `json:"reality_case_ids"`
	HardGateCount          int      `json:"hard_gate_count"`
	HardGates              []string `json:"hard_gates"`
}

func TestMemoryEligibilityCaseIsFrozen(t *testing.T) {
	manifest := loadMemoryEligibilityCase(t)
	if manifest.Version != "1" || manifest.ID != "W19-memory-eligibility-retention" ||
		manifest.ProfileName != "memory-eligibility-retention-v1" {
		t.Fatalf("unexpected W19 identity: %#v", manifest)
	}
	total := manifest.CurrentOpenEnded + manifest.Scheduled + manifest.Expired +
		manifest.Archived + manifest.Superseded + manifest.Deleted
	queries := manifest.QueryClientCount * manifest.QueriesPerClient
	if total != 10000 || manifest.TotalGovernedFacts != total {
		t.Fatalf("W19 state arithmetic drifted: total=%d manifest=%d", total, manifest.TotalGovernedFacts)
	}
	if manifest.TenantCount != 4 || manifest.ContinuitiesPerTenant != 5 || queries != 320 {
		t.Fatalf("W19 topology or query arithmetic drifted: %#v queries=%d", manifest, queries)
	}
	if manifest.CurrentOpenEnded != 4000 || manifest.Scheduled != 1500 || manifest.Expired != 1500 ||
		manifest.Archived != 1000 || manifest.Superseded != 1000 || manifest.Deleted != 1000 {
		t.Fatalf("W19 state counts drifted: %#v", manifest)
	}
	if manifest.ActiveProfileID != ProductionRetrievalProfileID ||
		manifest.DirectProviderTenantID != "w19-direct-provider-tenant" {
		t.Fatalf("W19 provider identity drifted: %#v", manifest)
	}
	wantCases := []string{
		"G01-language-default-local-override",
		"S01-deletion-and-source-injection",
		"C02-housing-viewing-validity",
		"W03-workspace-workaround-validity",
	}
	if !reflect.DeepEqual(manifest.RealityCaseIDs, wantCases) {
		t.Fatalf("W19 reality cases drifted: got=%#v want=%#v", manifest.RealityCaseIDs, wantCases)
	}
	if manifest.HardGateCount != 16 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("W19 hard-gate contract drifted: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	seen := make(map[string]struct{}, len(manifest.HardGates))
	for _, gate := range manifest.HardGates {
		if gate == "" {
			t.Fatal("W19 hard gate cannot be empty")
		}
		if _, exists := seen[gate]; exists {
			t.Fatalf("W19 hard gate is duplicated: %q", gate)
		}
		seen[gate] = struct{}{}
	}
}

func loadMemoryEligibilityCase(t *testing.T) memoryEligibilityCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W19-memory-eligibility-retention", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest memoryEligibilityCase
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("W19 case contains trailing JSON data: %v", err)
	}
	return manifest
}
