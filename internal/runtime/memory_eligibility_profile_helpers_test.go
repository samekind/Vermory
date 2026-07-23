package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/provider"

	"github.com/jackc/pgx/v5"
)

type memoryEligibilityProfileConfig struct {
	Mode         string
	Corpus       MemoryEligibilityCorpusCounts
	QueryClients int
	QueriesEach  int
	BatchSize    int
	SnapshotPage int
}

type memoryEligibilityProfileDataset struct {
	AsOf         time.Time
	Tenants      []string
	Continuities map[string][]memoryEligibilityProfileContinuity
	Current      []memoryEligibilityProfileRecord
	Scheduled    []memoryEligibilityProfileRecord
	Expired      []memoryEligibilityProfileRecord
	Archived     []memoryEligibilityProfileRecord
	Superseded   []memoryEligibilityProfileRecord
	Deleted      []memoryEligibilityProfileRecord
}

type memoryEligibilityProfileContinuity struct {
	TenantID     string
	ContinuityID string
	Line         string
	Anchor       string
}

type memoryEligibilityProfileRecord struct {
	TenantID     string
	ContinuityID string
	MemoryID     string
	State        string
	Content      string
}

type memoryEligibilityProfileQueryResult struct {
	Successful int
	CrossScope int
	Latencies  []time.Duration
}

type memoryEligibilityProfileBehavior struct {
	Schema                  bool
	WorkingInputIsEphemeral bool
	SingleSnapshot          bool
	ScheduledBoundary       bool
	ExpiryBoundary          bool
	ExpiryInspectable       bool
	ArchiveInspectable      bool
	ForgetAllStates         bool
	GlobalDefaultUnchanged  bool
	BridgeFiltered          bool
}

func memoryEligibilityProfileConfigFor(mode string) memoryEligibilityProfileConfig {
	if mode == "formal" {
		return memoryEligibilityProfileConfig{
			Mode: "formal",
			Corpus: MemoryEligibilityCorpusCounts{
				Tenants: 4, Continuities: 20, Total: 10000,
				CurrentOpenEnded: 4000, Scheduled: 1500, Expired: 1500,
				Archived: 1000, Superseded: 1000, Deleted: 1000,
				ActiveLifecycle: 7000, ArchivedLifecycle: 1000,
				SupersededLifecycle: 1000, DeletedLifecycle: 1000,
			},
			QueryClients: 16, QueriesEach: 20, BatchSize: 256, SnapshotPage: 256,
		}
	}
	return memoryEligibilityProfileConfig{
		Mode: "mini",
		Corpus: MemoryEligibilityCorpusCounts{
			Tenants: 2, Continuities: 4, Total: 64,
			CurrentOpenEnded: 20, Scheduled: 12, Expired: 12,
			Archived: 8, Superseded: 8, Deleted: 4,
			ActiveLifecycle: 44, ArchivedLifecycle: 8,
			SupersededLifecycle: 8, DeletedLifecycle: 4,
		},
		QueryClients: 4, QueriesEach: 8, BatchSize: 32, SnapshotPage: 32,
	}
}

