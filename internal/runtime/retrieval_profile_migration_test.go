package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vermory/internal/memorybackend"
)

func TestRetrievalProfileSpecsKeepProductionAndMigrationProfilesDistinct(t *testing.T) {
	production, ok := SupportedRetrievalProfile(ProductionRetrievalProfileID)
	if !ok {
		t.Fatal("production retrieval profile is not registered")
	}
	migration, ok := SupportedRetrievalProfile(MigrationRetrievalProfileID)
	if !ok {
		t.Fatal("migration retrieval profile is not registered")
	}
	if production.Model == migration.Model || production.ID == migration.ID {
		t.Fatalf("profile migration is not a distinct model/profile: production=%#v migration=%#v", production, migration)
	}
	if production.Dimensions != migration.Dimensions || production.BaseURL != migration.BaseURL {
		t.Fatalf("profiles do not share the supported projection class: production=%#v migration=%#v", production, migration)
	}
	if production.Status != "active" || migration.Status != "candidate" {
		t.Fatalf("unexpected profile lifecycle: production=%#v migration=%#v", production, migration)
	}
	if err := (RetrievalProfile{
		ID: MigrationRetrievalProfileID, BaseURL: migration.BaseURL, Model: migration.Model,
		Dimensions: migration.Dimensions, ProjectionClass: migration.ProjectionClass,
	}).Validate(); err != nil {
		t.Fatal(err)
	}
	if IsSupportedRetrievalProfileID("unknown-profile") {
		t.Fatal("unknown retrieval profile was accepted")
	}
}

func TestDimensionalMigrationProfileSpecIsFrozen(t *testing.T) {
	production, ok := SupportedRetrievalProfile(ProductionRetrievalProfileID)
	if !ok {
		t.Fatal("production retrieval profile is not registered")
	}
	existingCandidate, ok := SupportedRetrievalProfile(MigrationRetrievalProfileID)
	if !ok {
		t.Fatal("existing migration retrieval profile is not registered")
	}
	candidate, ok := SupportedRetrievalProfile(DimensionalMigrationRetrievalProfileID)
	if !ok {
		t.Fatal("dimensional migration retrieval profile is not registered")
	}
	if production.ProjectionClass != ProjectionClass1024 || existingCandidate.ProjectionClass != ProjectionClass1024 {
		t.Fatalf("existing profiles left the 1024 class: production=%#v candidate=%#v", production, existingCandidate)
	}
	if candidate.ID != "siliconflow-qwen3-embedding-4b-2560-v3" ||
		candidate.BaseURL != "https://api.siliconflow.cn/v1" ||
		candidate.Model != "Qwen/Qwen3-Embedding-4B" ||
		candidate.Dimensions != 2560 || candidate.ProjectionClass != ProjectionClass2560 ||
		candidate.Status != "candidate" {
		t.Fatalf("unexpected dimensional candidate: %#v", candidate)
	}
	valid := RetrievalProfile{
		ID: candidate.ID, BaseURL: candidate.BaseURL, Model: candidate.Model,
		Dimensions: candidate.Dimensions, ProjectionClass: candidate.ProjectionClass,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RetrievalProfile){
		"base URL":         func(profile *RetrievalProfile) { profile.BaseURL = "https://example.com/v1" },
		"model":            func(profile *RetrievalProfile) { profile.Model = "other" },
		"dimensions":       func(profile *RetrievalProfile) { profile.Dimensions = 1024 },
		"projection class": func(profile *RetrievalProfile) { profile.ProjectionClass = ProjectionClass1024 },
	} {
		t.Run(name, func(t *testing.T) {
			profile := valid
			mutate(&profile)
			if err := profile.Validate(); err == nil {
				t.Fatalf("invalid dimensional profile was accepted: %#v", profile)
			}
		})
	}
}

