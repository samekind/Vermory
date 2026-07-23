package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestSearchActiveMemoryPrefersExplicitCorrectionOnAuthorityTie(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "authority-ranking"
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, "/fixtures/authority-ranking")
	if err != nil {
		t.Fatal(err)
	}
	content := "The release policy applies to production."
	source, err := store.CommitGovernedObservation(ctx, tenantID, continuityID, CommitObservationRequest{
		OperationID: "authority-source",
		Kind:        ObservationKindSourceUpdate,
		Content:     content,
		SourceRef:   "repo:release-policy@source-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction, err := store.CommitGovernedObservation(ctx, tenantID, continuityID, CommitObservationRequest{
		OperationID: "authority-correction",
		Kind:        ObservationKindUserCorrection,
		Content:     content,
		SourceRef:   "conversation:user",
	})
	if err != nil {
		t.Fatal(err)
	}

	memories, err := store.SearchActiveMemory(ctx, tenantID, continuityID, "release policy applies", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 2 || memories[0].ID != correction.Memory.MemoryID || memories[1].ID != source.Memory.MemoryID {
		t.Fatalf("authority tie-break mismatch: %#v source=%s correction=%s", memories, source.Memory.MemoryID, correction.Memory.MemoryID)
	}
}

func TestSearchActiveVectorMemoryUsesAuthorityTieBreak(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "authority-vector-ranking"
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, tenantID, "/fixtures/authority-vector-ranking")
	if err != nil {
		t.Fatal(err)
	}
	content := "The release policy applies to production."
	source, err := store.CommitGovernedObservation(ctx, tenantID, continuityID, CommitObservationRequest{
		OperationID: "authority-vector-source",
		Kind:        ObservationKindSourceUpdate,
		Content:     content,
		SourceRef:   "repo:release-policy@source-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction, err := store.CommitGovernedObservation(ctx, tenantID, continuityID, CommitObservationRequest{
		OperationID: "authority-vector-correction",
		Kind:        ObservationKindUserCorrection,
		Content:     content,
		SourceRef:   "conversation:user",
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(content))
	hashText := hex.EncodeToString(hash[:])
	probeVector := make([]float32, 1024)
	probeVector[0] = 1
	vector := retrievalVectorLiteral(probeVector)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO memory_vector_documents (profile_id, tenant_id, continuity_id, memory_id, content_sha256, embedding)
VALUES
  ($1, $2, $3::uuid, $4::uuid, $5, $6::vector),
  ($1, $2, $3::uuid, $7::uuid, $5, $6::vector)`,
		ProductionRetrievalProfileID, tenantID, continuityID,
		source.Memory.MemoryID, hashText, vector, correction.Memory.MemoryID); err != nil {
		t.Fatal(err)
	}

	memories, err := store.searchActiveVectorMemory(ctx, tenantID, []string{continuityID}, probeVector, 2, ProductionRetrievalProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 2 || memories[0].ID != correction.Memory.MemoryID || memories[1].ID != source.Memory.MemoryID {
		t.Fatalf("vector authority tie-break mismatch: %#v source=%s correction=%s", memories, source.Memory.MemoryID, correction.Memory.MemoryID)
	}
	if !strings.Contains(memories[0].Content, "release policy") {
		t.Fatalf("vector result lost content: %#v", memories[0])
	}
}
