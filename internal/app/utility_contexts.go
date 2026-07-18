package app

import (
	"context"
	"encoding/json"
	"fmt"
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
	ProfilePath    string
	CaseRoot       string
	DatabaseURL    string
	BundlePath     string
	Mem0ContextDir string
	RunID          string
	ResetDedicated bool
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
		receipt, err := json.MarshalIndent(map[string]any{
			"case_id": caseID, "delivery_id": native.DeliveryID,
			"context_sha256": evidence.SHA256, "context_bytes": evidence.ByteSize,
			"run_id": opts.RunID,
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
	if strings.TrimSpace(opts.Mem0ContextDir) == "" {
		return utilityeval.ContextBundle{}, fmt.Errorf("utility context preparation requires --mem0-context-dir")
	}
	opts.RunID = chooseRunID(opts.RunID, "utility-contexts")

	store, err := runtime.OpenStore(ctx, opts.DatabaseURL)
	if err != nil {
		return utilityeval.ContextBundle{}, err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return utilityeval.ContextBundle{}, err
	}
	if opts.ResetDedicated {
		if err := store.ResetForTest(ctx); err != nil {
			return utilityeval.ContextBundle{}, fmt.Errorf("reset dedicated utility database: %w", err)
		}
	}

	inputs := make([]utilityeval.CaseInput, 0, len(profile.RealityCaseIDs))
	for _, caseID := range profile.RealityCaseIDs {
		frozenCase, err := reality.LoadCase(filepath.Join(opts.CaseRoot, caseID))
		if err != nil {
			return utilityeval.ContextBundle{}, err
		}
		native, err := prepareNativeCaseContext(ctx, store, frozenCase, opts.RunID, nil, nil, runtime.RetrievalProfile{})
		if err != nil {
			return utilityeval.ContextBundle{}, fmt.Errorf("prepare native %s: %w", caseID, err)
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

	bundle := utilityeval.ContextBundle{Version: "1", ProfileID: profile.ID, Inputs: inputs}
	if err := utilityeval.WriteContextBundle(opts.BundlePath, bundle); err != nil {
		return utilityeval.ContextBundle{}, err
	}
	return bundle, nil
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
