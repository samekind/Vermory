package runtime

import (
	"context"
	"fmt"
	"testing"
)

func applyProjectionRetentionEpoch(
	t *testing.T,
	store *Store,
	dataset *dimensionalFixtureDataset,
	epoch int,
	revisionsPerTenant int,
	deletesPerTenant int,
	newFactsPerTenant int,
) {
	t.Helper()
	for tenantIndex, tenantID := range dataset.Tenants {
		governance := NewGovernanceService(store, tenantID)
		records := dataset.Records[tenantID]
		if revisionsPerTenant+deletesPerTenant > len(records) {
			t.Fatalf("W18 epoch exceeds tenant records: epoch=%d revisions=%d deletes=%d records=%d", epoch, revisionsPerTenant, deletesPerTenant, len(records))
		}
		for index := 0; index < revisionsPerTenant; index++ {
			record := records[index]
			content := fmt.Sprintf("%s W18 epoch %02d revision is current.", record.Content, epoch)
			receipt, err := governance.ReviseSource(context.Background(), record.RepoRoot, record.MemoryID, GovernanceWriteRequest{
				OperationID: fmt.Sprintf("%s-epoch-%02d-revise-%02d-%04d", dataset.Prefix, epoch, tenantIndex, index),
				MemoryKey:   record.MemoryKey, Content: content,
				SourceRef: fmt.Sprintf("fixture:%s:epoch:%02d:revision:%02d:%04d", dataset.Prefix, epoch, tenantIndex, index),
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
				fmt.Sprintf("%s-epoch-%02d-delete-%02d-%04d", dataset.Prefix, epoch, tenantIndex, index),
			); err != nil {
				t.Fatal(err)
			}
		}
		records = append(records[:revisionsPerTenant], records[deleteEnd:]...)
		for newIndex := 0; newIndex < newFactsPerTenant; newIndex++ {
			continuityIndex := (epoch + newIndex) % dataset.ContinuitiesPerTenant
			repoRoot := fmt.Sprintf("/fixtures/%s/%02d/%02d", dataset.Prefix, tenantIndex, continuityIndex)
			continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)
			memoryKey := fmt.Sprintf("w18.%s.%02d.epoch.%02d.new.%04d", dataset.Prefix, tenantIndex, epoch, newIndex)
			content := fmt.Sprintf("W18 epoch %02d new marker T%02d-N%04d remains current.", epoch, tenantIndex, newIndex)
			receipt, err := governance.AddSource(context.Background(), repoRoot, GovernanceWriteRequest{
				OperationID: fmt.Sprintf("%s-epoch-%02d-new-%02d-%04d", dataset.Prefix, epoch, tenantIndex, newIndex),
				MemoryKey:   memoryKey, Content: content,
				SourceRef: fmt.Sprintf("fixture:%s:epoch:%02d:new:%02d:%04d", dataset.Prefix, epoch, tenantIndex, newIndex),
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

func projectionRetentionEventCount(t *testing.T, store *Store, tenantID string) int64 {
	t.Helper()
	var count int64
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*) FROM memory_projection_events WHERE tenant_id = $1`, tenantID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func projectionRetentionProfile(t *testing.T, profileID string) RetrievalProfile {
	t.Helper()
	spec, ok := SupportedRetrievalProfile(profileID)
	if !ok {
		t.Fatalf("retrieval profile %s is not registered", profileID)
	}
	return RetrievalProfile{
		ID: spec.ID, BaseURL: spec.BaseURL, Model: spec.Model,
		Dimensions: spec.Dimensions, ProjectionClass: spec.ProjectionClass,
	}
}

func projectionRetentionAuthorityEquivalent(t *testing.T, store *Store, tenantID string, class ProjectionClass) bool {
	t.Helper()
	return sameStringSet(
		governedActiveIDs(t, store, tenantID),
		dimensionalVectorIDs(t, store, tenantID, class),
	)
}

func projectionRetentionProfileVectorIDs(t *testing.T, store *Store, tenantID, profileID string) []string {
	t.Helper()
	table := "memory_vector_documents"
	if profileID == DimensionalMigrationRetrievalProfileID {
		table = "memory_vector_documents_2560"
	}
	rows, err := store.pool.Query(context.Background(), `
SELECT memory_id::text
FROM `+table+`
WHERE tenant_id = $1 AND profile_id = $2
ORDER BY memory_id`, tenantID, profileID)
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
