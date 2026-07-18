package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	storepostgres "vermory/internal/store/postgres"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type ResolutionStatus string

const (
	ResolutionResolved          ResolutionStatus = "resolved"
	ResolutionNeedsConfirmation ResolutionStatus = "needs_confirmation"
	ResolutionUnresolved        ResolutionStatus = "unresolved"
)

type WorkspaceResolution struct {
	Status              ResolutionStatus
	ContinuityID        string
	RepoRoot            string
	FilesystemNamespace string
}

type ObservationReceipt struct {
	ObservationID string `json:"observation_id"`
	Replayed      bool   `json:"replayed"`
}

type Memory struct {
	ID      string
	Content string
}

type GovernedMemory struct {
	ID                 string               `json:"id"`
	MemoryKey          string               `json:"memory_key,omitempty"`
	LifecycleStatus    string               `json:"lifecycle_status"`
	Content            string               `json:"content"`
	SupersedesMemoryID string               `json:"supersedes_memory_id,omitempty"`
	ValidFrom          *time.Time           `json:"valid_from,omitempty"`
	ValidUntil         *time.Time           `json:"valid_until,omitempty"`
	EffectiveState     MemoryEffectiveState `json:"effective_state"`
}

type DeliveryReceipt struct {
	DeliveryID      string    `json:"delivery_id"`
	Context         string    `json:"context"`
	EligibilityAsOf time.Time `json:"eligibility_as_of"`
	Replayed        bool      `json:"replayed"`
}

type MemoryReceipt struct {
	MemoryID string `json:"memory_id"`
	Status   string `json:"status"`
	Replayed bool   `json:"replayed"`
}

type GovernedObservationReceipt struct {
	Observation ObservationReceipt `json:"observation"`
	Memory      MemoryReceipt      `json:"memory"`
}

type Store struct {
	pool                             *pgxpool.Pool
	databaseURL                      string
	memoryEligibilityAfterTargetLock func()
	memoryEligibilityBeforeCommit    func()
	memoryDeleteAfterTargetLock      func()
}

type StoreOptions struct {
	EnforceTenantContext bool
}

func OpenStore(ctx context.Context, databaseURL string) (*Store, error) {
	return OpenStoreWithOptions(ctx, databaseURL, StoreOptions{})
}

func OpenStoreWithOptions(ctx context.Context, databaseURL string, options StoreOptions) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("runtime store: database URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open runtime store: %w", err)
	}
	if options.EnforceTenantContext {
		previousPrepare := config.PrepareConn
		config.PrepareConn = func(acquireCtx context.Context, conn *pgx.Conn) (bool, error) {
			if previousPrepare != nil {
				valid, err := previousPrepare(acquireCtx, conn)
				if !valid || err != nil {
					return valid, err
				}
			}
			tenantID, ok := tenantFromContext(acquireCtx)
			if !ok {
				if _, err := conn.Exec(acquireCtx, `SELECT set_config('vermory.tenant_id', '', false)`); err != nil {
					return false, err
				}
				return true, ErrTenantContextRequired
			}
			if _, err := conn.Exec(acquireCtx, `SELECT set_config('vermory.tenant_id', $1, false)`, tenantID); err != nil {
				return false, err
			}
			return true, nil
		}
		previousRelease := config.AfterRelease
		config.AfterRelease = func(conn *pgx.Conn) bool {
			resetCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := conn.Exec(resetCtx, `SELECT set_config('vermory.tenant_id', '', false)`); err != nil {
				return false
			}
			return previousRelease == nil || previousRelease(conn)
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open runtime store: %w", err)
	}
	return &Store{pool: pool, databaseURL: databaseURL}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}
	goose.SetBaseFS(storepostgres.Migrations)
	defer goose.SetBaseFS(nil)
	return goose.UpContext(ctx, db, "migrations")
}

