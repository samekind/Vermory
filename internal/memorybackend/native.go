package memorybackend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const nativeIndexTable = "contextmesh_memory_index"

type NativeConfig struct {
	DatabaseURL      string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	EmbeddingModel   string
	Dimensions       int
	HTTPClient       *http.Client
}

type NativeBackend struct {
	pool              *pgxpool.Pool
	embedder          *openAIEmbedder
	dimensions        int
	embeddingRequests atomic.Int64
}

func NewNativeBackend(ctx context.Context, config NativeConfig) (*NativeBackend, error) {
	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}
	if config.Dimensions <= 0 || config.Dimensions > 4096 {
		return nil, fmt.Errorf("embedding dimensions must be between 1 and 4096")
	}
	embedder, err := newOpenAIEmbedder(
		config.EmbeddingBaseURL,
		config.EmbeddingAPIKey,
		config.EmbeddingModel,
		config.Dimensions,
		config.HTTPClient,
	)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open native backend database: %w", err)
	}
	backend := &NativeBackend{pool: pool, embedder: embedder, dimensions: config.Dimensions}
	if err := backend.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return backend, nil
}

func (b *NativeBackend) Name() string { return "native" }

func (b *NativeBackend) Close() { b.pool.Close() }

func (b *NativeBackend) Health(ctx context.Context) error {
	if err := b.pool.Ping(ctx); err != nil {
		return fmt.Errorf("native backend PostgreSQL: %w", err)
	}
	return nil
}

func (b *NativeBackend) Put(ctx context.Context, record Record) error {
	b.embeddingRequests.Add(1)
	vector, err := b.embedder.Embed(ctx, record.Content)
	if err != nil {
		return fmt.Errorf("embed record %q: %w", record.ID, err)
	}
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return fmt.Errorf("encode record metadata: %w", err)
	}
	_, err = b.pool.Exec(ctx, `
		INSERT INTO `+nativeIndexTable+` (
			tenant_id, continuity_id, continuity_line, record_id,
			source_id, source_version, lifecycle_status, content, metadata, embedding, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::vector, now())
		ON CONFLICT (tenant_id, continuity_id, record_id) DO UPDATE SET
			continuity_line = EXCLUDED.continuity_line,
			source_id = EXCLUDED.source_id,
			source_version = EXCLUDED.source_version,
			lifecycle_status = EXCLUDED.lifecycle_status,
			content = EXCLUDED.content,
			metadata = EXCLUDED.metadata,
			embedding = EXCLUDED.embedding,
			updated_at = now()`,
		record.Scope.TenantID, record.Scope.ContinuityID, record.Scope.ContinuityLine, record.ID,
		record.SourceID, record.SourceVersion, record.Status, record.Content, metadata, vectorLiteral(vector),
	)
	if err != nil {
		return fmt.Errorf("upsert native record %q: %w", record.ID, err)
	}
	return nil
}

