package operatorcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"vermory/internal/provider"
	"vermory/internal/runtime"

	"github.com/spf13/cobra"
)

type connectionOptions struct {
	databaseURL         string
	tenantID            string
	filesystemNamespace string
}

type workspaceOutput struct {
	Status              string `json:"status"`
	ContinuityID        string `json:"continuity_id,omitempty"`
	RepoRoot            string `json:"repo_root"`
	FilesystemNamespace string `json:"filesystem_namespace,omitempty"`
}

type mutationOutput struct {
	ContinuityID  string `json:"continuity_id"`
	ObservationID string `json:"observation_id"`
	MemoryID      string `json:"memory_id"`
	MemoryStatus  string `json:"memory_status"`
	Replayed      bool   `json:"replayed"`
}

type memoryListOutput struct {
	ContinuityID string                   `json:"continuity_id"`
	RepoRoot     string                   `json:"repo_root"`
	Memories     []runtime.GovernedMemory `json:"memories"`
}

type sourceCandidateOutput struct {
	ContinuityID      string                             `json:"continuity_id"`
	RepoRoot          string                             `json:"repo_root"`
	Disposition       runtime.SourceCandidateDisposition `json:"disposition"`
	MemoryKey         string                             `json:"memory_key"`
	TargetMemoryID    string                             `json:"target_memory_id,omitempty"`
	ObservationID     string                             `json:"observation_id"`
	CandidateMemoryID string                             `json:"candidate_memory_id,omitempty"`
	CandidateStatus   string                             `json:"candidate_status,omitempty"`
	Replayed          bool                               `json:"replayed"`
}

type sourceMatchOutput struct {
	SourceMatchID      string                             `json:"source_match_id"`
	ContinuityID       string                             `json:"continuity_id"`
	RepoRoot           string                             `json:"repo_root"`
	Decision           runtime.SourceMatchStatus          `json:"decision"`
	SelectedMemoryKey  string                             `json:"selected_memory_key,omitempty"`
	MatchedMemoryID    string                             `json:"matched_memory_id,omitempty"`
	ObservationID      string                             `json:"observation_id,omitempty"`
	CandidateMemoryID  string                             `json:"candidate_memory_id,omitempty"`
	CandidateStatus    string                             `json:"candidate_status,omitempty"`
	Disposition        runtime.SourceCandidateDisposition `json:"disposition,omitempty"`
	Provider           string                             `json:"provider"`
	Model              string                             `json:"model"`
	FailureCode        string                             `json:"failure_code,omitempty"`
	Reason             string                             `json:"reason,omitempty"`
	CandidateSetSHA256 string                             `json:"candidate_set_sha256"`
	ProviderSHA256     string                             `json:"provider_artifact_sha256,omitempty"`
	Replayed           bool                               `json:"replayed"`
}

type sourceFormationOutput struct {
	SourceFormationID      string                               `json:"source_formation_id"`
	ContinuityID           string                               `json:"continuity_id"`
	RepoRoot               string                               `json:"repo_root,omitempty"`
	Channel                string                               `json:"channel,omitempty"`
	ThreadID               string                               `json:"thread_id,omitempty"`
	Status                 runtime.SourceFormationStatus        `json:"status"`
	InputKind              runtime.SourceFormationInputKind     `json:"input_kind"`
	InputManifestSHA256    string                               `json:"input_manifest_sha256"`
	SourceRef              string                               `json:"source_ref"`
	SourceSHA256           string                               `json:"source_sha256"`
	SourceBytes            int                                  `json:"source_bytes"`
	ActiveSnapshotSHA256   string                               `json:"active_snapshot_sha256"`
	Provider               string                               `json:"provider"`
	Model                  string                               `json:"model"`
	ProviderArtifactSHA256 string                               `json:"provider_artifact_sha256,omitempty"`
	FailureCode            string                               `json:"failure_code,omitempty"`
	Reason                 string                               `json:"reason,omitempty"`
	Items                  []runtime.SourceFormationItemReceipt `json:"items"`
	Replayed               bool                                 `json:"replayed"`
}

func NewWorkspaceCommand() *cobra.Command {
	options := connectionOptions{}
	command := &cobra.Command{
		Use:   "workspace",
		Short: "Manage trusted workspace bindings",
	}
	addConnectionFlags(command, &options)

	var confirmRoot string
	confirm := &cobra.Command{
		Use:   "confirm",
		Short: "Confirm a workspace binding",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				resolution, err := service.ConfirmWorkspace(cmd.Context(), confirmRoot)
				if err != nil {
					return err
				}
				return writeJSON(cmd, workspaceOutput{
					Status:              string(resolution.Status),
					ContinuityID:        resolution.ContinuityID,
					RepoRoot:            resolution.RepoRoot,
					FilesystemNamespace: resolution.FilesystemNamespace,
				})
			})
		},
	}
	confirm.Flags().StringVar(&confirmRoot, "repo-root", "", "absolute workspace root")
	_ = confirm.MarkFlagRequired("repo-root")

	var inspectRoot string
	inspect := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect a workspace binding",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				resolution, err := service.InspectWorkspace(cmd.Context(), inspectRoot)
				if err != nil {
					return err
				}
				return writeJSON(cmd, workspaceOutput{
					Status:              string(resolution.Status),
					ContinuityID:        resolution.ContinuityID,
					RepoRoot:            resolution.RepoRoot,
					FilesystemNamespace: resolution.FilesystemNamespace,
				})
			})
		},
	}
	inspect.Flags().StringVar(&inspectRoot, "repo-root", "", "absolute workspace root")
	_ = inspect.MarkFlagRequired("repo-root")

	command.AddCommand(confirm, inspect)
	return command
}

