package runtime

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	ProductionRetrievalProfileID           = "siliconflow-bge-m3-1024-v1"
	MigrationRetrievalProfileID            = "siliconflow-bge-large-zh-1024-v2"
	DimensionalMigrationRetrievalProfileID = "siliconflow-qwen3-embedding-4b-2560-v3"
	ProjectionStatusRebuildRequired        = "rebuild_required"
	ProjectionFailureRebuildRequired       = "projection_rebuild_required"
)

type ProjectionClass string

const (
	ProjectionClass1024 ProjectionClass = "vector_1024"
	ProjectionClass2560 ProjectionClass = "halfvec_2560"
)

type RetrievalProfileSpec struct {
	ID              string
	BaseURL         string
	Model           string
	Dimensions      int
	ProjectionClass ProjectionClass
	Status          string
}

func SupportedRetrievalProfile(id string) (RetrievalProfileSpec, bool) {
	specs := map[string]RetrievalProfileSpec{
		ProductionRetrievalProfileID: {
			ID: ProductionRetrievalProfileID, BaseURL: "https://api.siliconflow.cn/v1",
			Model: "BAAI/bge-m3", Dimensions: 1024, ProjectionClass: ProjectionClass1024, Status: "active",
		},
		MigrationRetrievalProfileID: {
			ID: MigrationRetrievalProfileID, BaseURL: "https://api.siliconflow.cn/v1",
			Model: "BAAI/bge-large-zh-v1.5", Dimensions: 1024, ProjectionClass: ProjectionClass1024, Status: "candidate",
		},
		DimensionalMigrationRetrievalProfileID: {
			ID: DimensionalMigrationRetrievalProfileID, BaseURL: "https://api.siliconflow.cn/v1",
			Model: "Qwen/Qwen3-Embedding-4B", Dimensions: 2560, ProjectionClass: ProjectionClass2560, Status: "candidate",
		},
	}
	spec, ok := specs[strings.TrimSpace(id)]
	return spec, ok
}

func IsSupportedRetrievalProfileID(id string) bool {
	_, ok := SupportedRetrievalProfile(id)
	return ok
}

type RetrievalMode string

const (
	RetrievalLexical RetrievalMode = "lexical"
	RetrievalShadow  RetrievalMode = "shadow"
	RetrievalVector  RetrievalMode = "vector"
)

type RetrievalRequest struct {
	OperationID     string
	TenantID        string
	ContinuityIDs   []string
	Query           string
	Limit           int
	Mode            RetrievalMode
	EligibilityAsOf time.Time
}

func (r RetrievalRequest) normalized() (RetrievalRequest, error) {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.TenantID = strings.TrimSpace(r.TenantID)
	r.Query = strings.TrimSpace(r.Query)
	if r.Mode == "" {
		r.Mode = RetrievalLexical
	}
	if r.Mode != RetrievalLexical && r.Mode != RetrievalShadow && r.Mode != RetrievalVector {
		return RetrievalRequest{}, fmt.Errorf("retrieval mode must be lexical, shadow, or vector")
	}
	if r.Mode != RetrievalLexical && r.OperationID == "" {
		return RetrievalRequest{}, fmt.Errorf("retrieval operation ID is required")
	}
	if r.TenantID == "" {
		return RetrievalRequest{}, fmt.Errorf("retrieval tenant ID is required")
	}
	if len(r.ContinuityIDs) == 0 || len(r.ContinuityIDs) > 50 {
		return RetrievalRequest{}, fmt.Errorf("retrieval requires between 1 and 50 continuity IDs")
	}
	continuityIDs := make([]string, 0, len(r.ContinuityIDs))
	seen := make(map[string]struct{}, len(r.ContinuityIDs))
	for _, continuityID := range r.ContinuityIDs {
		continuityID = strings.TrimSpace(continuityID)
		if continuityID == "" {
			return RetrievalRequest{}, fmt.Errorf("retrieval continuity ID is required")
		}
		if _, exists := seen[continuityID]; exists {
			return RetrievalRequest{}, fmt.Errorf("retrieval continuity IDs must be unique")
		}
		seen[continuityID] = struct{}{}
		continuityIDs = append(continuityIDs, continuityID)
	}
	sort.Strings(continuityIDs)
	r.ContinuityIDs = continuityIDs
	if r.Query == "" {
		return RetrievalRequest{}, fmt.Errorf("retrieval query is required")
	}
	if r.Limit < 0 {
		return RetrievalRequest{}, fmt.Errorf("retrieval limit cannot be negative")
	}
	if r.Limit == 0 {
		r.Limit = defaultContextItems
	}
	if r.Limit > maxContextItems {
		r.Limit = maxContextItems
	}
	if !r.EligibilityAsOf.IsZero() {
		r.EligibilityAsOf = r.EligibilityAsOf.UTC()
	}
	return r, nil
}