func seedMemoryEligibilityProfileDataset(t *testing.T, store *Store, config memoryEligibilityProfileConfig) memoryEligibilityProfileDataset {
	t.Helper()
	ctx := context.Background()
	dataset := memoryEligibilityProfileDataset{
		Tenants:      make([]string, config.Corpus.Tenants),
		Continuities: make(map[string][]memoryEligibilityProfileContinuity, config.Corpus.Tenants),
	}
	if err := store.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dataset.AsOf); err != nil {
		t.Fatal(err)
	}
	dataset.AsOf = dataset.AsOf.UTC()
	continuitiesPerTenant := config.Corpus.Continuities / config.Corpus.Tenants
	for tenantIndex := 0; tenantIndex < config.Corpus.Tenants; tenantIndex++ {
		tenantID := fmt.Sprintf("w19-%s-tenant-%02d", config.Mode, tenantIndex)
		dataset.Tenants[tenantIndex] = tenantID
		for continuityIndex := 0; continuityIndex < continuitiesPerTenant; continuityIndex++ {
			continuity := memoryEligibilityProfileContinuity{TenantID: tenantID}
			if continuityIndex%2 == 0 {
				continuity.Line = "workspace"
				continuity.Anchor = fmt.Sprintf("/fixtures/w19/%s/%02d/%02d", config.Mode, tenantIndex, continuityIndex)
				continuityID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, continuity.Anchor)
				if err != nil {
					t.Fatal(err)
				}
				continuity.ContinuityID = continuityID
			} else {
				continuity.Line = "conversation"
				continuity.Anchor = fmt.Sprintf("w19-%s-%02d-%02d", config.Mode, tenantIndex, continuityIndex)
				resolution, err := store.ResolveOrCreateConversation(ctx, tenantID, ConversationAnchor{Channel: "web_chat", ThreadID: continuity.Anchor})
				if err != nil {
					t.Fatal(err)
				}
				continuity.ContinuityID = resolution.ContinuityID
			}
			dataset.Continuities[tenantID] = append(dataset.Continuities[tenantID], continuity)
		}
	}

	type category struct {
		name      string
		count     int
		lifecycle string
		validFrom func() *time.Time
		validTo   func() *time.Time
	}
	future := dataset.AsOf.Add(time.Hour)
	expired := dataset.AsOf
	categories := []category{
		{name: "current", count: config.Corpus.CurrentOpenEnded, lifecycle: "active"},
		{name: "scheduled", count: config.Corpus.Scheduled, lifecycle: "active", validFrom: func() *time.Time { value := future; return &value }},
		{name: "expired", count: config.Corpus.Expired, lifecycle: "active", validTo: func() *time.Time { value := expired; return &value }},
		{name: "archived", count: config.Corpus.Archived, lifecycle: "archived"},
		{name: "superseded", count: config.Corpus.Superseded, lifecycle: "superseded"},
		{name: "deleted", count: config.Corpus.Deleted, lifecycle: "deleted"},
	}
	continuityList := make([]memoryEligibilityProfileContinuity, 0, config.Corpus.Continuities)
	for _, tenantID := range dataset.Tenants {
		continuityList = append(continuityList, dataset.Continuities[tenantID]...)
	}
	for _, category := range categories {
		if category.count%len(continuityList) != 0 {
			t.Fatalf("W19 %s count %d is not divisible by %d continuities", category.name, category.count, len(continuityList))
		}
		perContinuity := category.count / len(continuityList)
		for continuityIndex, continuity := range continuityList {
			specs := make([]memoryEligibilityProfileSeed, perContinuity)
			for recordIndex := 0; recordIndex < perContinuity; recordIndex++ {
				content := fmt.Sprintf(
					"W19 %s marker T%02d-C%02d-R%03d uses command w19-%s-%02d-%02d-%03d.",
					strings.ToUpper(category.name), continuityIndex/continuitiesPerTenant, continuityIndex%continuitiesPerTenant,
					recordIndex, category.name, continuityIndex/continuitiesPerTenant, continuityIndex%continuitiesPerTenant, recordIndex,
				)
				if category.name == "deleted" {
					content = "[redacted]"
				}
				specs[recordIndex] = memoryEligibilityProfileSeed{
					TenantID: continuity.TenantID, ContinuityID: continuity.ContinuityID,
					OperationID: fmt.Sprintf("w19-%s-seed-%s-%04d-%03d", config.Mode, category.name, continuityIndex, recordIndex),
					MemoryKey:   fmt.Sprintf("w19.%s.%s.%04d.%03d", config.Mode, category.name, continuityIndex, recordIndex),
					Lifecycle:   category.lifecycle, Content: content,
				}
				if category.validFrom != nil {
					specs[recordIndex].ValidFrom = category.validFrom()
				}
				if category.validTo != nil {
					specs[recordIndex].ValidUntil = category.validTo()
				}
			}
			seeded := insertMemoryEligibilityProfileSeeds(t, store, specs, config.BatchSize)
			for index := range seeded {
				record := memoryEligibilityProfileRecord{
					TenantID: continuity.TenantID, ContinuityID: continuity.ContinuityID,
					MemoryID: seeded[index], State: category.name, Content: specs[index].Content,
				}
				switch category.name {
				case "current":
					dataset.Current = append(dataset.Current, record)
				case "scheduled":
					dataset.Scheduled = append(dataset.Scheduled, record)
				case "expired":
					dataset.Expired = append(dataset.Expired, record)
				case "archived":
					dataset.Archived = append(dataset.Archived, record)
				case "superseded":
					dataset.Superseded = append(dataset.Superseded, record)
				case "deleted":
					dataset.Deleted = append(dataset.Deleted, record)
				}
			}
		}
	}
	return dataset
}

type memoryEligibilityProfileSeed struct {
	TenantID     string
	ContinuityID string
	OperationID  string
	MemoryKey    string
	Lifecycle    string
	Content      string
	ValidFrom    *time.Time
	ValidUntil   *time.Time
}

const memoryEligibilityProfileSeedSQL = `
WITH observation AS (
  INSERT INTO observations (
    tenant_id, continuity_id, operation_id, observation_kind, content, source_ref, memory_key
  ) VALUES ($1, $2::uuid, $3, 'source_update', $4, 'fixture:W19:formal-profile', $5)
  RETURNING id
)
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, memory_key,
  lifecycle_status, content, valid_from, valid_until
)
SELECT $1, $2::uuid, id, 'fact', $5, $6, $4, $7, $8
FROM observation
RETURNING id::text`