func NewMemoryCommand() *cobra.Command {
	options := connectionOptions{}
	command := &cobra.Command{
		Use:   "memory",
		Short: "Apply explicit local memory governance",
	}
	addConnectionFlags(command, &options)

	var inspectRoot string
	inspect := &cobra.Command{
		Use:   "inspect",
		Short: "List governed memories for a confirmed workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				resolution, memories, err := service.ListWorkspaceMemories(cmd.Context(), inspectRoot)
				if err != nil {
					return err
				}
				return writeJSON(cmd, memoryListOutput{
					ContinuityID: resolution.ContinuityID,
					RepoRoot:     resolution.RepoRoot,
					Memories:     memories,
				})
			})
		},
	}
	inspect.Flags().StringVar(&inspectRoot, "repo-root", "", "absolute workspace root")
	_ = inspect.MarkFlagRequired("repo-root")

	var sourceRoot, sourceOperationID, sourceKey, sourceContent, sourceRef string
	addSource := &cobra.Command{
		Use:   "add-source",
		Short: "Record a trusted source fact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				receipt, err := service.AddSource(cmd.Context(), sourceRoot, runtime.GovernanceWriteRequest{
					OperationID: sourceOperationID,
					MemoryKey:   sourceKey,
					Content:     sourceContent,
					SourceRef:   sourceRef,
				})
				if err != nil {
					return err
				}
				return writeMutationJSON(cmd, service, sourceRoot, receipt)
			})
		},
	}
	addSource.Flags().StringVar(&sourceRoot, "repo-root", "", "absolute workspace root")
	addSource.Flags().StringVar(&sourceOperationID, "operation-id", "", "idempotency key")
	addSource.Flags().StringVar(&sourceKey, "key", "", "stable source fact key")
	addSource.Flags().StringVar(&sourceContent, "content", "", "trusted source fact")
	addSource.Flags().StringVar(&sourceRef, "source-ref", "", "opaque source reference")
	markRequired(addSource, "repo-root", "operation-id", "content", "source-ref")

	var proposeRoot, proposeOperationID, proposeKey, proposeContent, proposeSourceRef string
	proposeSource := &cobra.Command{
		Use:   "propose-source",
		Short: "Propose a keyed source fact for operator review",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				receipt, err := service.ProposeSourceCandidate(cmd.Context(), proposeRoot, runtime.GovernanceWriteRequest{
					OperationID: proposeOperationID,
					MemoryKey:   proposeKey,
					Content:     proposeContent,
					SourceRef:   proposeSourceRef,
				})
				if err != nil {
					return err
				}
				return writeSourceCandidateJSON(cmd, service, proposeRoot, receipt)
			})
		},
	}
	proposeSource.Flags().StringVar(&proposeRoot, "repo-root", "", "absolute workspace root")
	proposeSource.Flags().StringVar(&proposeOperationID, "operation-id", "", "idempotency key")
	proposeSource.Flags().StringVar(&proposeKey, "key", "", "stable source fact key")
	proposeSource.Flags().StringVar(&proposeContent, "content", "", "candidate source fact")
	proposeSource.Flags().StringVar(&proposeSourceRef, "source-ref", "", "opaque source revision reference")
	markRequired(proposeSource, "repo-root", "operation-id", "key", "content", "source-ref")

	var matchRoot, matchOperationID, matchContent, matchSourceRef string
	var matchProvider, matchModel, matchBaseURL, matchAPIKeyEnv, matchGrokCommand string
	var matchDisableThinking bool
	matchSource := &cobra.Command{
		Use:   "match-source",
		Short: "Match an unkeyed trusted source fact to the current closed set",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			llm, providerName, model, err := buildDirectProvider(
				matchProvider,
				matchModel,
				matchBaseURL,
				matchAPIKeyEnv,
				matchGrokCommand,
				matchDisableThinking,
			)
			if err != nil {
				return err
			}
			return withSourceMatching(cmd.Context(), options, llm, providerName, model, func(store *runtime.Store, service *runtime.SourceMatchingService) error {
				receipt, err := service.MatchSource(cmd.Context(), matchRoot, runtime.SourceMatchRequest{
					OperationID:   matchOperationID,
					SourceRef:     matchSourceRef,
					SourceContent: matchContent,
				})
				if err != nil {
					return err
				}
				return writeSourceMatchJSON(cmd, store, options.tenantID, matchRoot, receipt)
			})
		},
	}
	matchSource.Flags().StringVar(&matchRoot, "repo-root", "", "absolute workspace root")
	matchSource.Flags().StringVar(&matchOperationID, "operation-id", "", "idempotency key")
	matchSource.Flags().StringVar(&matchContent, "content", "", "exact trusted source fact")
	matchSource.Flags().StringVar(&matchSourceRef, "source-ref", "", "opaque source revision reference")
	matchSource.Flags().StringVar(&matchProvider, "provider", "grok-cli", "provider: grok-cli, openai-compatible, siliconflow, or duojie")
	matchSource.Flags().StringVar(&matchModel, "model", "", "provider model name")
	matchSource.Flags().StringVar(&matchBaseURL, "base-url", "", "direct provider base URL")
	matchSource.Flags().StringVar(&matchAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	matchSource.Flags().StringVar(&matchGrokCommand, "grok-command", "", "authenticated Grok CLI command")
	matchSource.Flags().BoolVar(&matchDisableThinking, "disable-thinking", false, "request non-thinking mode from compatible providers")
	markRequired(matchSource, "repo-root", "operation-id", "content", "source-ref")

	var inspectMatchRoot, inspectMatchOperationID string
	inspectSourceMatch := &cobra.Command{
		Use:   "inspect-source-match",
		Short: "Inspect one durable source match decision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withSourceMatching(cmd.Context(), options, nil, "", "", func(store *runtime.Store, service *runtime.SourceMatchingService) error {
				receipt, err := service.InspectSourceMatch(cmd.Context(), inspectMatchRoot, inspectMatchOperationID)
				if err != nil {
					return err
				}
				return writeSourceMatchJSON(cmd, store, options.tenantID, inspectMatchRoot, receipt)
			})
		},
	}
	inspectSourceMatch.Flags().StringVar(&inspectMatchRoot, "repo-root", "", "absolute workspace root")
	inspectSourceMatch.Flags().StringVar(&inspectMatchOperationID, "operation-id", "", "source match idempotency key")
	markRequired(inspectSourceMatch, "repo-root", "operation-id")

	var formRoot, formOperationID, formSourceFile, formSourceRef string
	var formProvider, formModel, formBaseURL, formAPIKeyEnv, formGrokCommand string
	var formDisableThinking bool
	formDocument := &cobra.Command{
		Use:   "form-document",
		Short: "Form reviewable memory candidates from one trusted document",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			document, err := readSourceFormationFile(formSourceFile)
			if err != nil {
				return err
			}
			llm, providerName, model, err := buildDirectProvider(
				formProvider,
				formModel,
				formBaseURL,
				formAPIKeyEnv,
				formGrokCommand,
				formDisableThinking,
			)
			if err != nil {
				return err
			}
			return withSourceFormation(cmd.Context(), options, llm, providerName, model, func(store *runtime.Store, service *runtime.SourceFormationService) error {
				receipt, err := service.FormDocument(cmd.Context(), formRoot, runtime.SourceFormationRequest{
					OperationID:    formOperationID,
					SourceRef:      formSourceRef,
					SourceDocument: document,
				})
				if err != nil {
					return err
				}
				return writeSourceFormationJSON(cmd, store, options.tenantID, formRoot, receipt)
			})
		},
	}
	formDocument.Flags().StringVar(&formRoot, "repo-root", "", "absolute workspace root")
	formDocument.Flags().StringVar(&formOperationID, "operation-id", "", "idempotency key")
	formDocument.Flags().StringVar(&formSourceFile, "source-file", "", "trusted UTF-8 source file")
	formDocument.Flags().StringVar(&formSourceRef, "source-ref", "", "opaque source revision reference")
	formDocument.Flags().StringVar(&formProvider, "provider", "grok-cli", "provider: grok-cli, openai-compatible, siliconflow, or duojie")
	formDocument.Flags().StringVar(&formModel, "model", "", "provider model name")
	formDocument.Flags().StringVar(&formBaseURL, "base-url", "", "direct provider base URL")
	formDocument.Flags().StringVar(&formAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	formDocument.Flags().StringVar(&formGrokCommand, "grok-command", "", "authenticated Grok CLI command")
	formDocument.Flags().BoolVar(&formDisableThinking, "disable-thinking", false, "request non-thinking mode from compatible providers")
	markRequired(formDocument, "repo-root", "operation-id", "source-file", "source-ref")

	var inspectFormationRoot, inspectFormationOperationID string
	inspectSourceFormation := &cobra.Command{
		Use:   "inspect-source-formation",
		Short: "Inspect one durable source document formation run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withSourceFormation(cmd.Context(), options, nil, "", "", func(store *runtime.Store, service *runtime.SourceFormationService) error {
				receipt, err := service.InspectSourceFormation(cmd.Context(), inspectFormationRoot, inspectFormationOperationID)
				if err != nil {
					return err
				}
				return writeSourceFormationJSON(cmd, store, options.tenantID, inspectFormationRoot, receipt)
			})
		},
	}
	inspectSourceFormation.Flags().StringVar(&inspectFormationRoot, "repo-root", "", "absolute workspace root")
	inspectSourceFormation.Flags().StringVar(&inspectFormationOperationID, "operation-id", "", "source formation idempotency key")
	markRequired(inspectSourceFormation, "repo-root", "operation-id")

	var formConversationOperationID, formConversationChannel, formConversationThreadID string
	var formConversationObservationIDs []string
	var formConversationRecentLimit int
	var formConversationProvider, formConversationModel, formConversationBaseURL, formConversationAPIKeyEnv, formConversationGrokCommand string
	var formConversationDisableThinking bool
	formConversation := &cobra.Command{
		Use:   "form-conversation",
		Short: "Form reviewable memory candidates from bounded user conversation observations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			llm, providerName, model, err := buildDirectProvider(
				formConversationProvider,
				formConversationModel,
				formConversationBaseURL,
				formConversationAPIKeyEnv,
				formConversationGrokCommand,
				formConversationDisableThinking,
			)
			if err != nil {
				return err
			}
			anchor := runtime.ConversationAnchor{Channel: formConversationChannel, ThreadID: formConversationThreadID}
			return withSourceFormation(cmd.Context(), options, llm, providerName, model, func(store *runtime.Store, service *runtime.SourceFormationService) error {
				receipt, err := service.FormConversation(cmd.Context(), runtime.ConversationFormationRequest{
					OperationID:    formConversationOperationID,
					Anchor:         anchor,
					ObservationIDs: formConversationObservationIDs,
					RecentLimit:    formConversationRecentLimit,
				})
				if err != nil {
					return err
				}
				return writeConversationFormationJSON(cmd, store, options.tenantID, anchor, receipt)
			})
		},
	}
	formConversation.Flags().StringVar(&formConversationOperationID, "operation-id", "", "idempotency key")
	formConversation.Flags().StringVar(&formConversationChannel, "channel", "", "confirmed conversation channel")
	formConversation.Flags().StringVar(&formConversationThreadID, "thread-id", "", "confirmed conversation thread")
	formConversation.Flags().StringSliceVar(&formConversationObservationIDs, "observation-id", nil, "explicit user observation ID; repeat or use a comma-separated list")
	formConversation.Flags().IntVar(&formConversationRecentLimit, "recent-user-observations", 0, "select the most recent eligible user observations; defaults to 12 when no IDs are supplied")
	formConversation.Flags().StringVar(&formConversationProvider, "provider", "grok-cli", "provider: grok-cli, openai-compatible, siliconflow, or duojie")
	formConversation.Flags().StringVar(&formConversationModel, "model", "", "provider model name")
	formConversation.Flags().StringVar(&formConversationBaseURL, "base-url", "", "direct provider base URL")
	formConversation.Flags().StringVar(&formConversationAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	formConversation.Flags().StringVar(&formConversationGrokCommand, "grok-command", "", "authenticated Grok CLI command")
	formConversation.Flags().BoolVar(&formConversationDisableThinking, "disable-thinking", false, "request non-thinking mode from compatible providers")
	markRequired(formConversation, "operation-id", "channel", "thread-id")

	var inspectConversationFormationOperationID, inspectConversationFormationChannel, inspectConversationFormationThreadID string
	inspectConversationFormation := &cobra.Command{
		Use:   "inspect-conversation-formation",
		Short: "Inspect one durable conversation formation run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			anchor := runtime.ConversationAnchor{Channel: inspectConversationFormationChannel, ThreadID: inspectConversationFormationThreadID}
			return withSourceFormation(cmd.Context(), options, nil, "", "", func(store *runtime.Store, service *runtime.SourceFormationService) error {
				receipt, err := service.InspectConversationFormation(cmd.Context(), anchor, inspectConversationFormationOperationID)
				if err != nil {
					return err
				}
				return writeConversationFormationJSON(cmd, store, options.tenantID, anchor, receipt)
			})
		},
	}
	inspectConversationFormation.Flags().StringVar(&inspectConversationFormationOperationID, "operation-id", "", "conversation formation idempotency key")
	inspectConversationFormation.Flags().StringVar(&inspectConversationFormationChannel, "channel", "", "confirmed conversation channel")
	inspectConversationFormation.Flags().StringVar(&inspectConversationFormationThreadID, "thread-id", "", "confirmed conversation thread")
	markRequired(inspectConversationFormation, "operation-id", "channel", "thread-id")

	var acceptConversationOperationID, acceptConversationChannel, acceptConversationThreadID, acceptConversationMemoryID string
	acceptConversationCandidate := &cobra.Command{
		Use:   "accept-conversation-candidate",
		Short: "Accept one proposed candidate in an exact conversation continuity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			anchor := runtime.ConversationAnchor{Channel: acceptConversationChannel, ThreadID: acceptConversationThreadID}
			return withConversation(cmd.Context(), options, func(store *runtime.Store, service *runtime.ConversationService) error {
				receipt, err := service.AcceptCandidate(cmd.Context(), runtime.ReviewConversationCandidateRequest{
					OperationID: acceptConversationOperationID,
					Anchor:      anchor,
					MemoryID:    acceptConversationMemoryID,
				})
				if err != nil {
					return err
				}
				return writeConversationMutationJSON(cmd, service, anchor, receipt)
			})
		},
	}
	acceptConversationCandidate.Flags().StringVar(&acceptConversationOperationID, "operation-id", "", "idempotency key")
	acceptConversationCandidate.Flags().StringVar(&acceptConversationChannel, "channel", "", "confirmed conversation channel")
	acceptConversationCandidate.Flags().StringVar(&acceptConversationThreadID, "thread-id", "", "confirmed conversation thread")
	acceptConversationCandidate.Flags().StringVar(&acceptConversationMemoryID, "memory-id", "", "proposed conversation formation candidate")
	markRequired(acceptConversationCandidate, "operation-id", "channel", "thread-id", "memory-id")

	var rejectConversationOperationID, rejectConversationChannel, rejectConversationThreadID, rejectConversationMemoryID string
	rejectConversationCandidate := &cobra.Command{
		Use:   "reject-conversation-candidate",
		Short: "Reject one proposed candidate in an exact conversation continuity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			anchor := runtime.ConversationAnchor{Channel: rejectConversationChannel, ThreadID: rejectConversationThreadID}
			return withConversation(cmd.Context(), options, func(store *runtime.Store, service *runtime.ConversationService) error {
				receipt, err := service.RejectCandidate(cmd.Context(), runtime.ReviewConversationCandidateRequest{
					OperationID: rejectConversationOperationID,
					Anchor:      anchor,
					MemoryID:    rejectConversationMemoryID,
				})
				if err != nil {
					return err
				}
				return writeConversationMutationJSON(cmd, service, anchor, receipt)
			})
		},
	}
	rejectConversationCandidate.Flags().StringVar(&rejectConversationOperationID, "operation-id", "", "idempotency key")
	rejectConversationCandidate.Flags().StringVar(&rejectConversationChannel, "channel", "", "confirmed conversation channel")
	rejectConversationCandidate.Flags().StringVar(&rejectConversationThreadID, "thread-id", "", "confirmed conversation thread")
	rejectConversationCandidate.Flags().StringVar(&rejectConversationMemoryID, "memory-id", "", "proposed conversation formation candidate")
	markRequired(rejectConversationCandidate, "operation-id", "channel", "thread-id", "memory-id")

	var acceptRoot, acceptOperationID, acceptMemoryID string
	acceptCandidate := &cobra.Command{
		Use:   "accept-candidate",
		Short: "Accept one proposed source candidate",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				receipt, err := service.AcceptCandidate(cmd.Context(), acceptRoot, acceptMemoryID, acceptOperationID)
				if err != nil {
					return err
				}
				return writeMutationJSON(cmd, service, acceptRoot, receipt)
			})
		},
	}
	acceptCandidate.Flags().StringVar(&acceptRoot, "repo-root", "", "absolute workspace root")
	acceptCandidate.Flags().StringVar(&acceptOperationID, "operation-id", "", "idempotency key")
	acceptCandidate.Flags().StringVar(&acceptMemoryID, "memory-id", "", "proposed source candidate")
	markRequired(acceptCandidate, "repo-root", "operation-id", "memory-id")

	var rejectRoot, rejectOperationID, rejectMemoryID string
	rejectCandidate := &cobra.Command{
		Use:   "reject-candidate",
		Short: "Reject one proposed source candidate",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				receipt, err := service.RejectCandidate(cmd.Context(), rejectRoot, rejectMemoryID, rejectOperationID)
				if err != nil {
					return err
				}
				return writeMutationJSON(cmd, service, rejectRoot, receipt)
			})
		},
	}
	rejectCandidate.Flags().StringVar(&rejectRoot, "repo-root", "", "absolute workspace root")
	rejectCandidate.Flags().StringVar(&rejectOperationID, "operation-id", "", "idempotency key")
	rejectCandidate.Flags().StringVar(&rejectMemoryID, "memory-id", "", "proposed source candidate")
	markRequired(rejectCandidate, "repo-root", "operation-id", "memory-id")

	var reviseSourceRoot, reviseSourceOperationID, reviseSourceMemoryID, reviseSourceContent, reviseSourceRef string
	reviseSource := &cobra.Command{
		Use:   "revise-source",
		Short: "Replace one named active fact with a trusted source revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				receipt, err := service.ReviseSource(cmd.Context(), reviseSourceRoot, reviseSourceMemoryID, runtime.GovernanceWriteRequest{
					OperationID: reviseSourceOperationID,
					Content:     reviseSourceContent,
					SourceRef:   reviseSourceRef,
				})
				if err != nil {
					return err
				}
				return writeMutationJSON(cmd, service, reviseSourceRoot, receipt)
			})
		},
	}
	reviseSource.Flags().StringVar(&reviseSourceRoot, "repo-root", "", "absolute workspace root")
	reviseSource.Flags().StringVar(&reviseSourceOperationID, "operation-id", "", "idempotency key")
	reviseSource.Flags().StringVar(&reviseSourceMemoryID, "memory-id", "", "active memory superseded by the source revision")
	reviseSource.Flags().StringVar(&reviseSourceContent, "content", "", "replacement trusted source fact")
	reviseSource.Flags().StringVar(&reviseSourceRef, "source-ref", "", "opaque replacement source reference")
	markRequired(reviseSource, "repo-root", "operation-id", "memory-id", "content", "source-ref")

	var correctRoot, correctOperationID, correctMemoryID, correctContent string
	correct := &cobra.Command{
		Use:   "correct",
		Short: "Replace one named active fact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				receipt, err := service.Correct(cmd.Context(), correctRoot, correctMemoryID, runtime.GovernanceWriteRequest{
					OperationID: correctOperationID,
					Content:     correctContent,
				})
				if err != nil {
					return err
				}
				return writeMutationJSON(cmd, service, correctRoot, receipt)
			})
		},
	}
	correct.Flags().StringVar(&correctRoot, "repo-root", "", "absolute workspace root")
	correct.Flags().StringVar(&correctOperationID, "operation-id", "", "idempotency key")
	correct.Flags().StringVar(&correctMemoryID, "memory-id", "", "active memory to supersede")
	correct.Flags().StringVar(&correctContent, "content", "", "replacement fact")
	markRequired(correct, "repo-root", "operation-id", "memory-id", "content")

	var forgetRoot, forgetOperationID, forgetMemoryID string
	forget := &cobra.Command{
		Use:   "forget",
		Short: "Redact one named memory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
				receipt, err := service.Forget(cmd.Context(), forgetRoot, forgetMemoryID, forgetOperationID)
				if err != nil {
					return err
				}
				return writeMutationJSON(cmd, service, forgetRoot, receipt)
			})
		},
	}
	forget.Flags().StringVar(&forgetRoot, "repo-root", "", "absolute workspace root")
	forget.Flags().StringVar(&forgetOperationID, "operation-id", "", "idempotency key")
	forget.Flags().StringVar(&forgetMemoryID, "memory-id", "", "memory to redact")
	markRequired(forget, "repo-root", "operation-id", "memory-id")

	var validityContinuityID, validityOperationID, validityMemoryID string
	var validityFrom, validityUntil string
	setValidity := &cobra.Command{
		Use:   "set-validity",
		Short: "Set or clear the current-use validity interval for one memory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			validFrom, err := parseOptionalUTCEligibilityTime("--valid-from", validityFrom)
			if err != nil {
				return err
			}
			validUntil, err := parseOptionalUTCEligibilityTime("--valid-until", validityUntil)
			if err != nil {
				return err
			}
			return withMemoryEligibility(cmd.Context(), options, func(service *runtime.MemoryEligibilityService) error {
				receipt, err := service.SetValidity(cmd.Context(), runtime.SetMemoryValidityRequest{
					OperationID: validityOperationID, ContinuityID: validityContinuityID,
					MemoryID: validityMemoryID, ValidFrom: validFrom, ValidUntil: validUntil,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	setValidity.Flags().StringVar(&validityContinuityID, "continuity-id", "", "exact continuity identifier")
	setValidity.Flags().StringVar(&validityOperationID, "operation-id", "", "idempotency key")
	setValidity.Flags().StringVar(&validityMemoryID, "memory-id", "", "exact governed memory identifier")
	setValidity.Flags().StringVar(&validityFrom, "valid-from", "", "inclusive UTC RFC3339 boundary")
	setValidity.Flags().StringVar(&validityUntil, "valid-until", "", "exclusive UTC RFC3339 boundary")
	markRequired(setValidity, "continuity-id", "operation-id", "memory-id")

	var archiveContinuityID, archiveOperationID, archiveMemoryID string
	archive := &cobra.Command{
		Use:   "archive",
		Short: "Remove one memory from current use while preserving authorized history",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMemoryEligibility(cmd.Context(), options, func(service *runtime.MemoryEligibilityService) error {
				receipt, err := service.Archive(cmd.Context(), runtime.ArchiveMemoryRequest{
					OperationID: archiveOperationID, ContinuityID: archiveContinuityID, MemoryID: archiveMemoryID,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	archive.Flags().StringVar(&archiveContinuityID, "continuity-id", "", "exact continuity identifier")
	archive.Flags().StringVar(&archiveOperationID, "operation-id", "", "idempotency key")
	archive.Flags().StringVar(&archiveMemoryID, "memory-id", "", "exact governed memory identifier")
	markRequired(archive, "continuity-id", "operation-id", "memory-id")

	command.AddCommand(
		inspect, addSource, proposeSource, matchSource, inspectSourceMatch,
		formDocument, inspectSourceFormation, formConversation, inspectConversationFormation,
		acceptCandidate, rejectCandidate, acceptConversationCandidate, rejectConversationCandidate,
		reviseSource, correct, forget, setValidity, archive,
	)
	return command
}

func NewDefaultsCommand() *cobra.Command {
	options := connectionOptions{}
	command := &cobra.Command{
		Use:   "defaults",
		Short: "Manage explicit global defaults",
	}
	addConnectionFlags(command, &options)

	inspect := &cobra.Command{
		Use:   "inspect",
		Short: "List global default lifecycle state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGlobalDefaults(cmd.Context(), options, func(service *runtime.GlobalDefaultsService) error {
				inspection, err := service.Inspect(cmd.Context())
				if err != nil {
					return err
				}
				return writeJSON(cmd, inspection)
			})
		},
	}

	var setOperationID, setKey, setContent string
	set := &cobra.Command{
		Use:   "set",
		Short: "Create one explicit global default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGlobalDefaults(cmd.Context(), options, func(service *runtime.GlobalDefaultsService) error {
				receipt, err := service.Set(cmd.Context(), runtime.SetGlobalDefaultRequest{
					OperationID: setOperationID,
					Key:         setKey,
					Content:     setContent,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	set.Flags().StringVar(&setOperationID, "operation-id", "", "idempotency key")
	set.Flags().StringVar(&setKey, "key", "", "stable lowercase default key")
	set.Flags().StringVar(&setContent, "content", "", "semantic default content")
	markRequired(set, "operation-id", "key", "content")

	var correctOperationID, correctMemoryID, correctContent string
	correct := &cobra.Command{
		Use:   "correct",
		Short: "Replace one named active global default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGlobalDefaults(cmd.Context(), options, func(service *runtime.GlobalDefaultsService) error {
				receipt, err := service.Correct(cmd.Context(), runtime.CorrectGlobalDefaultRequest{
					OperationID: correctOperationID,
					MemoryID:    correctMemoryID,
					Content:     correctContent,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	correct.Flags().StringVar(&correctOperationID, "operation-id", "", "idempotency key")
	correct.Flags().StringVar(&correctMemoryID, "memory-id", "", "active global default to replace")
	correct.Flags().StringVar(&correctContent, "content", "", "replacement semantic content")
	markRequired(correct, "operation-id", "memory-id", "content")

	var forgetOperationID, forgetMemoryID string
	forget := &cobra.Command{
		Use:   "forget",
		Short: "Delete one named global default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withGlobalDefaults(cmd.Context(), options, func(service *runtime.GlobalDefaultsService) error {
				receipt, err := service.Forget(cmd.Context(), runtime.ForgetGlobalDefaultRequest{
					OperationID: forgetOperationID,
					MemoryID:    forgetMemoryID,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	forget.Flags().StringVar(&forgetOperationID, "operation-id", "", "idempotency key")
	forget.Flags().StringVar(&forgetMemoryID, "memory-id", "", "global default to delete")
	markRequired(forget, "operation-id", "memory-id")

	command.AddCommand(inspect, set, correct, forget)
	return command
}

func NewBridgeCommand() *cobra.Command {
	options := connectionOptions{}
	command := &cobra.Command{Use: "bridge", Short: "Manage explicit cross-continuity bridges"}
	addConnectionFlags(command, &options)

	var promoteOperationID, promoteChannel, promoteThreadID, promoteRepoRoot string
	var promoteMemoryIDs []string
	promote := &cobra.Command{
		Use: "promote", Short: "Promote selected conversation memory into a workspace", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBridges(cmd.Context(), options, func(service *runtime.BridgeService) error {
				receipt, err := service.PromoteConversationToWorkspace(cmd.Context(), runtime.PromoteConversationToWorkspaceRequest{
					OperationID:    promoteOperationID,
					Source:         runtime.ConversationAnchor{Channel: promoteChannel, ThreadID: promoteThreadID},
					TargetRepoRoot: promoteRepoRoot,
					MemoryIDs:      promoteMemoryIDs,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	promote.Flags().StringVar(&promoteOperationID, "operation-id", "", "idempotency key")
	promote.Flags().StringVar(&promoteChannel, "source-channel", "", "source conversation channel")
	promote.Flags().StringVar(&promoteThreadID, "source-thread-id", "", "source conversation thread")
	promote.Flags().StringVar(&promoteRepoRoot, "target-repo-root", "", "confirmed target workspace root")
	promote.Flags().StringSliceVar(&promoteMemoryIDs, "memory-id", nil, "selected active source memory id")
	markRequired(promote, "operation-id", "source-channel", "source-thread-id", "target-repo-root", "memory-id")

	var linkOperationID, primaryChannel, primaryThreadID, linkedChannel, linkedThreadID string
	link := &cobra.Command{
		Use: "link", Short: "Link governed memory across two conversation anchors", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBridges(cmd.Context(), options, func(service *runtime.BridgeService) error {
				receipt, err := service.LinkConversations(cmd.Context(), runtime.LinkConversationsRequest{
					OperationID: linkOperationID,
					Primary:     runtime.ConversationAnchor{Channel: primaryChannel, ThreadID: primaryThreadID},
					Linked:      runtime.ConversationAnchor{Channel: linkedChannel, ThreadID: linkedThreadID},
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	link.Flags().StringVar(&linkOperationID, "operation-id", "", "idempotency key")
	link.Flags().StringVar(&primaryChannel, "primary-channel", "", "primary conversation channel")
	link.Flags().StringVar(&primaryThreadID, "primary-thread-id", "", "primary conversation thread")
	link.Flags().StringVar(&linkedChannel, "linked-channel", "", "linked conversation channel")
	link.Flags().StringVar(&linkedThreadID, "linked-thread-id", "", "linked conversation thread")
	markRequired(link, "operation-id", "primary-channel", "primary-thread-id", "linked-channel", "linked-thread-id")

	var exportOperationID, exportRepoRoot, exportTitle, exportProfile string
	var exportMemoryIDs []string
	export := &cobra.Command{
		Use: "export", Short: "Export a bounded workspace memory view", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBridges(cmd.Context(), options, func(service *runtime.BridgeService) error {
				receipt, err := service.ExportWorkspace(cmd.Context(), runtime.ExportWorkspaceRequest{
					OperationID: exportOperationID, RepoRoot: exportRepoRoot, MemoryIDs: exportMemoryIDs,
					Title: exportTitle, TargetProfile: exportProfile,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	export.Flags().StringVar(&exportOperationID, "operation-id", "", "idempotency key")
	export.Flags().StringVar(&exportRepoRoot, "repo-root", "", "confirmed workspace root")
	export.Flags().StringSliceVar(&exportMemoryIDs, "memory-id", nil, "selected active workspace memory id")
	export.Flags().StringVar(&exportTitle, "title", "", "export title")
	export.Flags().StringVar(&exportProfile, "target-profile", "", "target consumer profile")
	markRequired(export, "operation-id", "repo-root", "memory-id", "title", "target-profile")

	var adoptOperationID, adoptExistingRoot, adoptNewRoot, adoptExistingNamespace, adoptNewNamespace string
	adopt := &cobra.Command{
		Use: "adopt", Short: "Add a confirmed alias to an existing workspace", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBridges(cmd.Context(), options, func(service *runtime.BridgeService) error {
				receipt, err := service.AdoptWorkspaceAnchor(cmd.Context(), runtime.AdoptWorkspaceAnchorRequest{
					OperationID: adoptOperationID, ExistingRepoRoot: adoptExistingRoot, NewRepoRoot: adoptNewRoot,
					ExistingNamespaceID: adoptExistingNamespace, NewNamespaceID: adoptNewNamespace,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	adopt.Flags().StringVar(&adoptOperationID, "operation-id", "", "idempotency key")
	adopt.Flags().StringVar(&adoptExistingRoot, "existing-repo-root", "", "existing confirmed workspace root")
	adopt.Flags().StringVar(&adoptNewRoot, "new-repo-root", "", "new alias root")
	adopt.Flags().StringVar(&adoptExistingNamespace, "existing-filesystem-namespace", "", "existing trusted filesystem namespace")
	adopt.Flags().StringVar(&adoptNewNamespace, "new-filesystem-namespace", "", "new trusted filesystem namespace")
	markRequired(adopt, "operation-id", "existing-repo-root", "new-repo-root")

	var rebindOperationID, rebindOldRoot, rebindNewRoot, rebindOldNamespace, rebindNewNamespace string
	rebind := &cobra.Command{
		Use: "rebind", Short: "Move a workspace continuity to a new root", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBridges(cmd.Context(), options, func(service *runtime.BridgeService) error {
				receipt, err := service.RebindWorkspace(cmd.Context(), runtime.RebindWorkspaceRequest{
					OperationID: rebindOperationID, OldRepoRoot: rebindOldRoot, NewRepoRoot: rebindNewRoot,
					OldNamespaceID: rebindOldNamespace, NewNamespaceID: rebindNewNamespace,
				})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	rebind.Flags().StringVar(&rebindOperationID, "operation-id", "", "idempotency key")
	rebind.Flags().StringVar(&rebindOldRoot, "old-repo-root", "", "current confirmed workspace root")
	rebind.Flags().StringVar(&rebindNewRoot, "new-repo-root", "", "replacement workspace root")
	rebind.Flags().StringVar(&rebindOldNamespace, "old-filesystem-namespace", "", "old trusted filesystem namespace")
	rebind.Flags().StringVar(&rebindNewNamespace, "new-filesystem-namespace", "", "new trusted filesystem namespace")
	markRequired(rebind, "operation-id", "old-repo-root", "new-repo-root")

	var reverseOperationID, reverseBridgeID string
	reverse := &cobra.Command{
		Use: "reverse", Short: "Reverse or revoke one bridge", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBridges(cmd.Context(), options, func(service *runtime.BridgeService) error {
				receipt, err := service.Reverse(cmd.Context(), runtime.ReverseBridgeRequest{OperationID: reverseOperationID, BridgeID: reverseBridgeID})
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	reverse.Flags().StringVar(&reverseOperationID, "operation-id", "", "idempotency key")
	reverse.Flags().StringVar(&reverseBridgeID, "bridge-id", "", "bridge to reverse or revoke")
	markRequired(reverse, "operation-id", "bridge-id")

	var inspectBridgeID string
	inspect := &cobra.Command{
		Use: "inspect", Short: "Inspect one durable bridge", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withBridges(cmd.Context(), options, func(service *runtime.BridgeService) error {
				receipt, err := service.Inspect(cmd.Context(), inspectBridgeID)
				if err != nil {
					return err
				}
				return writeJSON(cmd, receipt)
			})
		},
	}
	inspect.Flags().StringVar(&inspectBridgeID, "bridge-id", "", "bridge to inspect")
	markRequired(inspect, "bridge-id")

	command.AddCommand(promote, link, export, adopt, rebind, reverse, inspect)
	return command
}

func addConnectionFlags(command *cobra.Command, options *connectionOptions) {
	command.PersistentFlags().StringVar(&options.databaseURL, "database-url", "", "PostgreSQL connection URL")
	command.PersistentFlags().StringVar(&options.tenantID, "tenant-id", "", "server-owned tenant identifier")
	command.PersistentFlags().StringVar(&options.filesystemNamespace, "filesystem-namespace", "", "trusted filesystem namespace for workspace governance")
	_ = command.MarkPersistentFlagRequired("database-url")
	_ = command.MarkPersistentFlagRequired("tenant-id")
}

func markRequired(command *cobra.Command, names ...string) {
	for _, name := range names {
		_ = command.MarkFlagRequired(name)
	}
}

func withGovernance(ctx context.Context, options connectionOptions, run func(*runtime.GovernanceService) error) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(options.tenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	store, err := runtime.OpenStore(ctx, options.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	return run(runtime.NewGovernanceServiceWithNamespace(store, options.tenantID, options.filesystemNamespace))
}

func withMemoryEligibility(ctx context.Context, options connectionOptions, run func(*runtime.MemoryEligibilityService) error) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(options.tenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	store, err := runtime.OpenStore(ctx, options.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	return run(runtime.NewMemoryEligibilityService(store, options.tenantID))
}

func parseOptionalUTCEligibilityTime(name, raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be RFC3339: %w", name, err)
	}
	_, offset := parsed.Zone()
	if offset != 0 {
		return nil, fmt.Errorf("%s must use UTC", name)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func withGlobalDefaults(ctx context.Context, options connectionOptions, run func(*runtime.GlobalDefaultsService) error) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(options.tenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	store, err := runtime.OpenStore(ctx, options.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	return run(runtime.NewGlobalDefaultsService(store, options.tenantID))
}

func withBridges(ctx context.Context, options connectionOptions, run func(*runtime.BridgeService) error) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(options.tenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	store, err := runtime.OpenStore(ctx, options.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	return run(runtime.NewBridgeService(store, options.tenantID))
}

func withSourceMatching(
	ctx context.Context,
	options connectionOptions,
	llm provider.Provider,
	providerName string,
	model string,
	run func(*runtime.Store, *runtime.SourceMatchingService) error,
) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(options.tenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	store, err := runtime.OpenStore(ctx, options.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	return run(store, runtime.NewSourceMatchingService(store, options.tenantID, llm, providerName, model))
}

func withSourceFormation(
	ctx context.Context,
	options connectionOptions,
	llm provider.Provider,
	providerName string,
	model string,
	run func(*runtime.Store, *runtime.SourceFormationService) error,
) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(options.tenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	store, err := runtime.OpenStore(ctx, options.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	return run(store, runtime.NewSourceFormationService(store, options.tenantID, llm, providerName, model))
}

func withConversation(
	ctx context.Context,
	options connectionOptions,
	run func(*runtime.Store, *runtime.ConversationService) error,
) error {
	if strings.TrimSpace(options.databaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(options.tenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	store, err := runtime.OpenStore(ctx, options.databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return err
	}
	return run(store, runtime.NewConversationService(store, options.tenantID, nil, "", runtime.ConversationServiceConfig{}))
}

func buildDirectProvider(name, model, baseURL, apiKeyEnv, grokCommand string, disableThinking bool) (provider.Provider, string, string, error) {
	return provider.BuildDirect(provider.DirectOptions{
		Name: name, Model: model, BaseURL: baseURL, APIKeyEnv: apiKeyEnv,
		GrokCommand: grokCommand, DisableThinking: disableThinking,
	})
}

func readSourceFormationFile(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("--source-file is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source file must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > 65536 {
		return nil, fmt.Errorf("source file must contain between 1 and 65536 bytes")
	}
	document, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return nil, fmt.Errorf("read source file: %w", err)
	}
	if len(document) == 0 || len(document) > 65536 {
		return nil, fmt.Errorf("source file must contain between 1 and 65536 bytes")
	}
	return document, nil
}

func writeJSON(cmd *cobra.Command, value any) error {
	return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
}

func writeMutationJSON(cmd *cobra.Command, service *runtime.GovernanceService, repoRoot string, receipt runtime.GovernedObservationReceipt) error {
	resolution, err := service.InspectWorkspace(cmd.Context(), repoRoot)
	if err != nil {
		return err
	}
	return writeJSON(cmd, mutationOutput{
		ContinuityID:  resolution.ContinuityID,
		ObservationID: receipt.Observation.ObservationID,
		MemoryID:      receipt.Memory.MemoryID,
		MemoryStatus:  receipt.Memory.Status,
		Replayed:      receipt.Observation.Replayed || receipt.Memory.Replayed,
	})
}

func writeSourceCandidateJSON(cmd *cobra.Command, service *runtime.GovernanceService, repoRoot string, receipt runtime.SourceCandidateReceipt) error {
	resolution, err := service.InspectWorkspace(cmd.Context(), repoRoot)
	if err != nil {
		return err
	}
	return writeJSON(cmd, sourceCandidateOutput{
		ContinuityID:      resolution.ContinuityID,
		RepoRoot:          resolution.RepoRoot,
		Disposition:       receipt.Disposition,
		MemoryKey:         receipt.MemoryKey,
		TargetMemoryID:    receipt.TargetMemoryID,
		ObservationID:     receipt.Observation.ObservationID,
		CandidateMemoryID: receipt.Candidate.MemoryID,
		CandidateStatus:   receipt.Candidate.Status,
		Replayed:          receipt.Replayed,
	})
}

func writeSourceMatchJSON(cmd *cobra.Command, store *runtime.Store, tenantID, repoRoot string, receipt runtime.SourceMatchReceipt) error {
	governance := runtime.NewGovernanceService(store, tenantID)
	resolution, memories, err := governance.ListWorkspaceMemories(cmd.Context(), repoRoot)
	if err != nil {
		return err
	}
	candidateStatus := ""
	for _, memory := range memories {
		if memory.ID == receipt.CandidateMemoryID {
			candidateStatus = memory.LifecycleStatus
			break
		}
	}
	return writeJSON(cmd, sourceMatchOutput{
		SourceMatchID:      receipt.ID,
		ContinuityID:       resolution.ContinuityID,
		RepoRoot:           resolution.RepoRoot,
		Decision:           receipt.Status,
		SelectedMemoryKey:  receipt.SelectedMemoryKey,
		MatchedMemoryID:    receipt.TargetMemoryID,
		ObservationID:      receipt.ObservationID,
		CandidateMemoryID:  receipt.CandidateMemoryID,
		CandidateStatus:    candidateStatus,
		Disposition:        receipt.Disposition,
		Provider:           receipt.ProviderName,
		Model:              receipt.ResolvedModel,
		FailureCode:        receipt.FailureCode,
		Reason:             receipt.Reason,
		CandidateSetSHA256: receipt.CandidateSetFingerprint,
		ProviderSHA256:     receipt.ProviderArtifactSHA256,
		Replayed:           receipt.Replayed,
	})
}

func writeSourceFormationJSON(cmd *cobra.Command, store *runtime.Store, tenantID, repoRoot string, receipt runtime.SourceFormationReceipt) error {
	governance := runtime.NewGovernanceService(store, tenantID)
	resolution, memories, err := governance.ListWorkspaceMemories(cmd.Context(), repoRoot)
	if err != nil {
		return err
	}
	statusByMemoryID := make(map[string]string, len(memories))
	for _, memory := range memories {
		statusByMemoryID[memory.ID] = memory.LifecycleStatus
	}
	items := append([]runtime.SourceFormationItemReceipt(nil), receipt.Items...)
	for index := range items {
		if items[index].CandidateMemoryID != "" {
			items[index].CandidateStatus = statusByMemoryID[items[index].CandidateMemoryID]
		}
	}
	model := receipt.ResolvedModel
	if model == "" {
		model = receipt.RequestedModel
	}
	return writeJSON(cmd, sourceFormationOutput{
		SourceFormationID:      receipt.ID,
		ContinuityID:           resolution.ContinuityID,
		RepoRoot:               resolution.RepoRoot,
		Status:                 receipt.Status,
		InputKind:              receipt.InputKind,
		InputManifestSHA256:    receipt.InputManifestFingerprint,
		SourceRef:              receipt.SourceRef,
		SourceSHA256:           receipt.SourceSHA256,
		SourceBytes:            receipt.SourceBytes,
		ActiveSnapshotSHA256:   receipt.ActiveSnapshotFingerprint,
		Provider:               receipt.ProviderName,
		Model:                  model,
		ProviderArtifactSHA256: receipt.ProviderArtifactSHA256,
		FailureCode:            receipt.FailureCode,
		Reason:                 receipt.Reason,
		Items:                  items,
		Replayed:               receipt.Replayed,
	})
}

func writeConversationFormationJSON(
	cmd *cobra.Command,
	store *runtime.Store,
	tenantID string,
	anchor runtime.ConversationAnchor,
	receipt runtime.SourceFormationReceipt,
) error {
	service := runtime.NewConversationService(store, tenantID, nil, "", runtime.ConversationServiceConfig{})
	inspection, err := service.Inspect(cmd.Context(), anchor)
	if err != nil {
		return err
	}
	statusByMemoryID := make(map[string]string, len(inspection.Memories))
	for _, memory := range inspection.Memories {
		statusByMemoryID[memory.ID] = memory.LifecycleStatus
	}
	items := append([]runtime.SourceFormationItemReceipt(nil), receipt.Items...)
	for index := range items {
		if items[index].CandidateMemoryID != "" {
			items[index].CandidateStatus = statusByMemoryID[items[index].CandidateMemoryID]
		}
	}
	model := receipt.ResolvedModel
	if model == "" {
		model = receipt.RequestedModel
	}
	return writeJSON(cmd, sourceFormationOutput{
		SourceFormationID:      receipt.ID,
		ContinuityID:           inspection.Resolution.ContinuityID,
		Channel:                inspection.Resolution.Channel,
		ThreadID:               inspection.Resolution.ThreadID,
		Status:                 receipt.Status,
		InputKind:              receipt.InputKind,
		InputManifestSHA256:    receipt.InputManifestFingerprint,
		SourceRef:              receipt.SourceRef,
		SourceSHA256:           receipt.SourceSHA256,
		SourceBytes:            receipt.SourceBytes,
		ActiveSnapshotSHA256:   receipt.ActiveSnapshotFingerprint,
		Provider:               receipt.ProviderName,
		Model:                  model,
		ProviderArtifactSHA256: receipt.ProviderArtifactSHA256,
		FailureCode:            receipt.FailureCode,
		Reason:                 receipt.Reason,
		Items:                  items,
		Replayed:               receipt.Replayed,
	})
}

func writeConversationMutationJSON(
	cmd *cobra.Command,
	service *runtime.ConversationService,
	anchor runtime.ConversationAnchor,
	receipt runtime.GovernedObservationReceipt,
) error {
	inspection, err := service.Inspect(cmd.Context(), anchor)
	if err != nil {
		return err
	}
	return writeJSON(cmd, mutationOutput{
		ContinuityID:  inspection.Resolution.ContinuityID,
		ObservationID: receipt.Observation.ObservationID,
		MemoryID:      receipt.Memory.MemoryID,
		MemoryStatus:  receipt.Memory.Status,
		Replayed:      receipt.Observation.Replayed || receipt.Memory.Replayed,
	})
}
