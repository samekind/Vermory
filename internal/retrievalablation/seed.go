package retrievalablation

import (
	"context"
	"fmt"
	"strings"

	"vermory/internal/memorybackend"
	"vermory/internal/runtime"
)

type SeededScope struct {
	TenantID     string              `json:"tenant_id"`
	ContinuityID string              `json:"continuity_id"`
	Line         string              `json:"line"`
	Anchor       string              `json:"anchor"`
	BackendScope memorybackend.Scope `json:"-"`
}

type SeededRecord struct {
	MemoryID  string `json:"memory_id"`
	ScopeID   string `json:"scope_id"`
	Lifecycle string `json:"lifecycle"`
}

type SeededCorpus struct {
	Scopes         map[string]SeededScope            `json:"scopes"`
	Records        map[string]SeededRecord           `json:"records"`
	BackendRecords map[string][]memorybackend.Record `json:"-"`
}

func SeedCorpus(
	ctx context.Context,
	store *runtime.Store,
	backend memorybackend.Backend,
	corpus Corpus,
	runID string,
) (SeededCorpus, error) {
	if store == nil || backend == nil {
		return SeededCorpus{}, fmt.Errorf("runtime store and vector backend are required")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return SeededCorpus{}, fmt.Errorf("run_id is required")
	}
	seeded := SeededCorpus{
		Scopes:         make(map[string]SeededScope, len(corpus.Scopes)),
		Records:        make(map[string]SeededRecord, len(corpus.Records)),
		BackendRecords: make(map[string][]memorybackend.Record, len(corpus.Scopes)),
	}
	for _, scope := range corpus.Scopes {
		seededScope, err := seedScope(ctx, store, scope)
		if err != nil {
			return SeededCorpus{}, fmt.Errorf("seed scope %q: %w", scope.ID, err)
		}
		seeded.Scopes[scope.ID] = seededScope
	}

	records := make(map[string]Record, len(corpus.Records))
	replacements := make(map[string]struct{})
	for _, record := range corpus.Records {
		records[record.ID] = record
		if record.Lifecycle == "superseded" {
			replacements[record.ReplacementID] = struct{}{}
		}
	}
	for _, record := range corpus.Records {
		if record.Lifecycle != "active" {
			continue
		}
		if _, delayed := replacements[record.ID]; delayed {
			continue
		}
		if _, err := seedGovernedRecord(ctx, store, seeded, record, runID, ""); err != nil {
			return SeededCorpus{}, err
		}
	}
	for _, record := range corpus.Records {
		if record.Lifecycle != "superseded" {
			continue
		}
		oldMemoryID, err := seedGovernedRecord(ctx, store, seeded, record, runID, "")
		if err != nil {
			return SeededCorpus{}, err
		}
		replacement := records[record.ReplacementID]
		if _, err := seedGovernedRecord(ctx, store, seeded, replacement, runID, oldMemoryID); err != nil {
			return SeededCorpus{}, err
		}
		seeded.Records[record.ID] = SeededRecord{MemoryID: oldMemoryID, ScopeID: record.ScopeID, Lifecycle: "superseded"}
	}
	for _, record := range corpus.Records {
		if record.Lifecycle == "active" || record.Lifecycle == "superseded" {
			continue
		}
		memoryID, err := seedGovernedRecord(ctx, store, seeded, record, runID, "")
		if err != nil {
			return SeededCorpus{}, err
		}
		if record.Lifecycle == "deleted" {
			scope := seeded.Scopes[record.ScopeID]
			if err := store.DeleteMemory(ctx, scope.TenantID, scope.ContinuityID, memoryID); err != nil {
				return SeededCorpus{}, fmt.Errorf("delete corpus record %q: %w", record.ID, err)
			}
		}
		seeded.Records[record.ID] = SeededRecord{MemoryID: memoryID, ScopeID: record.ScopeID, Lifecycle: record.Lifecycle}
	}

	for _, record := range corpus.Records {
		seededRecord := seeded.Records[record.ID]
		scope := seeded.Scopes[record.ScopeID]
		backendRecord := memorybackend.Record{
			ID:            seededRecord.MemoryID,
			Scope:         scope.BackendScope,
			SourceID:      record.ID,
			SourceVersion: 1,
			Status:        record.Lifecycle,
			Content:       record.Content,
			Metadata:      map[string]string{"record_id": record.ID},
		}
		switch record.Lifecycle {
		case "proposed":
			continue
		case "superseded", "deleted":
			backendRecord.Status = "active"
			if err := backend.Put(ctx, backendRecord); err != nil {
				return SeededCorpus{}, fmt.Errorf("seed removable vector record %q: %w", record.ID, err)
			}
			if err := backend.Delete(ctx, scope.BackendScope, backendRecord.ID); err != nil {
				return SeededCorpus{}, fmt.Errorf("remove ineligible vector record %q: %w", record.ID, err)
			}
			continue
		case "active":
			if err := backend.Put(ctx, backendRecord); err != nil {
				return SeededCorpus{}, fmt.Errorf("seed vector record %q: %w", record.ID, err)
			}
		}
		seeded.BackendRecords[record.ScopeID] = append(seeded.BackendRecords[record.ScopeID], backendRecord)
	}
	return seeded, nil
}