func TestLiveRetrievalProfileMigrationPreservesProductionProjection(t *testing.T) {
	apiKey := os.Getenv("VERMORY_LIVE_EMBEDDING_API_KEY")
	databaseURL := os.Getenv("VERMORY_LIVE_MIGRATION_DATABASE_URL")
	if apiKey == "" || databaseURL == "" {
		t.Skip("VERMORY_LIVE_EMBEDDING_API_KEY and VERMORY_LIVE_MIGRATION_DATABASE_URL are required")
	}
	ctx := context.Background()
	store, err := OpenStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	productionSpec, _ := SupportedRetrievalProfile(ProductionRetrievalProfileID)
	migrationSpec, _ := SupportedRetrievalProfile(MigrationRetrievalProfileID)
	productionEmbedder, err := memorybackend.NewOpenAIEmbedder(
		productionSpec.BaseURL, apiKey, productionSpec.Model, productionSpec.Dimensions,
		&http.Client{Timeout: 2 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	migrationEmbedder, err := memorybackend.NewOpenAIEmbedder(
		migrationSpec.BaseURL, apiKey, migrationSpec.Model, migrationSpec.Dimensions,
		&http.Client{Timeout: 2 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	productionCounter := &countingEmbedder{Embedder: productionEmbedder}
	migrationCounter := &countingEmbedder{Embedder: migrationEmbedder}

	tenantIDs := []string{"batch2-dev", "batch2-research", "batch2-life", "batch2-other"}
	for _, tenantID := range tenantIDs {
		if err := store.ResetVectorProjection(ctx, tenantID, productionSpec.ID); err != nil {
			t.Fatal(err)
		}
		if err := store.ResetVectorProjection(ctx, tenantID, migrationSpec.ID); err != nil {
			t.Fatal(err)
		}
		worker, err := NewProjectionWorker(store, productionCounter, ProjectionWorkerOptions{
			TenantID: tenantID,
			Profile: RetrievalProfile{
				ID: productionSpec.ID, BaseURL: productionSpec.BaseURL, Model: productionSpec.Model,
				Dimensions: productionSpec.Dimensions, ProjectionClass: productionSpec.ProjectionClass,
			},
			BatchSize: 256,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result, err := worker.RebuildCurrent(ctx); err != nil || result.Lag != 0 {
			t.Fatalf("build production profile for %s: result=%#v err=%v", tenantID, result, err)
		}
	}

	var productionCountBefore int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memory_vector_documents WHERE profile_id = $1`, productionSpec.ID).Scan(&productionCountBefore); err != nil {
		t.Fatal(err)
	}
	if productionCountBefore == 0 {
		t.Fatal("production profile projection is empty before migration")
	}

	for _, tenantID := range tenantIDs {
		worker, err := NewProjectionWorker(store, migrationCounter, ProjectionWorkerOptions{
			TenantID: tenantID,
			Profile: RetrievalProfile{
				ID: migrationSpec.ID, BaseURL: migrationSpec.BaseURL, Model: migrationSpec.Model,
				Dimensions: migrationSpec.Dimensions, ProjectionClass: migrationSpec.ProjectionClass,
			},
			BatchSize: 256,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result, err := worker.RebuildCurrent(ctx); err != nil || result.Lag != 0 {
			t.Fatalf("build migration profile for %s: result=%#v err=%v", tenantID, result, err)
		}
	}

	var productionCountAfter, migrationCount int
	if err := store.pool.QueryRow(ctx, `
SELECT
  count(*) FILTER (WHERE profile_id = $1),
  count(*) FILTER (WHERE profile_id = $2)
FROM memory_vector_documents`, productionSpec.ID, migrationSpec.ID).Scan(&productionCountAfter, &migrationCount); err != nil {
		t.Fatal(err)
	}
	if productionCountAfter != productionCountBefore || migrationCount != productionCountBefore {
		t.Fatalf("profile migration changed production projection or built incomplete candidate: before=%d after=%d candidate=%d", productionCountBefore, productionCountAfter, migrationCount)
	}

	workspaceID, err := store.ConfirmWorkspaceBinding(ctx, "batch2-dev", "/fixtures/w10/atlas-service")
	if err != nil {
		t.Fatal(err)
	}
	query := "production release command"
	productionCoordinator, err := NewRetrievalCoordinator(store, productionCounter, RetrievalProfile{
		ID: productionSpec.ID, BaseURL: productionSpec.BaseURL, Model: productionSpec.Model,
		Dimensions: productionSpec.Dimensions, ProjectionClass: productionSpec.ProjectionClass,
	})
	if err != nil {
		t.Fatal(err)
	}
	migrationCoordinator, err := NewRetrievalCoordinator(store, migrationCounter, RetrievalProfile{
		ID: migrationSpec.ID, BaseURL: migrationSpec.BaseURL, Model: migrationSpec.Model,
		Dimensions: migrationSpec.Dimensions, ProjectionClass: migrationSpec.ProjectionClass,
	})
	if err != nil {
		t.Fatal(err)
	}
	productionResult, err := productionCoordinator.Retrieve(ctx, RetrievalRequest{
		OperationID: "live-profile-migration-production",
		TenantID:    "batch2-dev", ContinuityIDs: []string{workspaceID}, Query: query, Limit: 3, Mode: RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	migrationResult, err := migrationCoordinator.Retrieve(ctx, RetrievalRequest{
		OperationID: "live-profile-migration-candidate",
		TenantID:    "batch2-dev", ContinuityIDs: []string{workspaceID}, Query: query, Limit: 3, Mode: RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsRetrievedContent(productionResult.Memories, "pnpm exec verify:release") || !containsRetrievedContent(migrationResult.Memories, "pnpm exec verify:release") {
		t.Fatalf("profile migration lost the governed release command: production=%#v migration=%#v", productionResult.Memories, migrationResult.Memories)
	}

	productionStatus, err := store.RetrievalProjectionStatus(ctx, "batch2-dev", productionSpec.ID)
	if err != nil {
		t.Fatal(err)
	}
	migrationStatus, err := store.RetrievalProjectionStatus(ctx, "batch2-dev", migrationSpec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if productionStatus.Lag != 0 || migrationStatus.Lag != 0 || migrationStatus.VectorCount == 0 {
		t.Fatalf("profile migration status is not current: production=%#v migration=%#v", productionStatus, migrationStatus)
	}
	payload, _ := json.Marshal(map[string]any{
		"production_profile":            productionSpec.ID,
		"migration_profile":             migrationSpec.ID,
		"production_vector_count":       productionCountAfter,
		"migration_vector_count":        migrationCount,
		"production_top_content":        productionResult.Memories[0].Content,
		"migration_top_content":         migrationResult.Memories[0].Content,
		"production_lag":                productionStatus.Lag,
		"migration_lag":                 migrationStatus.Lag,
		"production_embedding_requests": productionCounter.Count(),
		"migration_embedding_requests":  migrationCounter.Count(),
	})
	t.Logf("profile migration evidence=%s", payload)
}

type countingEmbedder struct {
	Embedder
	requests atomic.Int64
}

func (e *countingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.requests.Add(1)
	return e.Embedder.Embed(ctx, text)
}

func (e *countingEmbedder) Count() int64 {
	return e.requests.Load()
}

func containsRetrievedContent(memories []Memory, fragment string) bool {
	for _, memory := range memories {
		if strings.Contains(memory.Content, fragment) {
			return true
		}
	}
	return false
}