func (s *Store) SchemaVersion(ctx context.Context) (int64, error) {
	var version int64
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(max(version_id) FILTER (WHERE is_applied), 0) FROM goose_db_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func (s *Store) ResetForTest(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
	TRUNCATE vermory_auth.api_tokens,
	  memory_eligibility_operations,
	  memory_projection_prune_runs, memory_projection_retention,
	  memory_retrieval_runs, memory_vector_documents_2560, memory_vector_documents,
	  memory_projection_cursors, memory_projection_events,
	  conversation_formation_schedules,
	  source_formation_items, source_formation_runs, source_match_decisions,
	  conversation_links, bridge_memory_effects, bridge_events, bridge_operations,
	  memory_search_documents, memory_deliveries, governed_memories,
	  conversation_tool_results, conversation_turns, observations,
	  conversation_bindings, continuity_bindings, continuity_spaces CASCADE`)
	if err != nil {
		return fmt.Errorf("reset runtime store: %w", err)
	}
	return nil
}

func (s *Store) ResolveWorkspace(ctx context.Context, tenantID string, anchor WorkspaceAnchor) (WorkspaceResolution, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return WorkspaceResolution{}, err
	}
	anchor, err = anchor.Normalized()
	if err != nil {
		return WorkspaceResolution{}, err
	}
	if anchor.ExplicitBindingID != "" {
		var continuityID string
		err := s.pool.QueryRow(ctx, `
		SELECT b.continuity_id::text
		FROM continuity_bindings b
		JOIN continuity_spaces c ON c.id = b.continuity_id
		WHERE b.continuity_id::text = $1 AND b.tenant_id = $2
		  AND b.filesystem_namespace = $3 AND b.repo_root = $4
		  AND b.binding_state = 'confirmed'
		  AND c.continuity_line = 'workspace' AND c.state = 'active'`,
			anchor.ExplicitBindingID, tenantID, anchor.FilesystemNamespace, anchor.RepoRoot).Scan(&continuityID)
		if err == nil {
			return WorkspaceResolution{Status: ResolutionResolved, ContinuityID: continuityID, RepoRoot: anchor.RepoRoot, FilesystemNamespace: anchor.FilesystemNamespace}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return WorkspaceResolution{}, fmt.Errorf("resolve explicit workspace binding: %w", err)
		}
		return WorkspaceResolution{Status: ResolutionNeedsConfirmation, RepoRoot: anchor.RepoRoot, FilesystemNamespace: anchor.FilesystemNamespace}, nil
	}

	var continuityID string
	err = s.pool.QueryRow(ctx, `
SELECT b.continuity_id::text
FROM continuity_bindings b
JOIN continuity_spaces c ON c.id = b.continuity_id
WHERE b.tenant_id = $1 AND b.filesystem_namespace = $2 AND b.repo_root = $3
  AND b.binding_state = 'confirmed'
  AND c.continuity_line = 'workspace' AND c.state = 'active'`, tenantID, anchor.FilesystemNamespace, anchor.RepoRoot).Scan(&continuityID)
	if err == nil {
		return WorkspaceResolution{Status: ResolutionResolved, ContinuityID: continuityID, RepoRoot: anchor.RepoRoot, FilesystemNamespace: anchor.FilesystemNamespace}, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkspaceResolution{Status: ResolutionNeedsConfirmation, RepoRoot: anchor.RepoRoot, FilesystemNamespace: anchor.FilesystemNamespace}, nil
	}
	return WorkspaceResolution{}, fmt.Errorf("resolve workspace binding: %w", err)
}

func (s *Store) ValidateRuntimeRole(ctx context.Context) error {
	validationCtx, err := withTenantContext(ctx, "__runtime_validation__")
	if err != nil {
		return ErrUnsafeRuntimeRole
	}
	var canLogin, superuser, bypassRLS bool
	if err := s.pool.QueryRow(validationCtx, `
SELECT rolcanlogin, rolsuper, rolbypassrls
FROM pg_roles
WHERE rolname = current_user`).Scan(&canLogin, &superuser, &bypassRLS); err != nil {
		return fmt.Errorf("validate runtime database role: %w", err)
	}
	if !canLogin || superuser || bypassRLS {
		return ErrUnsafeRuntimeRole
	}
	readWriteTables := []string{
		"continuity_spaces", "continuity_bindings", "conversation_bindings", "observations",
		"memory_deliveries", "memory_search_documents", "conversation_turns",
		"conversation_tool_results",
		"bridge_operations", "bridge_events", "bridge_memory_effects", "conversation_links",
		"source_match_decisions", "source_formation_runs", "source_formation_items",
		"conversation_formation_schedules",
		"memory_projection_events", "memory_projection_cursors", "memory_vector_documents",
		"memory_vector_documents_2560", "memory_retrieval_runs",
	}
	readOnlyTables := []string{"memory_eligibility_operations", "memory_projection_retention"}
	ownedTableSet := append(append([]string(nil), readWriteTables...), readOnlyTables...)
	ownedTableSet = append(ownedTableSet, "governed_memories", "memory_projection_prune_runs")
	var ownedTables int
	if err := s.pool.QueryRow(validationCtx, `
SELECT count(*)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public'
	  AND c.relname = ANY($1::text[])
	  AND c.relowner = (SELECT oid FROM pg_roles WHERE rolname = current_user)`, ownedTableSet).Scan(&ownedTables); err != nil {
		return fmt.Errorf("validate runtime table ownership: %w", err)
	}
	if ownedTables != 0 {
		return ErrUnsafeRuntimeRole
	}
	var hasRequiredPrivileges bool
	if err := s.pool.QueryRow(validationCtx, `
SELECT
  COALESCE(bool_and(
    has_table_privilege(current_user, 'public.' || required.table_name, 'SELECT')
    AND has_table_privilege(current_user, 'public.' || required.table_name, 'INSERT')
    AND has_table_privilege(current_user, 'public.' || required.table_name, 'UPDATE')
    AND has_table_privilege(current_user, 'public.' || required.table_name, 'DELETE')
  ), false)
	  AND has_sequence_privilege(current_user, 'public.observations_observation_seq_seq', 'USAGE')
	  AND has_sequence_privilege(current_user, 'public.memory_projection_events_event_id_seq', 'USAGE')
  AND has_function_privilege(current_user, 'vermory_auth.authenticate_token(text,bytea)', 'EXECUTE')
FROM unnest($1::text[]) AS required(table_name)`, readWriteTables).Scan(&hasRequiredPrivileges); err != nil {
		return fmt.Errorf("validate runtime required privileges: %w", err)
	}
	if !hasRequiredPrivileges {
		return ErrUnsafeRuntimeRole
	}
	var governedBoundary bool
	if err := s.pool.QueryRow(validationCtx, `
SELECT
  has_table_privilege(current_user, 'public.governed_memories', 'SELECT')
  AND has_table_privilege(current_user, 'public.governed_memories', 'DELETE')
  AND NOT has_table_privilege(current_user, 'public.governed_memories', 'INSERT')
  AND NOT has_table_privilege(current_user, 'public.governed_memories', 'UPDATE')
  AND has_column_privilege(current_user, 'public.governed_memories', 'tenant_id', 'INSERT')
  AND has_column_privilege(current_user, 'public.governed_memories', 'continuity_id', 'INSERT')
  AND has_column_privilege(current_user, 'public.governed_memories', 'origin_observation_id', 'INSERT')
  AND has_column_privilege(current_user, 'public.governed_memories', 'memory_kind', 'INSERT')
  AND has_column_privilege(current_user, 'public.governed_memories', 'memory_key', 'INSERT')
  AND has_column_privilege(current_user, 'public.governed_memories', 'lifecycle_status', 'INSERT,UPDATE')
  AND has_column_privilege(current_user, 'public.governed_memories', 'content', 'INSERT,UPDATE')
  AND has_column_privilege(current_user, 'public.governed_memories', 'supersedes_memory_id', 'INSERT')
  AND has_column_privilege(current_user, 'public.governed_memories', 'updated_at', 'UPDATE')
  AND NOT has_column_privilege(current_user, 'public.governed_memories', 'valid_from', 'INSERT,UPDATE')
  AND NOT has_column_privilege(current_user, 'public.governed_memories', 'valid_until', 'INSERT,UPDATE')`).Scan(&governedBoundary); err != nil {
		return fmt.Errorf("validate governed memory column boundary: %w", err)
	}
	if !governedBoundary {
		return ErrUnsafeRuntimeRole
	}
	var hasReadOnlyBoundary bool
	if err := s.pool.QueryRow(validationCtx, `
SELECT COALESCE(bool_and(
  has_table_privilege(current_user, 'public.' || required.table_name, 'SELECT')
  AND NOT has_table_privilege(current_user, 'public.' || required.table_name, 'INSERT')
  AND NOT has_table_privilege(current_user, 'public.' || required.table_name, 'UPDATE')
  AND NOT has_table_privilege(current_user, 'public.' || required.table_name, 'DELETE')
), false)
FROM unnest($1::text[]) AS required(table_name)`, readOnlyTables).Scan(&hasReadOnlyBoundary); err != nil {
		return fmt.Errorf("validate runtime read-only privileges: %w", err)
	}
	if !hasReadOnlyBoundary {
		return ErrUnsafeRuntimeRole
	}
	forbiddenTables := []string{
		"vermory_auth.api_tokens",
		"public.projects", "public.sources", "public.source_versions", "public.claims",
		"public.capsules", "public.capsule_claims", "public.packets", "public.audit_logs", "public.wcef_runs",
		"public.memory_projection_prune_runs",
	}
	var hasForbiddenPrivileges bool
	if err := s.pool.QueryRow(validationCtx, `
SELECT EXISTS (
  SELECT 1
  FROM unnest($1::text[]) AS forbidden(table_name)
  WHERE has_table_privilege(current_user, forbidden.table_name, 'SELECT')
     OR has_table_privilege(current_user, forbidden.table_name, 'INSERT')
     OR has_table_privilege(current_user, forbidden.table_name, 'UPDATE')
     OR has_table_privilege(current_user, forbidden.table_name, 'DELETE')
)`, forbiddenTables).Scan(&hasForbiddenPrivileges); err != nil {
		return fmt.Errorf("validate runtime privilege boundary: %w", err)
	}
	if hasForbiddenPrivileges {
		return ErrUnsafeRuntimeRole
	}
	return nil
}

func (s *Store) ValidateProjectionPruneOperatorRole(ctx context.Context) error {
	validationCtx, err := withTenantContext(ctx, "__prune_validation__")
	if err != nil {
		return ErrUnsafePruneRole
	}
	var allowed bool
	if err := s.pool.QueryRow(validationCtx, `
SELECT has_table_privilege(current_user, 'public.memory_projection_events', 'SELECT')
   AND has_table_privilege(current_user, 'public.memory_projection_events', 'DELETE')
   AND has_table_privilege(current_user, 'public.memory_projection_cursors', 'SELECT')
   AND has_table_privilege(current_user, 'public.memory_projection_retention', 'SELECT')
   AND has_table_privilege(current_user, 'public.memory_projection_retention', 'INSERT')
   AND has_table_privilege(current_user, 'public.memory_projection_retention', 'UPDATE')
   AND has_table_privilege(current_user, 'public.memory_projection_prune_runs', 'SELECT')
   AND has_table_privilege(current_user, 'public.memory_projection_prune_runs', 'INSERT')`).Scan(&allowed); err != nil {
		return fmt.Errorf("validate projection prune database role: %w", err)
	}
	if !allowed {
		return ErrUnsafePruneRole
	}
	return nil
}

func (s *Store) ConfirmWorkspaceBinding(ctx context.Context, tenantID, repoRoot string) (string, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return s.ConfirmWorkspaceAnchorBinding(ctx, tenantID, WorkspaceAnchor{RepoRoot: repoRoot})
}

func (s *Store) ConfirmWorkspaceAnchorBinding(ctx context.Context, tenantID string, anchor WorkspaceAnchor) (string, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return "", err
	}
	anchor, err = anchor.Normalized()
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin workspace binding: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx, `
SELECT continuity_id::text FROM continuity_bindings
WHERE tenant_id = $1 AND filesystem_namespace = $2 AND repo_root = $3 AND binding_state = 'confirmed'`, tenantID, anchor.FilesystemNamespace, anchor.RepoRoot).Scan(&existingID)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit existing workspace binding: %w", err)
		}
		return existingID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup workspace binding: %w", err)
	}

	var continuityID string
	if err := tx.QueryRow(ctx, `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ($1, 'workspace', 'active')
RETURNING id::text`, tenantID).Scan(&continuityID); err != nil {
		return "", fmt.Errorf("create workspace continuity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO continuity_bindings (continuity_id, tenant_id, filesystem_namespace, repo_root, binding_state)
	VALUES ($1::uuid, $2, $3, $4, 'confirmed')`, continuityID, tenantID, anchor.FilesystemNamespace, anchor.RepoRoot); err != nil {
		return "", fmt.Errorf("create workspace binding: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit workspace binding: %w", err)
	}
	return continuityID, nil
}