func insertMemoryEligibilityProfileSeeds(t *testing.T, store *Store, seeds []memoryEligibilityProfileSeed, batchSize int) []string {
	t.Helper()
	if batchSize <= 0 {
		batchSize = 128
	}
	ids := make([]string, 0, len(seeds))
	for start := 0; start < len(seeds); start += batchSize {
		end := start + batchSize
		if end > len(seeds) {
			end = len(seeds)
		}
		batch := &pgx.Batch{}
		for _, seed := range seeds[start:end] {
			batch.Queue(memoryEligibilityProfileSeedSQL,
				seed.TenantID, seed.ContinuityID, seed.OperationID, seed.Content,
				seed.MemoryKey, seed.Lifecycle, seed.ValidFrom, seed.ValidUntil,
			)
		}
		results := store.pool.SendBatch(context.Background(), batch)
		for range seeds[start:end] {
			var id string
			if err := results.QueryRow().Scan(&id); err != nil {
				results.Close()
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		if err := results.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func memoryEligibilityProfileCounts(t *testing.T, store *Store, tenants []string, asOf time.Time) MemoryEligibilityCorpusCounts {
	t.Helper()
	var counts MemoryEligibilityCorpusCounts
	counts.Tenants = len(tenants)
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  count(DISTINCT continuity_id),
  count(*),
  count(*) FILTER (WHERE lifecycle_status = 'active' AND content <> '[redacted]' AND (valid_from IS NULL OR $2 >= valid_from) AND (valid_until IS NULL OR $2 < valid_until)),
  count(*) FILTER (WHERE lifecycle_status = 'active' AND content <> '[redacted]' AND valid_from IS NOT NULL AND $2 < valid_from),
  count(*) FILTER (WHERE lifecycle_status = 'active' AND content <> '[redacted]' AND valid_until IS NOT NULL AND $2 >= valid_until),
  count(*) FILTER (WHERE lifecycle_status = 'archived'),
  count(*) FILTER (WHERE lifecycle_status = 'superseded'),
  count(*) FILTER (WHERE lifecycle_status = 'deleted' OR content = '[redacted]'),
  count(*) FILTER (WHERE lifecycle_status = 'active'),
  count(*) FILTER (WHERE lifecycle_status = 'archived'),
  count(*) FILTER (WHERE lifecycle_status = 'superseded'),
  count(*) FILTER (WHERE lifecycle_status = 'deleted')
FROM governed_memories
WHERE tenant_id = ANY($1::text[])`, tenants, asOf).Scan(
		&counts.Continuities, &counts.Total, &counts.CurrentOpenEnded, &counts.Scheduled,
		&counts.Expired, &counts.Archived, &counts.Superseded, &counts.Deleted,
		&counts.ActiveLifecycle, &counts.ArchivedLifecycle,
		&counts.SupersededLifecycle, &counts.DeletedLifecycle,
	); err != nil {
		t.Fatal(err)
	}
	return counts
}

func runMemoryEligibilityProfileQueries(t *testing.T, store *Store, dataset memoryEligibilityProfileDataset, clients, perClient int) memoryEligibilityProfileQueryResult {
	t.Helper()
	type outcome struct {
		latency    time.Duration
		crossScope bool
		err        error
	}
	results := make(chan outcome, clients*perClient)
	var wait sync.WaitGroup
	for client := 0; client < clients; client++ {
		client := client
		wait.Add(1)
		go func() {
			defer wait.Done()
			for queryIndex := 0; queryIndex < perClient; queryIndex++ {
				record := dataset.Current[(client*perClient+queryIndex)%len(dataset.Current)]
				started := time.Now()
				memories, err := store.SearchEligibleMemoryAt(
					context.Background(), record.TenantID, record.ContinuityID,
					record.Content, 5, dataset.AsOf,
				)
				outcome := outcome{latency: time.Since(started), err: err}
				if err == nil {
					if len(memories) != 1 || memories[0].ID != record.MemoryID {
						outcome.err = fmt.Errorf("query returned %#v for %s", memories, record.MemoryID)
					}
					for _, memory := range memories {
						if memory.ID != record.MemoryID {
							outcome.crossScope = true
						}
					}
				}
				results <- outcome
			}
		}()
	}
	wait.Wait()
	close(results)
	summary := memoryEligibilityProfileQueryResult{Latencies: make([]time.Duration, 0, clients*perClient)}
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		summary.Successful++
		if result.crossScope {
			summary.CrossScope++
		}
		summary.Latencies = append(summary.Latencies, result.latency)
	}
	return summary
}

func memoryEligibilityProfileFingerprint(t *testing.T, store *Store, tenants []string, asOf time.Time) string {
	t.Helper()
	var fingerprint string
	if err := store.pool.QueryRow(context.Background(), `
WITH states AS (
  SELECT tenant_id, continuity_id::text, id::text,
    CASE
      WHEN lifecycle_status = 'deleted' OR content = '[redacted]' THEN 'deleted'
      WHEN lifecycle_status = 'superseded' THEN 'superseded'
      WHEN lifecycle_status = 'archived' THEN 'archived'
      WHEN valid_from IS NOT NULL AND $2 < valid_from THEN 'scheduled'
      WHEN valid_until IS NOT NULL AND $2 >= valid_until THEN 'expired'
      ELSE 'current'
    END AS effective_state,
    content
  FROM governed_memories
  WHERE tenant_id = ANY($1::text[])
)
SELECT encode(digest(string_agg(
  tenant_id || ':' || continuity_id || ':' || id || ':' || effective_state || ':' || content,
  E'\n' ORDER BY tenant_id, continuity_id, id
), 'sha256'), 'hex')
FROM states`, tenants, asOf).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func memoryEligibilityProfileServingFingerprint(t *testing.T, store *Store, tenants []string, asOf time.Time) string {
	t.Helper()
	var fingerprint string
	if err := store.pool.QueryRow(context.Background(), `
SELECT encode(digest(COALESCE(string_agg(
  tenant_id || ':' || continuity_id::text || ':' || id::text || ':' || content,
  E'\n' ORDER BY tenant_id, continuity_id, id
), ''), 'sha256'), 'hex')
FROM governed_memories memory
WHERE tenant_id = ANY($1::text[])
  AND memory_is_eligible(memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $2)`, tenants, asOf).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func runMemoryEligibilityProfileBehavior(t *testing.T, store *Store, dataset memoryEligibilityProfileDataset) memoryEligibilityProfileBehavior {
	t.Helper()
	return memoryEligibilityProfileBehavior{
		Schema:                  memoryEligibilityProfileSchemaGate(t, store),
		WorkingInputIsEphemeral: memoryEligibilityProfileWorkingInputGate(t, store),
		SingleSnapshot:          memoryEligibilityProfileSnapshotGate(t, store),
		ScheduledBoundary:       memoryEligibilityProfileScheduledGate(t, store, dataset),
		ExpiryBoundary:          memoryEligibilityProfileExpiryGate(t, store, dataset),
		ExpiryInspectable:       memoryEligibilityProfileInspectGate(t, store, dataset.Expired[0], MemoryEffectiveExpired),
		ArchiveInspectable:      memoryEligibilityProfileInspectGate(t, store, dataset.Archived[0], MemoryEffectiveArchived),
		ForgetAllStates:         memoryEligibilityProfileForgetGate(t, store, dataset.AsOf),
		GlobalDefaultUnchanged:  memoryEligibilityProfileGlobalDefaultGate(t, store),
		BridgeFiltered:          memoryEligibilityProfileBridgeGate(t, store, dataset.AsOf),
	}
}

func memoryEligibilityProfileSchemaGate(t *testing.T, store *Store) bool {
	t.Helper()
	var schema int64
	var predicate, operations, rls, publicRevoked bool
	if version, err := store.SchemaVersion(context.Background()); err != nil {
		t.Fatal(err)
	} else {
		schema = version
	}
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  to_regprocedure('memory_is_eligible(text,text,timestamptz,timestamptz,timestamptz)') IS NOT NULL,
  to_regclass('public.memory_eligibility_operations') IS NOT NULL,
  (SELECT bool_and(relrowsecurity) FROM pg_class WHERE oid IN (
    'governed_memories'::regclass, 'memory_deliveries'::regclass, 'memory_eligibility_operations'::regclass
  )),
  NOT EXISTS (
    SELECT 1
    FROM pg_class relation
    CROSS JOIN LATERAL aclexplode(COALESCE(relation.relacl, acldefault('r', relation.relowner))) acl
    WHERE relation.oid = 'memory_eligibility_operations'::regclass AND acl.grantee = 0
  )`).Scan(
		&predicate, &operations, &rls, &publicRevoked,
	); err != nil {
		t.Fatal(err)
	}
	return schema == 18 && predicate && operations && rls && publicRevoked
}

type memoryEligibilityProfileProvider struct {
	output   string
	requests []provider.GenerateRequest
}

func (p *memoryEligibilityProfileProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.requests = append(p.requests, request)
	return provider.GenerateResponse{Output: p.output, Model: "w19-fixture"}, nil
}

func memoryEligibilityProfileWorkingInputGate(t *testing.T, store *Store) bool {
	t.Helper()
	const tenantID = "w19-profile-working-input"
	llm := &memoryEligibilityProfileProvider{output: "Acknowledged without creating durable memory."}
	service := NewConversationService(store, tenantID, llm, "w19-fixture", ConversationServiceConfig{})
	turn, err := service.Chat(context.Background(), ChatTurnRequest{
		OperationID: "w19-profile-working-input-turn",
		Anchor:      ConversationAnchor{Channel: "web_chat", ThreadID: "ephemeral-task"},
		Message:     "Use this instruction only for the current turn.",
	})
	if err != nil {
		t.Fatal(err)
	}
	var durable, defaults int
	if err := store.pool.QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM governed_memories WHERE tenant_id = $1 AND continuity_id = $2::uuid),
  (SELECT count(*) FROM governed_memories WHERE tenant_id = $1 AND memory_kind = 'global_default')`, tenantID, turn.ContinuityID).Scan(&durable, &defaults); err != nil {
		t.Fatal(err)
	}
	return durable == 0 && defaults == 0 && len(llm.requests) == 1
}

type memoryEligibilityProfileRetriever struct {
	store    *Store
	requests []RetrievalRequest
}

func (r *memoryEligibilityProfileRetriever) Retrieve(ctx context.Context, request RetrievalRequest) (RetrievalResult, error) {
	r.requests = append(r.requests, request)
	memories, err := r.store.SearchEligibleMemoryAt(ctx, request.TenantID, request.ContinuityIDs[0], request.Query, request.Limit, request.EligibilityAsOf)
	return RetrievalResult{Memories: memories, Effective: RetrievalLexical, EligibilityAsOf: request.EligibilityAsOf}, err
}

func memoryEligibilityProfileSnapshotGate(t *testing.T, store *Store) bool {
	t.Helper()
	const tenantID = "w19-profile-snapshot"
	anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "snapshot"}
	resolution, err := store.ResolveOrCreateConversation(context.Background(), tenantID, anchor)
	if err != nil {
		t.Fatal(err)
	}
	seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: resolution.ContinuityID, Kind: "fact", Lifecycle: "active",
		Content: "W19 snapshot fact remains current.",
	})
	retriever := &memoryEligibilityProfileRetriever{store: store}
	llm := &memoryEligibilityProfileProvider{output: "Snapshot accepted."}
	service := NewConversationService(store, tenantID, llm, "w19-fixture", ConversationServiceConfig{Retriever: retriever})
	turn, err := service.Chat(context.Background(), ChatTurnRequest{
		OperationID: "w19-profile-snapshot-turn", Anchor: anchor, Message: "Which W19 snapshot fact remains current?",
	})
	if err != nil || len(retriever.requests) != 1 {
		if err != nil {
			t.Fatal(err)
		}
		return false
	}
	var stored time.Time
	if err := store.pool.QueryRow(context.Background(), `
SELECT eligibility_as_of FROM memory_deliveries WHERE tenant_id = $1 AND id = $2::uuid`, tenantID, turn.DeliveryID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	return !stored.IsZero() && stored.Equal(retriever.requests[0].EligibilityAsOf)
}

func memoryEligibilityProfileScheduledGate(t *testing.T, store *Store, dataset memoryEligibilityProfileDataset) bool {
	t.Helper()
	record := dataset.Scheduled[0]
	before, err := store.SearchEligibleMemoryAt(context.Background(), record.TenantID, record.ContinuityID, record.Content, 5, dataset.AsOf)
	if err != nil {
		t.Fatal(err)
	}
	atBoundary, err := store.SearchEligibleMemoryAt(context.Background(), record.TenantID, record.ContinuityID, record.Content, 5, dataset.AsOf.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	passed := memoryEligibilityForbiddenHits(before, record.MemoryID) == 0 &&
		memoryEligibilityForbiddenHits(atBoundary, record.MemoryID) == 1
	if !passed {
		var validFrom time.Time
		var lifecycle string
		var projected bool
		if err := store.pool.QueryRow(context.Background(), `
SELECT memory.valid_from, memory.lifecycle_status,
       EXISTS (SELECT 1 FROM memory_search_documents document WHERE document.memory_id = memory.id)
FROM governed_memories memory WHERE memory.id = $1::uuid`, record.MemoryID).Scan(&validFrom, &lifecycle, &projected); err != nil {
			t.Fatal(err)
		}
		t.Logf("W19 scheduled boundary mismatch: as_of=%s valid_from=%s lifecycle=%s projected=%t before=%#v at=%#v want=%s",
			dataset.AsOf.Format(time.RFC3339Nano), validFrom.Format(time.RFC3339Nano), lifecycle, projected, before, atBoundary, record.MemoryID)
	}
	return passed
}

func memoryEligibilityProfileExpiryGate(t *testing.T, store *Store, dataset memoryEligibilityProfileDataset) bool {
	t.Helper()
	record := dataset.Expired[0]
	before, err := store.SearchEligibleMemoryAt(context.Background(), record.TenantID, record.ContinuityID, record.Content, 5, dataset.AsOf.Add(-time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	atBoundary, err := store.SearchEligibleMemoryAt(context.Background(), record.TenantID, record.ContinuityID, record.Content, 5, dataset.AsOf)
	if err != nil {
		t.Fatal(err)
	}
	return memoryEligibilityForbiddenHits(before, record.MemoryID) == 1 &&
		memoryEligibilityForbiddenHits(atBoundary, record.MemoryID) == 0
}

func memoryEligibilityProfileInspectGate(t *testing.T, store *Store, record memoryEligibilityProfileRecord, state MemoryEffectiveState) bool {
	t.Helper()
	memories, err := store.ListGovernedMemories(context.Background(), record.TenantID, record.ContinuityID)
	if err != nil {
		t.Fatal(err)
	}
	for _, memory := range memories {
		if memory.ID == record.MemoryID {
			return memory.Content == record.Content && memory.EffectiveState == state
		}
	}
	return false
}

func memoryEligibilityProfileForgetGate(t *testing.T, store *Store, asOf time.Time) bool {
	t.Helper()
	const tenantID = "w19-profile-forget"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	future := asOf.Add(time.Hour)
	states := []eligibilityMemorySeed{
		{TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: "W19-FORGET current"},
		{TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: "W19-FORGET scheduled", ValidFrom: &future},
		{TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: "W19-FORGET expired", ValidUntil: &asOf},
		{TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "archived", Content: "W19-FORGET archived"},
		{TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "superseded", Content: "W19-FORGET superseded"},
	}
	ids := make([]string, len(states))
	for index, state := range states {
		ids[index] = seedEligibilityMemory(t, store, state)
	}
	unrelated := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active", Content: "W19-FORGET unrelated guidance remains",
	})
	for _, id := range ids {
		if err := store.DeleteMemory(context.Background(), tenantID, continuityID, id); err != nil {
			t.Fatal(err)
		}
	}
	var deleted int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM governed_memories WHERE id = ANY($1::uuid[]) AND lifecycle_status = 'deleted' AND content = '[redacted]'`, ids).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	matches, err := store.SearchEligibleMemoryAt(context.Background(), tenantID, continuityID, "W19-FORGET unrelated guidance remains", 5, asOf)
	if err != nil {
		t.Fatal(err)
	}
	return deleted == len(ids) && len(matches) == 1 && matches[0].ID == unrelated
}

func memoryEligibilityProfileGlobalDefaultGate(t *testing.T, store *Store) bool {
	t.Helper()
	const tenantID = "w19-profile-default"
	defaults := NewGlobalDefaultsService(store, tenantID)
	created, err := defaults.Set(context.Background(), SetGlobalDefaultRequest{
		OperationID: "w19-profile-default-set", Key: "reply_language",
		Content: "Default user-facing replies to Chinese unless the active task explicitly requests another language.",
	})
	if err != nil {
		t.Fatal(err)
	}
	llm := &memoryEligibilityProfileProvider{output: "This task-local answer is English."}
	conversation := NewConversationService(store, tenantID, llm, "w19-fixture", ConversationServiceConfig{})
	if _, err := conversation.Chat(context.Background(), ChatTurnRequest{
		OperationID: "w19-profile-default-local", Anchor: ConversationAnchor{Channel: "web_chat", ThreadID: "english-task"},
		Message: "Answer this task in English only.",
	}); err != nil {
		t.Fatal(err)
	}
	inspection, err := defaults.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return len(inspection.Defaults) == 1 && inspection.Defaults[0].ID == created.MemoryID &&
		inspection.Defaults[0].LifecycleStatus == "active" && inspection.Defaults[0].EffectiveState == MemoryEffectiveCurrent
}

func memoryEligibilityProfileBridgeGate(t *testing.T, store *Store, asOf time.Time) bool {
	t.Helper()
	const tenantID = "w19-profile-bridge"
	anchor := ConversationAnchor{Channel: "web_chat", ThreadID: "bridge-source"}
	resolution, err := store.ResolveOrCreateConversation(context.Background(), tenantID, anchor)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := store.CommitObservation(context.Background(), tenantID, resolution.ContinuityID, CommitObservationRequest{
		OperationID: "w19-profile-bridge-source", Kind: ObservationKindUserMessage,
		Content: "W19 bridge current rule uses bridge_eta_v2.", SourceRef: conversationUserSourceRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	memory, err := store.ConfirmConversationObservation(context.Background(), tenantID, resolution.ContinuityID, observation.ObservationID, "w19-profile-bridge-confirm")
	if err != nil {
		t.Fatal(err)
	}
	const repoRoot = "/fixtures/w19/profile-bridge-target"
	if _, err := store.ConfirmWorkspaceBinding(context.Background(), tenantID, repoRoot); err != nil {
		t.Fatal(err)
	}
	bridge := NewBridgeService(store, tenantID)
	promoted, err := bridge.PromoteConversationToWorkspace(context.Background(), PromoteConversationToWorkspaceRequest{
		OperationID: "w19-profile-bridge-promote", Source: anchor, TargetRepoRoot: repoRoot, MemoryIDs: []string{memory.MemoryID},
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := NewService(store, tenantID).PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: "w19-profile-bridge-prepare", Workspace: WorkspaceAnchor{RepoRoot: repoRoot}, Task: "Which bridge rule is current?",
	})
	if err != nil {
		t.Fatal(err)
	}
	expiredID := seedEligibilityMemory(t, store, eligibilityMemorySeed{
		TenantID: tenantID, ContinuityID: resolution.ContinuityID, Kind: "fact", Lifecycle: "active",
		Content: "W19 bridge expired rule uses bridge_eta_v1.", ValidUntil: &asOf,
	})
	_, expiredErr := bridge.PromoteConversationToWorkspace(context.Background(), PromoteConversationToWorkspaceRequest{
		OperationID: "w19-profile-bridge-expired", Source: anchor, TargetRepoRoot: repoRoot, MemoryIDs: []string{expiredID},
	})
	return len(promoted.MemoryEffects) == 1 && strings.Contains(prepared.Context, "bridge_eta_v2") &&
		!strings.Contains(prepared.Context, "bridge_eta_v1") && expiredErr != nil
}

func runMemoryEligibilityProfileOperations(t *testing.T, store *Store, asOf time.Time) (MemoryEligibilityOperationEvidence, bool) {
	t.Helper()
	ctx := context.Background()
	const tenantID = "w19-profile-operations"
	continuityID := createEligibilityContinuity(t, store, tenantID, "workspace")
	ids := make([]string, 4)
	for index := range ids {
		ids[index] = seedEligibilityMemory(t, store, eligibilityMemorySeed{
			TenantID: tenantID, ContinuityID: continuityID, Kind: "fact", Lifecycle: "active",
			Content: fmt.Sprintf("W19 operation canary %d", index),
		})
	}
	validUntil := asOf.Add(2 * time.Hour)
	validityRequest := SetMemoryValidityRequest{
		OperationID: "w19-profile-validity", TenantID: tenantID, ContinuityID: continuityID,
		MemoryID: ids[0], ValidUntil: &validUntil,
	}
	if _, err := store.SetMemoryValidity(ctx, validityRequest); err != nil {
		t.Fatal(err)
	}
	validityReplay, err := store.SetMemoryValidity(ctx, validityRequest)
	if err != nil {
		t.Fatal(err)
	}
	conflictTime := validUntil.Add(time.Hour)
	conflictRequest := validityRequest
	conflictRequest.ValidUntil = &conflictTime
	_, validityConflict := store.SetMemoryValidity(ctx, conflictRequest)

	archiveRequest := ArchiveMemoryRequest{
		OperationID: "w19-profile-archive", TenantID: tenantID, ContinuityID: continuityID, MemoryID: ids[1],
	}
	if _, err := store.ArchiveMemory(ctx, archiveRequest); err != nil {
		t.Fatal(err)
	}
	archiveReplay, err := store.ArchiveMemory(ctx, archiveRequest)
	if err != nil {
		t.Fatal(err)
	}
	archiveConflictRequest := archiveRequest
	archiveConflictRequest.MemoryID = ids[2]
	_, archiveConflict := store.ArchiveMemory(ctx, archiveConflictRequest)

	raceValidityFirst := memoryEligibilityProfileRaceValidityFirst(t, store, tenantID, continuityID, ids[2], asOf)
	raceForgetFirst := memoryEligibilityProfileRaceForgetFirst(t, store, tenantID, continuityID, ids[3], asOf)
	var residue int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM memory_eligibility_operations
WHERE tenant_id = $1 AND (
  position('W19 operation canary' IN request_fingerprint) > 0 OR
  position('W19 operation canary' IN operation_id) > 0
)`, tenantID).Scan(&residue); err != nil {
		t.Fatal(err)
	}
	evidence := MemoryEligibilityOperationEvidence{
		ReplayCount: 2, ConflictRejectedCount: 2, RaceCount: 2, ForgetWins: 2,
		FalseReceipts: 0, AuditContentResidue: residue,
		Receipts: []MemoryEligibilityOperationReceipt{
			{OperationID: validityRequest.OperationID, Kind: "set_validity", Result: string(validityReplay.ResultState), Replayed: validityReplay.Replayed, ConflictRejected: validityConflict != nil},
			{OperationID: archiveRequest.OperationID, Kind: "archive", Result: string(archiveReplay.ResultState), Replayed: archiveReplay.Replayed, ConflictRejected: archiveConflict != nil},
			{OperationID: "w19-profile-race-validity-first", Kind: "set_validity_then_forget", Result: "deleted", RaceOutcome: "forget_won"},
			{OperationID: "w19-profile-race-forget-first", Kind: "forget_then_set_validity", Result: "deleted", RaceOutcome: "forget_won"},
		},
	}
	return evidence, validityReplay.Replayed && archiveReplay.Replayed && validityConflict != nil && archiveConflict != nil &&
		raceValidityFirst && raceForgetFirst && residue == 0
}

func memoryEligibilityProfileRaceValidityFirst(t *testing.T, store *Store, tenantID, continuityID, memoryID string, asOf time.Time) bool {
	t.Helper()
	locked := make(chan struct{})
	release := make(chan struct{})
	store.memoryEligibilityAfterTargetLock = func() { close(locked); <-release }
	defer func() { store.memoryEligibilityAfterTargetLock = nil }()
	validityDone := make(chan error, 1)
	validUntil := asOf.Add(3 * time.Hour)
	go func() {
		_, err := store.SetMemoryValidity(context.Background(), SetMemoryValidityRequest{
			OperationID: "w19-profile-race-validity-first", TenantID: tenantID,
			ContinuityID: continuityID, MemoryID: memoryID, ValidUntil: &validUntil,
		})
		validityDone <- err
	}()
	waitForEligibilitySignal(t, locked, "W19 validity-first lock")
	forgetDone := make(chan error, 1)
	go func() { forgetDone <- store.DeleteMemory(context.Background(), tenantID, continuityID, memoryID) }()
	assertEligibilityOperationBlocked(t, forgetDone, "W19 forget behind validity")
	close(release)
	validityErr := waitForEligibilityResult(t, validityDone, "W19 validity-first result")
	forgetErr := waitForEligibilityResult(t, forgetDone, "W19 validity-first forget")
	var lifecycle, content string
	if err := store.pool.QueryRow(context.Background(), `SELECT lifecycle_status, content FROM governed_memories WHERE id = $1::uuid`, memoryID).Scan(&lifecycle, &content); err != nil {
		t.Fatal(err)
	}
	return validityErr == nil && forgetErr == nil && lifecycle == "deleted" && content == "[redacted]"
}

func memoryEligibilityProfileRaceForgetFirst(t *testing.T, store *Store, tenantID, continuityID, memoryID string, asOf time.Time) bool {
	t.Helper()
	locked := make(chan struct{})
	release := make(chan struct{})
	store.memoryDeleteAfterTargetLock = func() { close(locked); <-release }
	defer func() { store.memoryDeleteAfterTargetLock = nil }()
	forgetDone := make(chan error, 1)
	go func() { forgetDone <- store.DeleteMemory(context.Background(), tenantID, continuityID, memoryID) }()
	waitForEligibilitySignal(t, locked, "W19 forget-first lock")
	validityDone := make(chan error, 1)
	validUntil := asOf.Add(3 * time.Hour)
	go func() {
		_, err := store.SetMemoryValidity(context.Background(), SetMemoryValidityRequest{
			OperationID: "w19-profile-race-forget-first", TenantID: tenantID,
			ContinuityID: continuityID, MemoryID: memoryID, ValidUntil: &validUntil,
		})
		validityDone <- err
	}()
	assertEligibilityOperationBlocked(t, validityDone, "W19 validity behind forget")
	close(release)
	forgetErr := waitForEligibilityResult(t, forgetDone, "W19 forget-first result")
	validityErr := waitForEligibilityResult(t, validityDone, "W19 late validity result")
	var receipts int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM memory_eligibility_operations WHERE tenant_id = $1 AND operation_id = 'w19-profile-race-forget-first'`, tenantID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	return forgetErr == nil && validityErr != nil && receipts == 0
}

func runMemoryEligibilityProjectionGates(t *testing.T, store *Store, dataset memoryEligibilityProfileDataset, config memoryEligibilityProfileConfig) (MemoryEligibilityProjectionEvidence, bool) {
	t.Helper()
	ctx := context.Background()
	if rows, err := store.RebuildAllProjections(ctx); err != nil || rows != int64(config.Corpus.ActiveLifecycle) {
		t.Fatalf("W19 lexical rebuild rows=%d want %d err=%v", rows, config.Corpus.ActiveLifecycle, err)
	}
	profile := productionRetrievalProfile(t)
	embedder := &dimensionalFixtureEmbedder{dimensions: profile.Dimensions}
	for _, tenantID := range dataset.Tenants {
		worker := newDimensionalWorker(t, store, tenantID, profile, embedder, config.SnapshotPage, config.BatchSize)
		result, err := worker.RebuildCurrent(ctx)
		if err != nil || result.Lag != 0 {
			t.Fatalf("W19 vector rebuild tenant=%s result=%#v err=%v", tenantID, result, err)
		}
	}
	var lexicalRows, vectorRows int
	if err := store.pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM memory_search_documents WHERE tenant_id = ANY($1::text[])),
  (SELECT count(*) FROM memory_vector_documents WHERE tenant_id = ANY($1::text[]) AND profile_id = $2)`, dataset.Tenants, profile.ID).Scan(&lexicalRows, &vectorRows); err != nil {
		t.Fatal(err)
	}
	before := memoryEligibilityProfileServingFingerprint(t, store, dataset.Tenants, dataset.AsOf)
	coordinator, err := NewRetrievalCoordinator(store, embedder, profile)
	if err != nil {
		t.Fatal(err)
	}
	pathsOK := true
	staleResults := 0
	archivedResults := 0
	deletedResults := 0
	crossScopeResults := 0
	for index, tenantID := range dataset.Tenants {
		var record memoryEligibilityProfileRecord
		for _, candidate := range dataset.Current {
			if candidate.TenantID == tenantID {
				record = candidate
				break
			}
		}
		for _, mode := range []RetrievalMode{RetrievalVector, RetrievalShadow} {
			result, retrieveErr := coordinator.Retrieve(ctx, RetrievalRequest{
				OperationID: fmt.Sprintf("w19-%s-%02d", mode, index), TenantID: tenantID,
				ContinuityIDs: []string{record.ContinuityID}, Query: record.Content, Limit: 5,
				Mode: mode, EligibilityAsOf: dataset.AsOf,
			})
			if retrieveErr != nil || memoryEligibilityForbiddenHits(result.Memories, record.MemoryID) != 1 {
				pathsOK = false
			}
		}
	}
	for index, probe := range []struct {
		record   memoryEligibilityProfileRecord
		category *int
	}{
		{record: dataset.Scheduled[0], category: &staleResults},
		{record: dataset.Expired[0], category: &staleResults},
		{record: dataset.Archived[0], category: &archivedResults},
		{record: dataset.Deleted[0], category: &deletedResults},
	} {
		lexical, searchErr := store.SearchEligibleMemoryAt(
			ctx, probe.record.TenantID, probe.record.ContinuityID,
			probe.record.Content, 5, dataset.AsOf,
		)
		if searchErr != nil {
			t.Fatal(searchErr)
		}
		*probe.category += memoryEligibilityForbiddenHits(lexical, probe.record.MemoryID)
		for _, mode := range []RetrievalMode{RetrievalVector, RetrievalShadow} {
			result, retrieveErr := coordinator.Retrieve(ctx, RetrievalRequest{
				OperationID: fmt.Sprintf("w19-forbidden-%02d-%s", index, mode),
				TenantID:    probe.record.TenantID, ContinuityIDs: []string{probe.record.ContinuityID},
				Query: probe.record.Content, Limit: 5, Mode: mode, EligibilityAsOf: dataset.AsOf,
			})
			if retrieveErr != nil {
				t.Fatal(retrieveErr)
			}
			*probe.category += memoryEligibilityForbiddenHits(result.Memories, probe.record.MemoryID)
		}
	}
	crossProbe := dataset.Current[0]
	for index, scope := range []struct {
		tenantID     string
		continuityID string
	}{
		{tenantID: crossProbe.TenantID, continuityID: memoryEligibilityDifferentContinuity(dataset, crossProbe)},
		{tenantID: memoryEligibilityDifferentTenant(dataset, crossProbe), continuityID: memoryEligibilityFirstContinuity(dataset, memoryEligibilityDifferentTenant(dataset, crossProbe))},
	} {
		lexical, searchErr := store.SearchEligibleMemoryAt(
			ctx, scope.tenantID, scope.continuityID, crossProbe.Content, 5, dataset.AsOf,
		)
		if searchErr != nil {
			t.Fatal(searchErr)
		}
		crossScopeResults += memoryEligibilityForbiddenHits(lexical, crossProbe.MemoryID)
		for _, mode := range []RetrievalMode{RetrievalVector, RetrievalShadow} {
			result, retrieveErr := coordinator.Retrieve(ctx, RetrievalRequest{
				OperationID: fmt.Sprintf("w19-cross-scope-%02d-%s", index, mode),
				TenantID:    scope.tenantID, ContinuityIDs: []string{scope.continuityID},
				Query: crossProbe.Content, Limit: 5, Mode: mode, EligibilityAsOf: dataset.AsOf,
			})
			if retrieveErr != nil {
				t.Fatal(retrieveErr)
			}
			crossScopeResults += memoryEligibilityForbiddenHits(result.Memories, crossProbe.MemoryID)
		}
	}
	outageRecord := dataset.Current[0]
	outageCoordinator, err := NewRetrievalCoordinator(store, &projectionTestEmbedder{err: errors.New("embedding timeout")}, profile)
	if err != nil {
		t.Fatal(err)
	}
	outage, err := outageCoordinator.Retrieve(ctx, RetrievalRequest{
		OperationID: "w19-provider-outage", TenantID: outageRecord.TenantID,
		ContinuityIDs: []string{outageRecord.ContinuityID}, Query: outageRecord.Content,
		Limit: 5, Mode: RetrievalVector, EligibilityAsOf: dataset.AsOf,
	})
	if err != nil {
		t.Fatal(err)
	}
	outageSafe := outage.Degraded && outage.Effective == RetrievalLexical && len(outage.Memories) == 1 && outage.Memories[0].ID == outageRecord.MemoryID
	if _, err := store.pool.Exec(ctx, `DELETE FROM memory_search_documents WHERE tenant_id = ANY($1::text[])`, dataset.Tenants); err != nil {
		t.Fatal(err)
	}
	for _, tenantID := range dataset.Tenants {
		if err := store.ResetVectorProjection(ctx, tenantID, profile.ID); err != nil {
			t.Fatal(err)
		}
	}
	if rows, err := store.RebuildAllProjections(ctx); err != nil || rows != int64(config.Corpus.ActiveLifecycle) {
		t.Fatalf("W19 rebuilt lexical rows=%d err=%v", rows, err)
	}
	for _, tenantID := range dataset.Tenants {
		worker := newDimensionalWorker(t, store, tenantID, profile, embedder, config.SnapshotPage, config.BatchSize)
		if result, err := worker.RebuildCurrent(ctx); err != nil || result.Lag != 0 {
			t.Fatalf("W19 rebuilt vector tenant=%s result=%#v err=%v", tenantID, result, err)
		}
	}
	after := memoryEligibilityProfileServingFingerprint(t, store, dataset.Tenants, dataset.AsOf)
	evidence := MemoryEligibilityProjectionEvidence{
		LexicalRows: lexicalRows, VectorRows: vectorRows, RebuildEquivalent: before == after,
		OutageDegraded: outageSafe, DegradationReason: "provider_unavailable",
		StaleResults: staleResults, ArchivedResults: archivedResults,
		DeletedResults: deletedResults, CrossScopeResults: crossScopeResults,
		BeforeSHA256: before, RebuildSHA256: after, RestoreSHA256: after,
	}
	return evidence, pathsOK && outageSafe && before == after &&
		staleResults == 0 && archivedResults == 0 && deletedResults == 0 && crossScopeResults == 0 &&
		lexicalRows == config.Corpus.ActiveLifecycle && vectorRows == config.Corpus.ActiveLifecycle
}

func memoryEligibilityForbiddenHits(memories []Memory, forbiddenID string) int {
	hits := 0
	for _, memory := range memories {
		if memory.ID == forbiddenID {
			hits++
		}
	}
	return hits
}

func memoryEligibilityDifferentContinuity(dataset memoryEligibilityProfileDataset, record memoryEligibilityProfileRecord) string {
	for _, continuity := range dataset.Continuities[record.TenantID] {
		if continuity.ContinuityID != record.ContinuityID {
			return continuity.ContinuityID
		}
	}
	return ""
}

func memoryEligibilityDifferentTenant(dataset memoryEligibilityProfileDataset, record memoryEligibilityProfileRecord) string {
	for _, tenantID := range dataset.Tenants {
		if tenantID != record.TenantID {
			return tenantID
		}
	}
	return ""
}

func memoryEligibilityFirstContinuity(dataset memoryEligibilityProfileDataset, tenantID string) string {
	if continuities := dataset.Continuities[tenantID]; len(continuities) > 0 {
		return continuities[0].ContinuityID
	}
	return ""
}

func sortedMemoryEligibilityLatencies(values []time.Duration) []time.Duration {
	values = append([]time.Duration(nil), values...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}
