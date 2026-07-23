package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"vermory/internal/brand"
	"vermory/internal/resolver"
	"vermory/internal/runtime"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	TenantID  string
	Workspace runtime.WorkspaceAnchor
}

type Handler struct {
	service   *runtime.Service
	tenantID  string
	workspace runtime.WorkspaceAnchor
	configErr error
}

type PrepareContextInput struct {
	OperationID string `json:"operation_id" jsonschema:"stable id for this context preparation operation"`
	Task        string `json:"task" jsonschema:"current task that needs governed context"`
	MaxItems    int    `json:"max_items,omitempty" jsonschema:"maximum number of governed facts to return"`
	// These fields are retained for direct Go API compatibility only. json:"-"
	// keeps them out of the model-visible MCP schema; production stdio always
	// supplies the workspace through startup configuration.
	RepoRoot string `json:"-"`
	CWD      string `json:"-"`
}

type PrepareContextOutput struct {
	Status     string `json:"status" jsonschema:"workspace resolution status"`
	DeliveryID string `json:"delivery_id,omitempty" jsonschema:"opaque receipt for a later observation writeback"`
	Context    string `json:"context" jsonschema:"governed semantic context for the current task"`
}

type CommitObservationInput struct {
	OperationID string `json:"operation_id" jsonschema:"stable id for this post-task observation"`
	DeliveryID  string `json:"delivery_id" jsonschema:"opaque receipt returned by prepare_context"`
	Content     string `json:"content" jsonschema:"post-task result or observation proposed by the coding agent"`
	SourceRef   string `json:"source_ref,omitempty" jsonschema:"optional non-sensitive source reference for audit"`
}

type CommitObservationOutput struct {
	ObservationID string `json:"observation_id" jsonschema:"opaque observation receipt"`
	MemoryID      string `json:"memory_id,omitempty" jsonschema:"opaque proposed memory receipt"`
	MemoryStatus  string `json:"memory_status" jsonschema:"governance state assigned to this writeback"`
	Replayed      bool   `json:"replayed" jsonschema:"whether this operation id replayed an existing result"`
}

func New(service *runtime.Service, config Config) *Handler {
	handler := &Handler{service: service, tenantID: strings.TrimSpace(config.TenantID)}
	if _, err := config.Workspace.Normalized(); err != nil {
		handler.configErr = err
	} else if config.Workspace.RepoRoot != "" {
		handler.workspace = config.Workspace
	} else {
		handler.configErr = fmt.Errorf("trusted workspace attachment is required")
	}
	return handler
}

func NewWithAttachment(service *runtime.Service, tenantID string, attachment resolver.WorkspaceAttachment) *Handler {
	workspace, err := runtime.WorkspaceAnchorFromAttachment(attachment)
	if err != nil {
		return &Handler{service: service, tenantID: strings.TrimSpace(tenantID), configErr: err}
	}
	return New(service, Config{TenantID: tenantID, Workspace: workspace})
}

func NewServer(handler *Handler) *mcp.Server {
	server := mcp.NewServer(serverImplementation(), nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "prepare_context",
		Description: "Resolve a workspace and return governed context for the current task.",
	}, handler.PrepareContext)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "commit_observation",
		Description: "Record a coding-agent result as a proposed observation for the prepared workspace.",
	}, handler.CommitObservation)
	return server
}

func serverImplementation() *mcp.Implementation {
	return &mcp.Implementation{Name: brand.Slug, Version: brand.Version}
}

func (h *Handler) PrepareContext(ctx context.Context, _ *mcp.CallToolRequest, input PrepareContextInput) (*mcp.CallToolResult, PrepareContextOutput, error) {
	if h == nil || h.service == nil || h.tenantID == "" {
		return nil, PrepareContextOutput{}, fmt.Errorf("MCP handler is not configured")
	}
	workspace, err := h.workspaceForInput(input)
	if err != nil {
		return nil, PrepareContextOutput{}, err
	}
	if err := h.validateWorkspace(workspace); err != nil {
		return nil, PrepareContextOutput{}, err
	}
	if h.workspace.RepoRoot == "" {
		h.workspace = workspace
		h.configErr = nil
	}
	result, err := h.service.PrepareContext(ctx, runtime.PrepareContextRequest{
		OperationID: input.OperationID,
		Workspace:   workspace,
		Task:        input.Task,
		MaxItems:    input.MaxItems,
	})
	if err != nil {
		return nil, PrepareContextOutput{}, err
	}
	return nil, PrepareContextOutput{
		Status:     string(result.Status),
		DeliveryID: result.DeliveryID,
		Context:    result.Context,
	}, nil
}

func (h *Handler) CommitObservation(ctx context.Context, _ *mcp.CallToolRequest, input CommitObservationInput) (*mcp.CallToolResult, CommitObservationOutput, error) {
	if err := h.validate(); err != nil {
		return nil, CommitObservationOutput{}, err
	}
	result, err := h.service.CommitObservation(ctx, runtime.CommitObservationRequest{
		OperationID: input.OperationID,
		DeliveryID:  input.DeliveryID,
		Kind:        runtime.ObservationKindAgentResult,
		Content:     input.Content,
		SourceRef:   input.SourceRef,
	})
	if err != nil {
		return nil, CommitObservationOutput{}, err
	}
	return nil, CommitObservationOutput{
		ObservationID: result.ObservationID,
		MemoryID:      result.MemoryID,
		MemoryStatus:  result.MemoryStatus,
		Replayed:      result.Replayed,
	}, nil
}

func (h *Handler) validate() error {
	if h == nil || h.service == nil || h.tenantID == "" {
		return fmt.Errorf("MCP handler is not configured")
	}
	if h.configErr != nil {
		return fmt.Errorf("MCP handler trusted workspace attachment: %w", h.configErr)
	}
	return nil
}

func (h *Handler) workspaceForInput(input PrepareContextInput) (runtime.WorkspaceAnchor, error) {
	if h.workspace.RepoRoot != "" {
		return h.workspace, nil
	}
	if strings.TrimSpace(input.RepoRoot) == "" {
		return runtime.WorkspaceAnchor{}, fmt.Errorf("MCP handler trusted workspace attachment: %w", h.configErr)
	}
	workspace, err := (runtime.WorkspaceAnchor{RepoRoot: input.RepoRoot, CWD: input.CWD}).Normalized()
	if err != nil {
		return runtime.WorkspaceAnchor{}, err
	}
	return workspace, nil
}

func (h *Handler) validateWorkspace(workspace runtime.WorkspaceAnchor) error {
	if h.configErr != nil && h.workspace.RepoRoot != "" {
		return fmt.Errorf("MCP handler trusted workspace attachment: %w", h.configErr)
	}
	_, err := workspace.Normalized()
	return err
}