func (s *Store) CommitObservation(ctx context.Context, tenantID, continuityID string, request CommitObservationRequest) (ObservationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return ObservationReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return ObservationReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ObservationReceipt{}, fmt.Errorf("begin observation: %w", err)
	}
	defer tx.Rollback(ctx)

	receipt, err := commitObservationTx(ctx, tx, tenantID, continuityID, request)
	if err != nil {
		return ObservationReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ObservationReceipt{}, fmt.Errorf("commit observation: %w", err)
	}
	return receipt, nil
}

func (s *Store) CommitGovernedObservation(ctx context.Context, tenantID, continuityID string, request CommitObservationRequest) (GovernedObservationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return GovernedObservationReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("begin governed observation: %w", err)
	}
	defer tx.Rollback(ctx)

	observation, err := commitObservationTx(ctx, tx, tenantID, continuityID, request)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	var memory MemoryReceipt
	if request.Kind == ObservationKindForgetRequest {
		if err := deleteMemoryTx(ctx, tx, tenantID, continuityID, request.TargetMemoryID); err != nil {
			return GovernedObservationReceipt{}, err
		}
		memory = MemoryReceipt{MemoryID: request.TargetMemoryID, Status: "deleted", Replayed: observation.Replayed}
	} else {
		memory, err = governObservationTx(ctx, tx, tenantID, continuityID, observation.ObservationID, request)
		if err != nil {
			return GovernedObservationReceipt{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return GovernedObservationReceipt{}, fmt.Errorf("commit governed observation: %w", err)
	}
	return GovernedObservationReceipt{Observation: observation, Memory: memory}, nil
}

func commitObservationTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID string, request CommitObservationRequest) (ObservationReceipt, error) {
	var validContinuity bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_spaces
  WHERE id = $1::uuid AND tenant_id = $2
    AND continuity_line IN ('workspace', 'conversation', 'global_defaults') AND state = 'active'
)`, continuityID, tenantID).Scan(&validContinuity); err != nil {
		return ObservationReceipt{}, fmt.Errorf("check observation continuity: %w", err)
	}
	if !validContinuity {
		return ObservationReceipt{}, fmt.Errorf("continuity is not active for this tenant")
	}

	var existingID, existingContinuityID, existingKind, existingContent, existingSourceRef, existingMemoryKey string
	err := tx.QueryRow(ctx, `