func seedScope(ctx context.Context, store *runtime.Store, scope Scope) (SeededScope, error) {
	var continuityID string
	switch scope.Line {
	case "workspace":
		resolved, err := store.ConfirmWorkspaceBinding(ctx, scope.TenantID, scope.Anchor)
		if err != nil {
			return SeededScope{}, err
		}
		continuityID = resolved
	case "conversation":
		channel, threadID, found := strings.Cut(scope.Anchor, ":")
		if !found || strings.TrimSpace(channel) == "" || strings.TrimSpace(threadID) == "" {
			return SeededScope{}, fmt.Errorf("conversation anchor must be channel:thread_id")
		}
		resolution, err := store.ResolveOrCreateConversation(ctx, scope.TenantID, runtime.ConversationAnchor{Channel: channel, ThreadID: threadID})
		if err != nil {
			return SeededScope{}, err
		}
		continuityID = resolution.ContinuityID
	default:
		return SeededScope{}, fmt.Errorf("unsupported scope line %q", scope.Line)
	}
	return SeededScope{
		TenantID:     scope.TenantID,
		ContinuityID: continuityID,
		Line:         scope.Line,
		Anchor:       scope.Anchor,
		BackendScope: memorybackend.Scope{TenantID: scope.TenantID, ContinuityID: continuityID, ContinuityLine: scope.Line},
	}, nil
}

func seedGovernedRecord(
	ctx context.Context,
	store *runtime.Store,
	seeded SeededCorpus,
	record Record,
	runID string,
	supersedesMemoryID string,
) (string, error) {
	if existing, exists := seeded.Records[record.ID]; exists {
		return existing.MemoryID, nil
	}
	scope, exists := seeded.Scopes[record.ScopeID]
	if !exists {
		return "", fmt.Errorf("record %q references unseeded scope %q", record.ID, record.ScopeID)
	}
	kind := runtime.ObservationKindSourceUpdate
	if record.Lifecycle == "proposed" {
		kind = runtime.ObservationKindAgentResult
	}
	request := runtime.CommitObservationRequest{
		OperationID:        runID + ":record:" + record.ID,
		Kind:               kind,
		Content:            record.Content,
		SourceRef:          "casebook:" + record.ProvenanceCase + "#" + record.ID,
		MemoryKey:          record.MemoryKey,
		SupersedesMemoryID: supersedesMemoryID,
	}
	if kind == runtime.ObservationKindAgentResult {
		request.SourceRef = ""
		request.MemoryKey = ""
	}
	receipt, err := store.CommitGovernedObservation(ctx, scope.TenantID, scope.ContinuityID, request)
	if err != nil {
		return "", fmt.Errorf("seed governed record %q: %w", record.ID, err)
	}
	seeded.Records[record.ID] = SeededRecord{MemoryID: receipt.Memory.MemoryID, ScopeID: record.ScopeID, Lifecycle: record.Lifecycle}
	return receipt.Memory.MemoryID, nil
}
