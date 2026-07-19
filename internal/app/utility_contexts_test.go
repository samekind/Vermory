package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vermory/internal/runtime"
	"vermory/internal/utilityeval"
)

func TestLoadNativeContextEvidenceValidatesReceipt(t *testing.T) {
	directory := t.TempDir()
	caseID := "C01-device-maintenance-continuity"
	body := "Governed memory:\nGboard had 1,333,470 personal-dictionary rows."
	evidence := utilityeval.NewContextEvidence(body, "native")
	receipt := nativeContextEvidenceReceipt{
		CaseID: caseID, DeliveryID: "11111111-1111-1111-1111-111111111111",
		ContextSHA256: evidence.SHA256, ContextBytes: evidence.ByteSize,
		RunID: "w27-native-vector-test", RetrievalMode: string(runtime.RetrievalVector),
		RetrievalProfile: runtime.ProductionRetrievalProfileID,
	}
	writeNativeContextFixture(t, directory, caseID, evidence.Body, receipt)

	loaded, loadedReceipt, err := loadNativeContextEvidence(directory, caseID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Context != evidence.Body || loaded.DeliveryID != receipt.DeliveryID || loadedReceipt != receipt {
		t.Fatalf("unexpected native evidence: loaded=%#v receipt=%#v", loaded, loadedReceipt)
	}
}

func TestLoadNativeContextEvidenceRejectsTamperedBody(t *testing.T) {
	directory := t.TempDir()
	caseID := "W01-synapseloom-continuity"
	evidence := utilityeval.NewContextEvidence("Governed memory:\nFrontend port is 5173.", "native")
	receipt := nativeContextEvidenceReceipt{
		CaseID: caseID, DeliveryID: "11111111-1111-1111-1111-111111111111",
		ContextSHA256: evidence.SHA256, ContextBytes: evidence.ByteSize,
		RunID: "w27-native-vector-test", RetrievalMode: string(runtime.RetrievalVector),
		RetrievalProfile: runtime.ProductionRetrievalProfileID,
	}
	writeNativeContextFixture(t, directory, caseID, evidence.Body+"\nHistorical port is 3000.", receipt)

	_, _, err := loadNativeContextEvidence(directory, caseID)
	if err == nil || !strings.Contains(err.Error(), "does not match its receipt") {
		t.Fatalf("tampered native body was accepted: %v", err)
	}
}

func TestNativeReceiptRetrievalModeUsesExplicitDefaultsPath(t *testing.T) {
	if got := nativeReceiptRetrievalMode("G01-language-default-local-override", string(runtime.RetrievalVector)); got != "global_defaults" {
		t.Fatalf("global defaults receipt mode=%q", got)
	}
	if got := nativeReceiptRetrievalMode("W01-synapseloom-continuity", ""); got != string(runtime.RetrievalLexical) {
		t.Fatalf("default workspace receipt mode=%q", got)
	}
}

func writeNativeContextFixture(t *testing.T, directory, caseID, body string, receipt nativeContextEvidenceReceipt) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, caseID+".md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, caseID+".receipt.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