SELECT id::text, continuity_id::text, observation_kind, content, source_ref, memory_key
FROM observations
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, request.OperationID).Scan(
		&existingID,
		&existingContinuityID,
		&existingKind,
		&existingContent,
		&existingSourceRef,
		&existingMemoryKey,
	)
	if err == nil {
		if existingContinuityID != continuityID ||
			existingKind != string(request.Kind) ||
			existingContent != request.Content ||
			existingSourceRef != request.SourceRef ||
			existingMemoryKey != request.MemoryKey {
			return ObservationReceipt{}, fmt.Errorf("operation_id is already bound to another logical observation")
		}
		return ObservationReceipt{ObservationID: existingID, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ObservationReceipt{}, fmt.Errorf("lookup observation receipt: %w", err)
	}

	var observationID string
	if err := tx.QueryRow(ctx, `
	INSERT INTO observations (tenant_id, continuity_id, operation_id, observation_kind, content, source_ref, memory_key)
	VALUES ($1, $2::uuid, $3, $4, $5, $6, $7)
	RETURNING id::text`, tenantID, continuityID, request.OperationID, request.Kind, request.Content, request.SourceRef, request.MemoryKey).Scan(&observationID); err != nil {
		return ObservationReceipt{}, fmt.Errorf("insert observation: %w", err)
	}
	return ObservationReceipt{ObservationID: observationID}, nil
}

