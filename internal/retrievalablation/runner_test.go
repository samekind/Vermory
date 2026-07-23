package retrievalablation

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"vermory/internal/memorybackend"
)

func TestRunWithDependenciesExecutesAllConditionsAndRebuild(t *testing.T) {
	store := openRetrievalTestStore(t)
	backend := newScriptedBackend()
	backend.orders["current command"] = []string{"current"}
	corpus := seedTestCorpus()

	report, err := RunWithDependencies(context.Background(), Options{RunID: "runner-pass", Corpus: corpus}, store, backend)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HardGates.Pass || !report.ProjectionRebuildEquivalent {
		t.Fatalf("runner gates mismatch: %#v", report)
	}
	if len(report.AuthorityFingerprint) != 64 || report.EngineVersion != "rrf-v1" || report.QualificationStatus != "measured" || report.RequestFingerprint != ReportRequestFingerprint(report) {
		t.Fatalf("runner identity mismatch: %#v", report)
	}
	for _, condition := range []string{ConditionLexical, ConditionVector, ConditionHybrid} {
		got := conditionReport(t, report, condition)
		if len(got.Queries) != 1 || got.Queries[0].Metrics.RecallAtK != 1 || got.Queries[0].Results[0].RecordID != "current" {
			t.Fatalf("condition %s mismatch: %#v", condition, got)
		}
	}
	if backend.rebuilds != len(corpus.Scopes) {
		t.Fatalf("rebuild count=%d, want %d", backend.rebuilds, len(corpus.Scopes))
	}
}

func TestRunWithDependenciesRejectsIneligibleVectorCandidates(t *testing.T) {
	store := openRetrievalTestStore(t)
	backend := newScriptedBackend()
	backend.orders["current command"] = []string{"old", "current"}

	report, err := RunWithDependencies(context.Background(), Options{RunID: "runner-ineligible", Corpus: seedTestCorpus()}, store, backend)
	if err != nil {
		t.Fatal(err)
	}
	vector := conditionReport(t, report, ConditionVector).Queries[0]
	if ids := rankedRecordIDs(vector.Results); !reflect.DeepEqual(ids, []string{"current"}) {
		t.Fatalf("ineligible vector result reached delivery: %v", ids)
	}
	if len(vector.RejectedResults) != 1 || vector.RejectedResults[0].RecordID != "old" || vector.RejectedResults[0].Eligible {
		t.Fatalf("rejected vector evidence mismatch: %#v", vector.RejectedResults)
	}
	if vector.Metrics.IneligibleCount != 1 || report.HardGates.Pass || report.HardGates.IneligibleCount != 1 {
		t.Fatalf("ineligible hard gate mismatch: query=%#v gates=%#v", vector, report.HardGates)
	}
}

func TestRunWithDependenciesRejectsForbiddenDeliveredResults(t *testing.T) {
	store := openRetrievalTestStore(t)
	backend := newScriptedBackend()
	backend.orders["current command"] = []string{"current"}

	corpus := seedTestCorpus()
	corpus.Queries[0].ForbiddenRecordIDs = []string{"current"}
	report, err := RunWithDependencies(context.Background(), Options{RunID: "runner-forbidden", Corpus: corpus}, store, backend)
	if err != nil {
		t.Fatal(err)
	}
	if report.HardGates.Pass || report.HardGates.ForbiddenCount != 3 {
		t.Fatalf("forbidden hard gate mismatch: gates=%#v", report.HardGates)
	}
}

func TestRunWithDependenciesDegradesOnlyFailedVectorQueries(t *testing.T) {
	store := openRetrievalTestStore(t)
	backend := newScriptedBackend()
	backend.orders["current command"] = []string{"current"}
	backend.failures["failing vector query"] = errors.New("embedding unavailable")
	corpus := seedTestCorpus()
	corpus.Queries = append(corpus.Queries, Query{
		ID: "failing", ScopeID: "workspace-a", Text: "failing vector query", Limit: 4,
		RelevantRecordIDs: []string{"current"}, ForbiddenRecordIDs: []string{"old", "proposed", "deleted", "other"}, Cohorts: []string{"failure"},
	})

	report, err := RunWithDependencies(context.Background(), Options{RunID: "runner-degraded", Corpus: corpus}, store, backend)
	if err != nil {
		t.Fatal(err)
	}
	vector := conditionReport(t, report, ConditionVector)
	hybrid := conditionReport(t, report, ConditionHybrid)
	if len(vector.Queries) != 2 || len(vector.Queries[0].Results) == 0 || vector.Queries[1].Error == "" {
		t.Fatalf("completed vector evidence was not preserved: %#v", vector.Queries)
	}
	if !hybrid.Queries[1].DegradedToLexical {
		t.Fatalf("hybrid query did not degrade: %#v", hybrid.Queries[1])
	}
	lexicalIDs := rankedRecordIDs(conditionReport(t, report, ConditionLexical).Queries[1].Results)
	hybridIDs := rankedRecordIDs(hybrid.Queries[1].Results)
	if !reflect.DeepEqual(lexicalIDs, hybridIDs) {
		t.Fatalf("degraded hybrid order=%v, lexical=%v", hybridIDs, lexicalIDs)
	}
	if len(report.Failures) != 1 || report.Failures[0].QueryID != "failing" {
		t.Fatalf("failure ledger mismatch: %#v", report.Failures)
	}
}

func conditionReport(t *testing.T, report Report, name string) ConditionReport {
	t.Helper()
	for _, condition := range report.Conditions {
		if condition.Name == name {
			return condition
		}
	}
	t.Fatalf("missing condition %q", name)
	return ConditionReport{}
}

type scriptedBackend struct {
	*recordingBackend
	orders   map[string][]string
	failures map[string]error
	rebuilds int
}

func newScriptedBackend() *scriptedBackend {
	return &scriptedBackend{
		recordingBackend: newRecordingBackend(),
		orders:           map[string][]string{},
		failures:         map[string]error{},
	}
}

func (b *scriptedBackend) Search(_ context.Context, query memorybackend.Query) ([]memorybackend.Result, error) {
	if err := b.failures[query.Text]; err != nil {
		return nil, err
	}
	byRecordID := map[string]memorybackend.Record{}
	for _, record := range b.records {
		if record.Scope == query.Scope {
			byRecordID[record.Metadata["record_id"]] = record
		}
	}
	for _, record := range b.archive {
		if record.Scope == query.Scope {
			byRecordID[record.Metadata["record_id"]] = record
		}
	}
	order := b.orders[query.Text]
	results := make([]memorybackend.Result, 0, len(order))
	for index, recordID := range order {
		record, exists := byRecordID[recordID]
		if !exists {
			continue
		}
		results = append(results, memorybackend.Result{Record: record, Score: 1 - float64(index)/100})
		if len(results) == query.Limit {
			break
		}
	}
	return results, nil
}

func (b *scriptedBackend) RebuildScope(ctx context.Context, scope memorybackend.Scope, records []memorybackend.Record) error {
	b.rebuilds++
	return b.recordingBackend.RebuildScope(ctx, scope, records)
}
