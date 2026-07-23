package casebook

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadBenchmarkMap(t *testing.T) {
	entries, err := LoadBenchmarkMap("../../casebook/benchmarks/public-benchmark-map.json")
	if err != nil {
		t.Fatalf("expected benchmark map to load, got error: %v", err)
	}

	if len(entries) != 11 {
		t.Fatalf("expected 11 benchmark entries, got %d", len(entries))
	}

	locomo := findBenchmark(entries, "LoCoMo")
	if locomo == nil {
		t.Fatal("expected LoCoMo benchmark entry")
	}
	if locomo.Line != BenchmarkTrackConversation {
		t.Fatalf("expected LoCoMo line %q, got %q", BenchmarkTrackConversation, locomo.Line)
	}

	everMemBench := findBenchmark(entries, "EverMemBench")
	if everMemBench == nil {
		t.Fatal("expected EverMemBench benchmark entry")
	}
	if everMemBench.Line != BenchmarkTrackBridge {
		t.Fatalf("expected EverMemBench line %q, got %q", BenchmarkTrackBridge, everMemBench.Line)
	}
	if everMemBench.Capability != "team_handoff" {
		t.Fatalf("expected EverMemBench capability team_handoff, got %q", everMemBench.Capability)
	}
}

func TestBenchmarkMapMeetsInternalReadyCoverage(t *testing.T) {
	entries, err := LoadBenchmarkMap("../../casebook/benchmarks/public-benchmark-map.json")
	if err != nil {
		t.Fatalf("expected benchmark map to load, got error: %v", err)
	}

	report := ValidateBenchmarkCoverage(entries)
	if report.Total != 11 {
		t.Fatalf("expected 11 benchmark entries, got %d", report.Total)
	}
	if len(report.MissingTranslatedTask) != 0 {
		t.Fatalf("expected every benchmark to reach at least translated task, missing: %v", report.MissingTranslatedTask)
	}
	if report.ExecutableCount < 4 {
		t.Fatalf("expected at least 4 executable evaluations, got %d", report.ExecutableCount)
	}
	if len(report.ExecutableWithoutCases) != 0 {
		t.Fatalf("expected executable benchmarks to name case ids, missing: %v", report.ExecutableWithoutCases)
	}
}

func TestLoadCaseDirectory(t *testing.T) {
	casebookCase, err := LoadCase("../../casebook/cases/001-contextmesh-bluebridge-preparation")
	if err != nil {
		t.Fatalf("expected case to load, got error: %v", err)
	}

	if casebookCase.ID != "001-contextmesh-bluebridge-preparation" {
		t.Fatalf("expected case id 001-contextmesh-bluebridge-preparation, got %q", casebookCase.ID)
	}
	if casebookCase.SourceMD == "" {
		t.Fatal("expected source body to be loaded")
	}
	if !strings.Contains(casebookCase.SourceMD, "ContextMesh is a multi-platform AI workflow continuity context governance platform") {
		t.Fatalf("expected source body to contain Bluebridge case summary, got %q", casebookCase.SourceMD)
	}
	if len(casebookCase.Tasks) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(casebookCase.Tasks))
	}
	if len(casebookCase.Claims) != 6 {
		t.Fatalf("expected 6 claims, got %d", len(casebookCase.Claims))
	}
	if casebookCase.Tasks[0].ID != "self-case-stale-context" {
		t.Fatalf("expected first task id self-case-stale-context, got %q", casebookCase.Tasks[0].ID)
	}
	if casebookCase.Claims[0].Type != "goal" {
		t.Fatalf("expected first claim type goal, got %q", casebookCase.Claims[0].Type)
	}
}

func TestLoadMinimumV1MainCaseMatrix(t *testing.T) {
	caseRoot := "../../casebook/cases"
	entries, err := os.ReadDir(caseRoot)
	if err != nil {
		t.Fatalf("read case root: %v", err)
	}

	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		loadedCase, err := LoadCase(filepath.Join(caseRoot, entry.Name()))
		if err != nil {
			t.Fatalf("load case %s: %v", entry.Name(), err)
		}
		if len(loadedCase.Tasks) == 0 {
			t.Fatalf("case %s has no tasks", entry.Name())
		}
		if len(loadedCase.Claims) == 0 {
			t.Fatalf("case %s has no claims", entry.Name())
		}
		loaded++
	}

	if loaded < 16 {
		t.Fatalf("expected at least 16 V1 main cases, got %d", loaded)
	}
}

func TestLoadWorkspaceSourceRevisionCase(t *testing.T) {
	loaded, err := LoadCase("../../casebook/cases/106-workspace-source-revision")
	if err != nil {
		t.Fatalf("load source revision case: %v", err)
	}
	if loaded.ID != "106-workspace-source-revision" || len(loaded.Tasks) != 1 {
		t.Fatalf("unexpected source revision case: %#v", loaded)
	}
	task := loaded.Tasks[0]
	for _, required := range []string{"pnpm exec release:verify --mode locked", "800 ms"} {
		if !slices.Contains(task.MustInclude, required) {
			t.Fatalf("source revision task does not require %q: %#v", required, task)
		}
	}
	if !slices.Contains(task.MustNotInclude, "npm run release:verify -- --legacy") {
		t.Fatalf("source revision task does not forbid the stale command: %#v", task)
	}
}

func findBenchmark(entries []BenchmarkMapEntry, benchmark string) *BenchmarkMapEntry {
	for i := range entries {
		if string(entries[i].Benchmark) == benchmark {
			return &entries[i]
		}
	}
	return nil
}