func (s *Store) RecordDelivery(ctx context.Context, tenantID, continuityID, operationID, task, contextBody string) (DeliveryReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	snapshot, err := s.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	return s.RecordDeliveryAt(ctx, tenantID, continuityID, operationID, task, contextBody, snapshot.AsOf)
}

func (s *Store) RecordDeliveryAt(ctx context.Context, tenantID, continuityID, operationID, task, contextBody string, asOf time.Time) (DeliveryReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	asOf, err = normalizeEligibilityAsOf(asOf)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeliveryReceipt{}, fmt.Errorf("begin context delivery: %w", err)
	}
	defer tx.Rollback(ctx)

	var deliveryID, existingContinuityID, existingContext string
	var eligibilityAsOf time.Time
	err = tx.QueryRow(ctx, `
SELECT id::text, continuity_id::text, context_body, eligibility_as_of
FROM memory_deliveries
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(
		&deliveryID, &existingContinuityID, &existingContext, &eligibilityAsOf,
	)
	if err == nil {
		if existingContinuityID != continuityID {
			return DeliveryReceipt{}, fmt.Errorf("operation_id is already bound to another continuity")
		}
		if err := tx.Commit(ctx); err != nil {
			return DeliveryReceipt{}, fmt.Errorf("commit replayed context delivery: %w", err)
		}
		return DeliveryReceipt{
			DeliveryID: deliveryID, Context: existingContext,
			EligibilityAsOf: eligibilityAsOf.UTC(), Replayed: true,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DeliveryReceipt{}, fmt.Errorf("lookup context delivery: %w", err)
	}
	if err := tx.QueryRow(ctx, `
INSERT INTO memory_deliveries (
  tenant_id, continuity_id, operation_id, task, context_body, eligibility_as_of
)
VALUES ($1, $2::uuid, $3, $4, $5, $6)
RETURNING id::text, eligibility_as_of`, tenantID, continuityID, operationID, task, contextBody, asOf).Scan(
		&deliveryID, &eligibilityAsOf,
	); err != nil {
		return DeliveryReceipt{}, fmt.Errorf("record context delivery: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DeliveryReceipt{}, fmt.Errorf("commit context delivery: %w", err)
	}
	return DeliveryReceipt{
		DeliveryID: deliveryID, Context: contextBody,
		EligibilityAsOf: eligibilityAsOf.UTC(),
	}, nil
}

func (s *Store) DeliveryContinuity(ctx context.Context, tenantID, deliveryID string) (string, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return "", err
	}
	var continuityID string
	err = s.pool.QueryRow(ctx, `
SELECT continuity_id::text
FROM memory_deliveries
WHERE id = $1::uuid AND tenant_id = $2`, deliveryID, tenantID).Scan(&continuityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("delivery does not belong to this tenant")
	}
	if err != nil {
		return "", fmt.Errorf("resolve delivery continuity: %w", err)
	}
	return continuityID, nil
}

func (s *Store) GovernObservation(ctx context.Context, tenantID, continuityID, observationID string, request CommitObservationRequest) (MemoryReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return MemoryReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return MemoryReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MemoryReceipt{}, fmt.Errorf("begin governed memory: %w", err)
	}
	defer tx.Rollback(ctx)
	memory, err := governObservationTx(ctx, tx, tenantID, continuityID, observationID, request)
	if err != nil {
		return MemoryReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MemoryReceipt{}, fmt.Errorf("commit governed memory: %w", err)
	}
	return memory, nil
}

func governObservationTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, observationID string, request CommitObservationRequest) (MemoryReceipt, error) {
	var observationBelongsToContinuity bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM observations
  WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
)`, observationID, tenantID, continuityID).Scan(&observationBelongsToContinuity); err != nil {
		return MemoryReceipt{}, fmt.Errorf("check governed observation scope: %w", err)
	}
	if !observationBelongsToContinuity {
		return MemoryReceipt{}, fmt.Errorf("observation does not belong to this continuity")
	}

	var existingID, existingStatus, existingSupersedesMemoryID, existingMemoryKey string
	err := tx.QueryRow(ctx, `
SELECT id::text, lifecycle_status, COALESCE(supersedes_memory_id::text, ''), memory_key
FROM governed_memories
WHERE origin_observation_id = $1::uuid`, observationID).Scan(&existingID, &existingStatus, &existingSupersedesMemoryID, &existingMemoryKey)
	if err == nil {
		if existingSupersedesMemoryID != request.SupersedesMemoryID ||
			(request.MemoryKey != "" && existingMemoryKey != request.MemoryKey) {
			return MemoryReceipt{}, fmt.Errorf("operation_id is already bound to another supersession target")
		}
		return MemoryReceipt{MemoryID: existingID, Status: existingStatus, Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MemoryReceipt{}, fmt.Errorf("lookup governed memory: %w", err)
	}

	status := "proposed"
	if request.Kind == ObservationKindUserCorrection || request.Kind == ObservationKindSourceUpdate {
		status = "active"
	}
	if request.SupersedesMemoryID != "" {
		var targetMemoryKey string
		if err := tx.QueryRow(ctx, `
	SELECT memory_key
	FROM governed_memories
	WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid AND lifecycle_status = 'active'
	FOR UPDATE`, request.SupersedesMemoryID, tenantID, continuityID).Scan(&targetMemoryKey); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return MemoryReceipt{}, fmt.Errorf("superseded memory must be an active fact in the delivery continuity")
			}
			return MemoryReceipt{}, fmt.Errorf("lock superseded governed memory: %w", err)
		}
		if request.Kind == ObservationKindSourceCandidate && request.MemoryKey != targetMemoryKey {
			return MemoryReceipt{}, fmt.Errorf("source candidate memory_key does not match the active target")
		}
		if request.Kind != ObservationKindSourceCandidate && request.MemoryKey != "" && targetMemoryKey != "" && request.MemoryKey != targetMemoryKey {
			return MemoryReceipt{}, fmt.Errorf("source candidate memory_key does not match the active target")
		}
		if request.MemoryKey == "" {
			request.MemoryKey = targetMemoryKey
		}
		if request.Kind != ObservationKindSourceCandidate {
			command, err := tx.Exec(ctx, `
	UPDATE governed_memories
	SET lifecycle_status = 'superseded', updated_at = now()
	WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid AND lifecycle_status = 'active'`, request.SupersedesMemoryID, tenantID, continuityID)
			if err != nil {
				return MemoryReceipt{}, fmt.Errorf("supersede governed memory: %w", err)
			}
			if command.RowsAffected() != 1 {
				return MemoryReceipt{}, fmt.Errorf("superseded memory must be an active fact in the delivery continuity")
			}
			if _, err := tx.Exec(ctx, `DELETE FROM memory_search_documents WHERE memory_id = $1::uuid`, request.SupersedesMemoryID); err != nil {
				return MemoryReceipt{}, fmt.Errorf("remove superseded search document: %w", err)
			}
		}
	}

	var memoryID string
	if err := tx.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, memory_key, lifecycle_status, content, supersedes_memory_id
)
VALUES ($1, $2::uuid, $3::uuid, 'fact', $4, $5, $6, NULLIF($7, '')::uuid)
RETURNING id::text`, tenantID, continuityID, observationID, request.MemoryKey, status, request.Content, request.SupersedesMemoryID).Scan(&memoryID); err != nil {
		return MemoryReceipt{}, fmt.Errorf("create governed memory: %w", err)
	}
	if status == "active" {
		if _, err := tx.Exec(ctx, `
INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
VALUES ($1::uuid, $2, $3::uuid, $4, to_tsvector('simple', $4))`, memoryID, tenantID, continuityID, request.Content); err != nil {
			return MemoryReceipt{}, fmt.Errorf("project governed memory: %w", err)
		}
	}
	return MemoryReceipt{MemoryID: memoryID, Status: status}, nil
}

func (s *Store) DeleteMemory(ctx context.Context, tenantID, continuityID, memoryID string) error {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete governed memory: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := deleteMemoryTx(ctx, tx, tenantID, continuityID, memoryID, s.memoryDeleteAfterTargetLock); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete governed memory: %w", err)
	}
	return nil
}

func deleteMemoryTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, memoryID string, afterTargetLock ...func()) error {
	var lifecycleStatus string
	var originObservationID *string
	var memoryContent string
	err := tx.QueryRow(ctx, `
SELECT lifecycle_status, origin_observation_id::text, content
FROM governed_memories memory
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
FOR UPDATE`, memoryID, tenantID, continuityID).Scan(&lifecycleStatus, &originObservationID, &memoryContent)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("memory does not belong to this continuity")
	}
	if err != nil {
		return fmt.Errorf("lookup governed memory for deletion: %w", err)
	}
	if len(afterTargetLock) > 0 && afterTargetLock[0] != nil {
		afterTargetLock[0]()
	}
	if lifecycleStatus != "deleted" {
		if _, err := tx.Exec(ctx, `