type RetrievalResult struct {
	Memories        []Memory
	Effective       RetrievalMode
	Degraded        bool
	FailureCode     string
	AuditID         string
	EligibilityAsOf time.Time
}

type MemoryRetriever interface {
	Retrieve(context.Context, RetrievalRequest) (RetrievalResult, error)
}

type RetrievalProfile struct {
	ID              string
	BaseURL         string
	Model           string
	Dimensions      int
	ProjectionClass ProjectionClass
}

func (p RetrievalProfile) Validate() error {
	spec, ok := SupportedRetrievalProfile(p.ID)
	if !ok {
		return fmt.Errorf("unsupported retrieval profile %q", p.ID)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("embedding base URL is invalid or contains credentials")
	}
	if baseURL != spec.BaseURL {
		return fmt.Errorf("embedding base URL must use direct SiliconFlow v1")
	}
	if strings.TrimSpace(p.Model) != spec.Model {
		return fmt.Errorf("embedding model must be %s", spec.Model)
	}
	if p.Dimensions != spec.Dimensions {
		return fmt.Errorf("embedding dimensions must be %d", spec.Dimensions)
	}
	if p.ProjectionClass != spec.ProjectionClass {
		return fmt.Errorf("retrieval projection class must be %s", spec.ProjectionClass)
	}
	return nil
}

type Embedder interface {
	Embed(context.Context, string) ([]float32, error)
}

type BatchEmbedder interface {
	EmbedBatch(context.Context, []string) ([][]float32, error)
}

type ProjectionStatus struct {
	TenantID             string     `json:"tenant_id"`
	ProfileID            string     `json:"profile_id"`
	LastEventID          int64      `json:"last_event_id"`
	LatestEventID        int64      `json:"latest_event_id"`
	PrunedThroughEventID int64      `json:"pruned_through_event_id"`
	Lag                  int64      `json:"lag"`
	Status               string     `json:"status"`
	RebuildRequired      bool       `json:"rebuild_required"`
	AttemptCount         int        `json:"attempt_count"`
	LastErrorCode        string     `json:"last_error_code,omitempty"`
	LastAttemptAt        *time.Time `json:"last_attempt_at,omitempty"`
	VectorCount          int64      `json:"vector_count"`
}

type ProjectionEvent struct {
	EventID          int64
	TenantID         string
	ContinuityID     string
	MemoryID         string
	DesiredState     string
	AuthorityVersion time.Time
}

type ProjectionWorkerOptions struct {
	TenantID           string
	Profile            RetrievalProfile
	BatchSize          int
	EmbeddingBatchSize int
	SnapshotPageSize   int
	PollInterval       time.Duration
}

func (o *ProjectionWorkerOptions) normalize() error {
	o.TenantID = strings.TrimSpace(o.TenantID)
	if o.TenantID == "" {
		return fmt.Errorf("projection worker tenant ID is required")
	}
	if err := o.Profile.Validate(); err != nil {
		return err
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 32
	}
	if o.BatchSize > 256 {
		o.BatchSize = 256
	}
	if o.EmbeddingBatchSize <= 0 {
		o.EmbeddingBatchSize = 1
	}
	if o.EmbeddingBatchSize > 256 {
		o.EmbeddingBatchSize = 256
	}
	if o.SnapshotPageSize <= 0 {
		o.SnapshotPageSize = 128
	}
	if o.SnapshotPageSize > 1000 {
		o.SnapshotPageSize = 1000
	}
	if o.PollInterval <= 0 {
		o.PollInterval = time.Second
	}
	return nil
}

type ProjectionRunResult struct {
	Processed      int    `json:"processed"`
	LastEventID    int64  `json:"last_event_id"`
	LatestEventID  int64  `json:"latest_event_id"`
	Lag            int64  `json:"lag"`
	Status         string `json:"status"`
	FailureCode    string `json:"failure_code,omitempty"`
	AlreadyRunning bool   `json:"already_running"`
}

type ProjectionRebuildResult struct {
	Scanned        int    `json:"scanned"`
	Projected      int    `json:"projected"`
	SkippedChanged int    `json:"skipped_changed"`
	Watermark      int64  `json:"watermark"`
	LastEventID    int64  `json:"last_event_id"`
	LatestEventID  int64  `json:"latest_event_id"`
	Lag            int64  `json:"lag"`
	Status         string `json:"status"`
	FailureCode    string `json:"failure_code,omitempty"`
	AlreadyRunning bool   `json:"already_running"`
}

type projectionRunError struct {
	code string
}

func (e projectionRunError) Error() string {
	return "projection worker: " + e.code
}
