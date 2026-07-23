package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vermory/internal/memorybackend"
	"vermory/internal/reality"
	"vermory/internal/runtime"
	"vermory/internal/utilityeval"
)

type UtilityContextBundleOptions struct {
	ProfilePath                 string
	CaseRoot                    string
	DatabaseURL                 string
	BundlePath                  string
	NativeContextDir            string
	Mem0ContextDir              string
	ExpectedNativeRetrievalMode string
}

type NativeUtilityContextOptions struct {
	ProfilePath         string
	CaseRoot            string
	DatabaseURL         string
	OutputDir           string
	RunID               string
	ResetDedicated      bool
	RetrievalMode       string
	RetrievalProfile    string
	EmbeddingBaseURL    string
	EmbeddingAPIKey     string
	EmbeddingModel      string
	EmbeddingDimensions int
}

func PrepareNativeUtilityContexts(ctx context.Context, opts NativeUtilityContextOptions) error {
	profile, err := utilityeval.LoadProfile(opts.ProfilePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.DatabaseURL) == "" || strings.TrimSpace(opts.OutputDir) == "" {
		return fmt.Errorf("native utility preparation requires database URL and output directory")
	}
	if err := os.MkdirAll(opts.OutputDir, 0o700); err != nil {
		return err
	}
	opts.RunID = chooseRunID(opts.RunID, "native-contexts")
	store, err := runtime.OpenStore(ctx, opts.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	if opts.ResetDedicated {
		if err := store.ResetForTest(ctx); err != nil {
			return fmt.Errorf("reset dedicated utility database: %w", err)
		}
	}
	retriever, embedder, profileSpec, err := configureUtilityRetriever(store, opts)
	if err != nil {
		return err
	}
	for _, caseID := range profile.RealityCaseIDs {
		frozenCase, err := reality.LoadCase(filepath.Join(opts.CaseRoot, caseID))
		if err != nil {
			return err
		}
		native, err := prepareNativeCaseContext(ctx, store, frozenCase, opts.RunID, retriever, embedder, profileSpec)
		if err != nil {
			return fmt.Errorf("prepare native %s: %w", caseID, err)
		}
		evidence := utilityeval.NewContextEvidence(native.Context, "postgresql:memory_delivery:"+native.DeliveryID)
		if err := os.WriteFile(filepath.Join(opts.OutputDir, caseID+".md"), []byte(evidence.Body), 0o600); err != nil {
			return err
		}
		receipt, err := json.MarshalIndent(nativeContextEvidenceReceipt{
			CaseID:           caseID,
			DeliveryID:       native.DeliveryID,
			ContextSHA256:    evidence.SHA256,
			ContextBytes:     evidence.ByteSize,
			RunID:            opts.RunID,
			RetrievalMode:    nativeReceiptRetrievalMode(caseID, opts.RetrievalMode),
			RetrievalProfile: profileSpec.ID,
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(opts.OutputDir, caseID+".receipt.json"), append(receipt, '\n'), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func PrepareUtilityContextBundle(ctx context.Context, opts UtilityContextBundleOptions) (utilityeval.ContextBundle, error) {
	profile, err := utilityeval.LoadProfile(opts.ProfilePath)
	if err != nil {
		return utilityeval.ContextBundle{}, err
	}
	if strings.TrimSpace(opts.DatabaseURL) == "" {
		return utilityeval.ContextBundle{}, fmt.Errorf("utility context preparation requires --database-url")
	}
	if strings.TrimSpace(opts.BundlePath) == "" {
		return utilityeval.ContextBundle{}, fmt.Errorf("utility context preparation requires --bundle")
	}
	if strings.TrimSpace(opts.NativeContextDir) == "" {
		return utilityeval.ContextBundle{}, fmt.Errorf("utility context preparation requires --native-context-dir")
	}
	if strings.TrimSpace(opts.Mem0ContextDir) == "" {
		return utilityeval.ContextBundle{}, fmt.Errorf("utility context preparation requires --mem0-context-dir")
	}
	expectedNativeMode := strings.TrimSpace(opts.ExpectedNativeRetrievalMode)
	if expectedNativeMode == "" {
		expectedNativeMode = string(runtime.RetrievalVector)
	}
	if expectedNativeMode != string(runtime.RetrievalLexical) && expectedNativeMode != string(runtime.RetrievalVector) && expectedNativeMode != string(runtime.RetrievalShadow) {
		return utilityeval.ContextBundle{}, fmt.Errorf("unsupported expected native retrieval mode %q", expectedNativeMode)
	}

	store, err := runtime.OpenStore(ctx, opts.DatabaseURL)
	if err != nil {
		return utilityeval.ContextBundle{}, err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return utilityeval.ContextBundle{}, err
	}

	inputs := make([]utilityeval.CaseInput, 0, len(profile.RealityCaseIDs))
	for _, caseID := range profile.RealityCaseIDs {
		frozenCase, err := reality.LoadCase(filepath.Join(opts.CaseRoot, caseID))
		if err != nil {
			return utilityeval.ContextBundle{}, err
		}
		native, receipt, err := loadNativeContextEvidence(opts.NativeContextDir, caseID)
		if err != nil {
			return utilityeval.ContextBundle{}, err
		}
		expectedCaseMode := expectedNativeMode
		if caseID == "G01-language-default-local-override" {
			expectedCaseMode = "global_defaults"
		}
		if receipt.RetrievalMode != expectedCaseMode {
			return utilityeval.ContextBundle{}, fmt.Errorf("native context %s retrieval mode is %q, expected %q", caseID, receipt.RetrievalMode, expectedCaseMode)
		}
		if expectedCaseMode != string(runtime.RetrievalLexical) && expectedCaseMode != "global_defaults" && strings.TrimSpace(receipt.RetrievalProfile) == "" {
			return utilityeval.ContextBundle{}, fmt.Errorf("native context %s has no retrieval profile", caseID)
		}
		tenantID := "w27-" + compactIdentifier(caseID)
		stored, err := store.LookupDelivery(ctx, tenantID, native.DeliveryID)
		if err != nil {
			return utilityeval.ContextBundle{}, fmt.Errorf("verify native delivery %s: %w", caseID, err)
		}
		if strings.TrimSpace(stored.Context) != strings.TrimSpace(native.Context) {
			return utilityeval.ContextBundle{}, fmt.Errorf("native context %s does not match PostgreSQL delivery", caseID)
		}
		mem0Bytes, err := os.ReadFile(filepath.Join(opts.Mem0ContextDir, caseID+".md"))
		if err != nil {
			return utilityeval.ContextBundle{}, fmt.Errorf("read mem0 context %s: %w", caseID, err)
		}
		contexts, err := utilityeval.BuildComparableContexts(frozenCase, native.Context, string(mem0Bytes))
		if err != nil {
			return utilityeval.ContextBundle{}, err
		}
		input, err := utilityeval.NewCaseInput(frozenCase, contexts)
		if err != nil {
			return utilityeval.ContextBundle{}, err
		}
		input.ScoringAliases = profile.ScoringAliases[caseID]
		input.Context[utilityeval.ConditionVermoryNative] = utilityeval.ContextEvidence{
			Body:       strings.TrimSpace(native.Context),
			Source:     "postgresql:memory_delivery:" + native.DeliveryID,
			SHA256:     utilityContextSHA(native.Context),
			ByteSize:   len([]byte(strings.TrimSpace(native.Context))),
			DeliveryID: native.DeliveryID,
		}
		if err := input.Validate(); err != nil {
			return utilityeval.ContextBundle{}, err
		}
		inputs = append(inputs, input)
	}

	bundle := utilityeval.ContextBundle{Version: "2", ProfileID: profile.ID, ProfileSHA256: profile.SHA256, ScorerVersion: profile.ScorerVersion, Inputs: inputs}
	if err := utilityeval.WriteContextBundle(opts.BundlePath, bundle); err != nil {
		return utilityeval.ContextBundle{}, err
	}
	return bundle, nil
}

type nativeContextEvidenceReceipt struct {
	CaseID           string `json:"case_id"`
	DeliveryID       string `json:"delivery_id"`
	ContextSHA256    string `json:"context_sha256"`
	ContextBytes     int    `json:"context_bytes"`
	RunID            string `json:"run_id"`
	RetrievalMode    string `json:"retrieval_mode"`
	RetrievalProfile string `json:"retrieval_profile,omitempty"`
}

func loadNativeContextEvidence(directory, caseID string) (nativeContextReceipt, nativeContextEvidenceReceipt, error) {
	bodyBytes, err := os.ReadFile(filepath.Join(directory, caseID+".md"))
	if err != nil {
		return nativeContextReceipt{}, nativeContextEvidenceReceipt{}, fmt.Errorf("read native context %s: %w", caseID, err)
	}
	receiptBytes, err := os.ReadFile(filepath.Join(directory, caseID+".receipt.json"))
	if err != nil {
		return nativeContextReceipt{}, nativeContextEvidenceReceipt{}, fmt.Errorf("read native receipt %s: %w", caseID, err)
	}
	var receipt nativeContextEvidenceReceipt
	decoder := json.NewDecoder(bytes.NewReader(receiptBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return nativeContextReceipt{}, nativeContextEvidenceReceipt{}, fmt.Errorf("decode native receipt %s: %w", caseID, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nativeContextReceipt{}, nativeContextEvidenceReceipt{}, fmt.Errorf("decode native receipt %s: trailing JSON", caseID)
	}
	if receipt.CaseID != caseID || strings.TrimSpace(receipt.DeliveryID) == "" || strings.TrimSpace(receipt.RunID) == "" || strings.TrimSpace(receipt.RetrievalMode) == "" {
		return nativeContextReceipt{}, nativeContextEvidenceReceipt{}, fmt.Errorf("native receipt %s identity is invalid", caseID)
	}
	evidence := utilityeval.NewContextEvidence(string(bodyBytes), "postgresql:memory_delivery:"+receipt.DeliveryID)
	if receipt.ContextSHA256 != evidence.SHA256 || receipt.ContextBytes != evidence.ByteSize {
		return nativeContextReceipt{}, nativeContextEvidenceReceipt{}, fmt.Errorf("native context %s does not match its receipt", caseID)
	}
	return nativeContextReceipt{Context: evidence.Body, DeliveryID: receipt.DeliveryID}, receipt, nil
}

func nativeReceiptRetrievalMode(caseID, requestedMode string) string {
	if caseID == "G01-language-default-local-override" {
		return "global_defaults"
	}
	mode := strings.TrimSpace(requestedMode)
	if mode == "" {
		return string(runtime.RetrievalLexical)
	}
	return mode
}

type utilityRetrievalBinding struct {
	retriever runtime.MemoryRetriever
	mode      runtime.RetrievalMode
}

func (binding utilityRetrievalBinding) Retrieve(ctx context.Context, request runtime.RetrievalRequest) (runtime.RetrievalResult, error) {
	request.Mode = binding.mode
	return binding.retriever.Retrieve(ctx, request)
}

func configureUtilityRetriever(store *runtime.Store, opts NativeUtilityContextOptions) (runtime.MemoryRetriever, memorybackend.Embedder, runtime.RetrievalProfile, error) {
	mode := strings.TrimSpace(opts.RetrievalMode)
	if mode == "" || mode == string(runtime.RetrievalLexical) {
		return nil, nil, runtime.RetrievalProfile{}, nil
	}
	if store == nil {
		return nil, nil, runtime.RetrievalProfile{}, fmt.Errorf("utility semantic retrieval store is required")
	}
	if mode != string(runtime.RetrievalVector) && mode != string(runtime.RetrievalShadow) {
		return nil, nil, runtime.RetrievalProfile{}, fmt.Errorf("unsupported utility retrieval mode %q", mode)
	}
	profileID := strings.TrimSpace(opts.RetrievalProfile)
	if profileID == "" {
		profileID = runtime.ProductionRetrievalProfileID
	}
	spec, ok := runtime.SupportedRetrievalProfile(profileID)
	if !ok {
		return nil, nil, runtime.RetrievalProfile{}, fmt.Errorf("unsupported utility retrieval profile %q", profileID)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(opts.EmbeddingBaseURL), "/")
	if baseURL == "" {
		baseURL = spec.BaseURL
	}
	model := strings.TrimSpace(opts.EmbeddingModel)
	if model == "" {
		model = spec.Model
	}
	dimensions := opts.EmbeddingDimensions
	if dimensions == 0 {
		dimensions = spec.Dimensions
	}
	profile := runtime.RetrievalProfile{ID: spec.ID, BaseURL: baseURL, Model: model, Dimensions: dimensions, ProjectionClass: spec.ProjectionClass}
	if err := profile.Validate(); err != nil {
		return nil, nil, runtime.RetrievalProfile{}, err
	}
	if strings.TrimSpace(opts.EmbeddingAPIKey) == "" {
		return nil, nil, runtime.RetrievalProfile{}, fmt.Errorf("embedding API key is required for utility semantic retrieval")
	}
	embedder, err := memorybackend.NewOpenAIEmbedder(baseURL, opts.EmbeddingAPIKey, model, dimensions, &http.Client{Timeout: 60 * time.Second})
	if err != nil {
		return nil, nil, runtime.RetrievalProfile{}, err
	}
	coordinator, err := runtime.NewRetrievalCoordinator(store, embedder, profile)
	if err != nil {
		return nil, nil, runtime.RetrievalProfile{}, err
	}
	return utilityRetrievalBinding{retriever: coordinator, mode: runtime.RetrievalMode(mode)}, embedder, profile, nil
}

type nativeContextReceipt struct {
	Context    string
	DeliveryID string
}

func prepareNativeCaseContext(
	ctx context.Context,
	store *runtime.Store,
	frozenCase reality.Case,
	runID string,
	retriever runtime.MemoryRetriever,
	embedder memorybackend.Embedder,
	profile runtime.RetrievalProfile,
) (nativeContextReceipt, error) {
	tenantID := "w27-" + compactIdentifier(frozenCase.Manifest.ID)
	for index, fact := range frozenCase.Manifest.Expectations.CurrentFacts {
		if strings.HasPrefix(fact, "The temporary recovery code has been deleted.") {
			continue
		}
		if frozenCase.Manifest.ID == "G01-language-default-local-override" {
			continue
		}
		continuityID, err := seedContinuity(ctx, store, tenantID, frozenCase.Manifest.ID, fact, index, runID)
		if err != nil {
			return nativeContextReceipt{}, err
		}
		if continuityID == "" {
			return nativeContextReceipt{}, fmt.Errorf("seeded case %s without continuity", frozenCase.Manifest.ID)
		}
	}
	if retriever != nil && frozenCase.Manifest.ID != "G01-language-default-local-override" {
		worker, err := runtime.NewProjectionWorker(store, embedder, runtime.ProjectionWorkerOptions{
			TenantID: tenantID,
			Profile:  profile,
		})
		if err != nil {
			return nativeContextReceipt{}, err
		}
		result, err := worker.RebuildCurrent(ctx)
		if err != nil {
			return nativeContextReceipt{}, fmt.Errorf("rebuild semantic projection: %w", err)
		}
		if result.FailureCode != "" || result.Lag != 0 {
			return nativeContextReceipt{}, fmt.Errorf("semantic projection is not current: status=%s lag=%d failure=%s", result.Status, result.Lag, result.FailureCode)
		}
	}

	switch frozenCase.Manifest.ID {
	case "W01-synapseloom-continuity":
		return prepareNativeWorkspace(ctx, store, tenantID, frozenCase, runID, retriever)
	case "G01-language-default-local-override":
		return prepareNativeGlobalDefaults(ctx, store, tenantID, frozenCase, runID)
	case "C01-device-maintenance-continuity", "S01-deletion-and-source-injection":
		return prepareNativeConversation(ctx, store, tenantID, frozenCase, runID, retriever)
	default:
		return nativeContextReceipt{}, fmt.Errorf("native utility case %s is not mapped", frozenCase.Manifest.ID)
	}
}

func seedContinuity(ctx context.Context, store *runtime.Store, tenantID, caseID, fact string, index int, runID string) (string, error) {
	operationID := fmt.Sprintf("%s-%s-fact-%02d", runID, compactIdentifier(caseID), index)
	request := runtime.CommitObservationRequest{
		OperationID: operationID,
		Kind:        runtime.ObservationKindSourceUpdate,
		MemoryKey:   fmt.Sprintf("w27.fact.%02d", index),
		Content:     fact,
		SourceRef:   "reality:" + caseID,
	}
	var continuityID string
	var err error
	switch caseID {
	case "W01-synapseloom-continuity":
		anchor := runtime.WorkspaceAnchor{RepoRoot: "/vermory/w27/" + compactIdentifier(caseID), FilesystemNamespace: "w27-" + compactIdentifier(caseID)}
		continuityID, err = store.ConfirmWorkspaceAnchorBinding(ctx, tenantID, anchor)
	case "C01-device-maintenance-continuity", "S01-deletion-and-source-injection":
		resolution, resolveErr := store.ResolveOrCreateConversation(ctx, tenantID, runtime.ConversationAnchor{Channel: "web_chat", ThreadID: "w27-" + compactIdentifier(caseID)})
		continuityID, err = resolution.ContinuityID, resolveErr
	default:
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := store.CommitGovernedObservation(ctx, tenantID, continuityID, request); err != nil {
		return "", err
	}
	return continuityID, nil
}

func prepareNativeWorkspace(ctx context.Context, store *runtime.Store, tenantID string, frozenCase reality.Case, runID string, retriever runtime.MemoryRetriever) (nativeContextReceipt, error) {
	anchor := runtime.WorkspaceAnchor{RepoRoot: "/vermory/w27/" + compactIdentifier(frozenCase.Manifest.ID), FilesystemNamespace: "w27-" + compactIdentifier(frozenCase.Manifest.ID)}
	service := runtime.NewServiceWithRetriever(store, tenantID, retriever)
	response, err := service.PrepareContext(ctx, runtime.PrepareContextRequest{
		OperationID: runID + "-" + compactIdentifier(frozenCase.Manifest.ID) + "-delivery",
		Workspace:   anchor,
		Task:        frozenCase.Manifest.Task.Prompt,
		MaxItems:    12,
	})
	if err != nil {
		return nativeContextReceipt{}, err
	}
	if response.Status != runtime.ResolutionResolved {
		return nativeContextReceipt{}, fmt.Errorf("workspace status is %s", response.Status)
	}
	return nativeContextReceipt{Context: response.Context, DeliveryID: response.DeliveryID}, nil
}

func prepareNativeConversation(ctx context.Context, store *runtime.Store, tenantID string, frozenCase reality.Case, runID string, retriever runtime.MemoryRetriever) (nativeContextReceipt, error) {
	anchor := runtime.ConversationAnchor{Channel: "web_chat", ThreadID: "w27-" + compactIdentifier(frozenCase.Manifest.ID)}
	resolution, err := store.ResolveConversation(ctx, tenantID, anchor)
	if err != nil {
		return nativeContextReceipt{}, err
	}
	if err := store.RebuildProjection(ctx, tenantID, resolution.ContinuityID); err != nil {
		return nativeContextReceipt{}, err
	}
	service := runtime.NewConversationService(store, tenantID, nil, "", runtime.ConversationServiceConfig{
		MemoryLimit: 12,
		RecentLimit: 1,
		Retriever:   retriever,
	})
	prepared, err := service.PrepareExternalTurn(ctx, runtime.ExternalConversationTurnRequest{
		OperationID: runID + "-" + compactIdentifier(frozenCase.Manifest.ID) + "-delivery",
		Anchor:      anchor,
		Message:     frozenCase.Manifest.Task.Prompt,
	})
	if err != nil {
		return nativeContextReceipt{}, err
	}
	return nativeContextReceipt{Context: prepared.Context, DeliveryID: prepared.DeliveryID}, nil
}

func prepareNativeGlobalDefaults(ctx context.Context, store *runtime.Store, tenantID string, frozenCase reality.Case, runID string) (nativeContextReceipt, error) {
	defaults := runtime.NewGlobalDefaultsService(store, tenantID)
	for index, content := range []string{
		"The stable global reply language is Chinese unless the active task explicitly overrides it.",
		"Task-local overrides are local-scope and expire with their task.",
	} {
		if _, err := defaults.Set(ctx, runtime.SetGlobalDefaultRequest{
			OperationID: fmt.Sprintf("%s-%s-default-%02d", runID, compactIdentifier(frozenCase.Manifest.ID), index),
			Key:         fmt.Sprintf("w27_default_%02d", index),
			Content:     content,
		}); err != nil {
			return nativeContextReceipt{}, err
		}
	}
	anchor := runtime.ConversationAnchor{Channel: "web_chat", ThreadID: "w27-" + compactIdentifier(frozenCase.Manifest.ID)}
	service := runtime.NewConversationService(store, tenantID, nil, "", runtime.ConversationServiceConfig{MemoryLimit: 12, RecentLimit: 1})
	prepared, err := service.PrepareExternalTurn(ctx, runtime.ExternalConversationTurnRequest{
		OperationID: runID + "-" + compactIdentifier(frozenCase.Manifest.ID) + "-delivery",
		Anchor:      anchor,
		Message:     frozenCase.Manifest.Task.Prompt,
	})
	if err != nil {
		return nativeContextReceipt{}, err
	}
	return nativeContextReceipt{Context: prepared.Context, DeliveryID: prepared.DeliveryID}, nil
}

func compactIdentifier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer("_", "-", "/", "-", " ", "-").Replace(value)
}

func utilityContextSHA(value string) string {
	return utilityeval.NewContextEvidence(value, "native").SHA256
}