UPDATE governed_memories
SET lifecycle_status = 'deleted', content = '[redacted]', updated_at = now()
WHERE id = $1::uuid`, memoryID); err != nil {
			return fmt.Errorf("redact governed memory: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM memory_search_documents WHERE memory_id = $1::uuid`, memoryID); err != nil {
		return fmt.Errorf("remove deleted search document: %w", err)
	}
	if err := redactSourceMatchMemoryTx(ctx, tx, tenantID, continuityID, memoryID, memoryContent); err != nil {
		return err
	}
	if err := redactSourceFormationMemoryTx(ctx, tx, tenantID, continuityID, memoryID); err != nil {
		return err
	}
	if originObservationID != nil {
		if err := redactConversationObservationTx(ctx, tx, tenantID, continuityID, *originObservationID); err != nil {
			return err
		}
	}
	if memoryContent != "" && memoryContent != "[redacted]" {
		if _, err := tx.Exec(ctx, `
UPDATE observations observation
SET content = '[redacted]'
WHERE observation.tenant_id = $1
  AND observation.id IN (
    SELECT turn.assistant_observation_id
    FROM conversation_turns turn
    JOIN memory_deliveries delivery ON delivery.id = turn.delivery_id
    WHERE turn.tenant_id = $1
      AND delivery.tenant_id = $1
      AND turn.assistant_observation_id IS NOT NULL
      AND position($2 IN delivery.context_body) > 0
  )`, tenantID, memoryContent); err != nil {
			return fmt.Errorf("redact downstream assistant observations: %w", err)
		}
		if _, err := tx.Exec(ctx, `
UPDATE conversation_turns turn
SET answer = '[redacted]', updated_at = now()
FROM memory_deliveries delivery
WHERE turn.delivery_id = delivery.id
  AND turn.tenant_id = $1
  AND delivery.tenant_id = $1
  AND position($2 IN delivery.context_body) > 0`, tenantID, memoryContent); err != nil {
			return fmt.Errorf("redact downstream conversation turns: %w", err)
		}
		query := `
UPDATE memory_deliveries
SET context_body = replace(context_body, $2, '[redacted]')
WHERE tenant_id = $1
	  AND position($2 IN context_body) > 0`
		if _, err := tx.Exec(ctx, query, tenantID, memoryContent); err != nil {
			return fmt.Errorf("redact memory from delivery history: %w", err)
		}
	}
	return nil
}

func redactConversationObservationTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, observationID string) error {
	if _, err := tx.Exec(ctx, `
UPDATE observations
SET content = '[redacted]'
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND (
    id = $3::uuid
    OR id IN (
      SELECT user_observation_id FROM conversation_turns
      WHERE tenant_id = $1 AND continuity_id = $2::uuid
        AND (user_observation_id = $3::uuid OR assistant_observation_id = $3::uuid)
      UNION
      SELECT assistant_observation_id FROM conversation_turns
      WHERE tenant_id = $1 AND continuity_id = $2::uuid
        AND assistant_observation_id IS NOT NULL
        AND (user_observation_id = $3::uuid OR assistant_observation_id = $3::uuid)
    )
  )`, tenantID, continuityID, observationID); err != nil {
		return fmt.Errorf("redact origin conversation turn observations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE conversation_turns
SET answer = '[redacted]', updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND (user_observation_id = $3::uuid OR assistant_observation_id = $3::uuid)`,
		tenantID, continuityID, observationID); err != nil {
		return fmt.Errorf("redact conversation turn: %w", err)
	}
	return nil
}

func (s *Store) RebuildProjection(ctx context.Context, tenantID, continuityID string) error {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin projection rebuild: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
DELETE FROM memory_search_documents
WHERE tenant_id = $1 AND continuity_id = $2::uuid`, tenantID, continuityID); err != nil {
		return fmt.Errorf("clear search projection: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
SELECT id, tenant_id, continuity_id, content, to_tsvector('simple', content)
FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND lifecycle_status = 'active'`, tenantID, continuityID); err != nil {
		return fmt.Errorf("rebuild search projection: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit projection rebuild: %w", err)
	}
	return nil
}

// RebuildAllProjections recreates disposable search state from authoritative active memories.
// Operational callers must use an administrative store rather than the tenant-scoped runtime pool.
func (s *Store) RebuildAllProjections(ctx context.Context) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin all-projection rebuild: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM memory_search_documents`); err != nil {
		return 0, fmt.Errorf("clear all search projections: %w", err)
	}
	result, err := tx.Exec(ctx, `
INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
SELECT id, tenant_id, continuity_id, content, to_tsvector('simple', content)
FROM governed_memories
WHERE lifecycle_status = 'active'`)
	if err != nil {
		return 0, fmt.Errorf("rebuild all search projections: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit all-projection rebuild: %w", err)
	}
	return result.RowsAffected(), nil
}

func (s *Store) ListGovernedMemories(ctx context.Context, tenantID, continuityID string) ([]GovernedMemory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	var asOf time.Time
	if err := s.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&asOf); err != nil {
		return nil, fmt.Errorf("read governed memory eligibility clock: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
SELECT id::text, memory_key, lifecycle_status, content,
       COALESCE(supersedes_memory_id::text, ''), valid_from, valid_until
FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid
ORDER BY created_at ASC, id ASC`, tenantID, continuityID)
	if err != nil {
		return nil, fmt.Errorf("list governed memories: %w", err)
	}
	defer rows.Close()

	memories := make([]GovernedMemory, 0)
	for rows.Next() {
		var memory GovernedMemory
		if err := rows.Scan(
			&memory.ID, &memory.MemoryKey, &memory.LifecycleStatus, &memory.Content,
			&memory.SupersedesMemoryID, &memory.ValidFrom, &memory.ValidUntil,
		); err != nil {
			return nil, fmt.Errorf("scan governed memory: %w", err)
		}
		memory.EffectiveState = EffectiveMemoryState(
			memory.LifecycleStatus,
			memory.Content,
			MemoryValidity{ValidFrom: memory.ValidFrom, ValidUntil: memory.ValidUntil},
			asOf,
		)
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate governed memories: %w", err)
	}
	return memories, nil
}

func (s *Store) SearchActiveMemory(ctx context.Context, tenantID, continuityID, query string, limit int) ([]Memory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.CurrentEligibilitySnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.SearchEligibleMemoryAt(ctx, tenantID, continuityID, query, limit, snapshot.AsOf)
}

func (s *Store) SearchEligibleMemoryAt(ctx context.Context, tenantID, continuityID, query string, limit int, asOf time.Time) ([]Memory, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	asOf, err = normalizeEligibilityAsOf(asOf)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if limit <= 0 {
		limit = defaultContextItems
	}
	if limit > maxContextItems {
		limit = maxContextItems
	}
	rows, err := s.pool.Query(ctx, `
WITH query_terms AS (
  SELECT
    lower($3)::text AS exact_query,
    plainto_tsquery('simple', $3) AS all_terms,
    to_tsquery('simple', array_to_string(tsvector_to_array(to_tsvector('simple', $3)), ' | ')) AS any_terms
), exact_matches AS (
  SELECT 1
  FROM memory_search_documents document
  JOIN governed_memories memory ON memory.id = document.memory_id
  JOIN observations origin ON origin.tenant_id = $1 AND origin.id = memory.origin_observation_id
  CROSS JOIN query_terms
  WHERE document.tenant_id = $1
    AND document.continuity_id = $2::uuid
    AND memory.tenant_id = $1
    AND memory.continuity_id = $2::uuid
	AND memory_is_eligible(
	  memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $5
	)
    AND position(query_terms.exact_query IN lower(document.content)) > 0
  LIMIT 1
)
SELECT memory.id::text, memory.content
FROM memory_search_documents document
JOIN governed_memories memory ON memory.id = document.memory_id
JOIN observations origin ON origin.tenant_id = $1 AND origin.id = memory.origin_observation_id
CROSS JOIN query_terms
WHERE document.tenant_id = $1
  AND document.continuity_id = $2::uuid
  AND memory.tenant_id = $1
  AND memory.continuity_id = $2::uuid
	AND memory_is_eligible(
	  memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $5
	)
  AND (
    position(query_terms.exact_query IN lower(document.content)) > 0
    OR (
      NOT EXISTS (SELECT 1 FROM exact_matches)
      AND (
        document.search_document @@ query_terms.any_terms
        OR similarity(lower(document.content), query_terms.exact_query) >= 0.2
      )
    )
  )
ORDER BY
  (position(query_terms.exact_query IN lower(document.content)) > 0) DESC,
  ts_rank(document.search_document, query_terms.all_terms) DESC,
  ts_rank(document.search_document, query_terms.any_terms) DESC,
  similarity(lower(document.content), query_terms.exact_query) DESC,
  CASE origin.observation_kind
    WHEN 'user_correction' THEN 4
    WHEN 'user_confirmation' THEN 4
    WHEN 'source_update' THEN 3
    WHEN 'bridge_promote' THEN 2
    ELSE 1
  END DESC,
  memory.updated_at DESC
LIMIT $4`, tenantID, continuityID, query, limit, asOf)
	if err != nil {
		return nil, fmt.Errorf("search active memory: %w", err)
	}
	defer rows.Close()
	memories := make([]Memory, 0)
	for rows.Next() {
		var memory Memory
		if err := rows.Scan(&memory.ID, &memory.Content); err != nil {
			return nil, fmt.Errorf("scan active memory: %w", err)
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active memory: %w", err)
	}
	return memories, nil
}