func (b *NativeBackend) Search(ctx context.Context, query Query) ([]Result, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	var rows pgx.Rows
	var err error
	if query.Text == "" {
		rows, err = b.pool.Query(ctx, `
			SELECT continuity_line, record_id, source_id, source_version, lifecycle_status, content, metadata, 1.0
			FROM `+nativeIndexTable+`
			WHERE tenant_id = $1 AND continuity_id = $2 AND ($3 OR lifecycle_status = 'active')
			ORDER BY record_id
			LIMIT $4`, query.Scope.TenantID, query.Scope.ContinuityID, query.IncludeHistory, limit)
	} else {
		b.embeddingRequests.Add(1)
		vector, embedErr := b.embedder.Embed(ctx, query.Text)
		if embedErr != nil {
			return nil, fmt.Errorf("embed search query: %w", embedErr)
		}
		rows, err = b.pool.Query(ctx, `
			SELECT continuity_line, record_id, source_id, source_version, lifecycle_status, content, metadata,
				1 - (embedding <=> $3::vector) AS score
			FROM `+nativeIndexTable+`
			WHERE tenant_id = $1 AND continuity_id = $2 AND ($4 OR lifecycle_status = 'active')
			ORDER BY embedding <=> $3::vector
			LIMIT $5`, query.Scope.TenantID, query.Scope.ContinuityID, vectorLiteral(vector), query.IncludeHistory, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("query native index: %w", err)
	}
	defer rows.Close()
	results := make([]Result, 0, limit)
	for rows.Next() {
		var record Record
		var metadata []byte
		var score float64
		record.Scope = query.Scope
		if err := rows.Scan(
			&record.Scope.ContinuityLine, &record.ID, &record.SourceID, &record.SourceVersion,
			&record.Status, &record.Content, &metadata, &score,
		); err != nil {
			return nil, fmt.Errorf("scan native result: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &record.Metadata); err != nil {
				return nil, fmt.Errorf("decode native metadata: %w", err)
			}
		}
		results = append(results, Result{Record: record, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate native results: %w", err)
	}
	return results, nil
}

func (b *NativeBackend) Update(ctx context.Context, record Record) error {
	return b.Put(ctx, record)
}

func (b *NativeBackend) Delete(ctx context.Context, scope Scope, recordID string) error {
	_, err := b.pool.Exec(ctx, `DELETE FROM `+nativeIndexTable+` WHERE tenant_id = $1 AND continuity_id = $2 AND record_id = $3`, scope.TenantID, scope.ContinuityID, recordID)
	if err != nil {
		return fmt.Errorf("delete native record %q: %w", recordID, err)
	}
	return nil
}

func (b *NativeBackend) ResetScope(ctx context.Context, scope Scope) error {
	_, err := b.pool.Exec(ctx, `DELETE FROM `+nativeIndexTable+` WHERE tenant_id = $1 AND continuity_id = $2`, scope.TenantID, scope.ContinuityID)
	if err != nil {
		return fmt.Errorf("reset native scope: %w", err)
	}
	return nil
}

func (b *NativeBackend) RebuildScope(ctx context.Context, scope Scope, records []Record) error {
	if err := b.ResetScope(ctx, scope); err != nil {
		return err
	}
	for _, record := range records {
		if record.Status != "active" {
			continue
		}
		if err := b.Put(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (b *NativeBackend) Stats(ctx context.Context) (Stats, error) {
	stats := Stats{MeasuredAt: time.Now().UTC(), Extra: map[string]any{
		"provider": "postgresql-pgvector", "embedding_requests": b.embeddingRequests.Load(),
	}}
	if err := b.pool.QueryRow(ctx, `SELECT count(*), pg_total_relation_size('`+nativeIndexTable+`')`).Scan(&stats.RecordCount, &stats.DiskBytes); err != nil {
		return Stats{}, fmt.Errorf("read native backend stats: %w", err)
	}
	return stats, nil
}

func (b *NativeBackend) ensureSchema(ctx context.Context) error {
	if _, err := b.pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("enable pgvector: %w", err)
	}
	ddl := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			tenant_id text NOT NULL,
			continuity_id text NOT NULL,
			continuity_line text NOT NULL,
			record_id text NOT NULL,
			source_id text NOT NULL,
			source_version integer NOT NULL,
			lifecycle_status text NOT NULL,
			content text NOT NULL,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			embedding vector(%d) NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (tenant_id, continuity_id, record_id)
		)`, nativeIndexTable, b.dimensions)
	if _, err := b.pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("create native memory index: %w", err)
	}
	if _, err := b.pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS contextmesh_memory_index_scope_status
		ON `+nativeIndexTable+` (tenant_id, continuity_id, lifecycle_status)`); err != nil {
		return fmt.Errorf("create native scope index: %w", err)
	}
	if _, err := b.pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS contextmesh_memory_index_embedding_hnsw
		ON `+nativeIndexTable+` USING hnsw (embedding vector_cosine_ops)
		WITH (m = 16, ef_construction = 64)`); err != nil {
		return fmt.Errorf("create native HNSW index: %w", err)
	}
	return nil
}
