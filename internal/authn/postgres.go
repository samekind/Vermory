package authn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresAuthenticator struct {
	pool *pgxpool.Pool
}

func NewPostgresAuthenticator(pool *pgxpool.Pool) *PostgresAuthenticator {
	return &PostgresAuthenticator{pool: pool}
}

func (authenticator *PostgresAuthenticator) Authenticate(ctx context.Context, raw string) (Principal, error) {
	if authenticator == nil || authenticator.pool == nil {
		return Principal{}, errors.New("authenticate API token: database pool is required")
	}
	parsed, err := ParseToken(raw)
	if err != nil {
		return Principal{}, ErrAuthenticationFailed
	}
	var principal Principal
	if err := authenticator.pool.QueryRow(ctx, `
SELECT token_id::text, tenant_id, subject_id, role, expires_at
FROM vermory_auth.authenticate_token($1, $2)`, parsed.PublicID, parsed.Digest[:]).Scan(
		&principal.TokenID,
		&principal.TenantID,
		&principal.SubjectID,
		&principal.Role,
		&principal.ExpiresAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrAuthenticationFailed
		}
		return Principal{}, fmt.Errorf("authenticate API token: %w", err)
	}
	return principal, nil
}

func IssueToken(ctx context.Context, pool *pgxpool.Pool, request IssueTokenRequest) (IssueTokenReceipt, error) {
	if pool == nil {
		return IssueTokenReceipt{}, errors.New("issue API token: database pool is required")
	}
	now := time.Now().UTC()
	if err := request.Validate(now); err != nil {
		return IssueTokenReceipt{}, err
	}
	raw, err := NewToken()
	if err != nil {
		return IssueTokenReceipt{}, err
	}
	digest := raw.Digest()
	fingerprint := issueRequestFingerprint(request)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return IssueTokenReceipt{}, fmt.Errorf("begin API token issue: %w", err)
	}
	defer tx.Rollback(ctx)

	inspection, err := scanTokenInspection(tx.QueryRow(ctx, `
INSERT INTO vermory_auth.api_tokens (
  issue_operation_id, request_fingerprint, public_id, token_digest,
  tenant_id, subject_id, role, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, issue_operation_id) DO NOTHING
RETURNING id::text, public_id, tenant_id, subject_id, role, status,
          expires_at, created_at, revoked_at`,
		request.OperationID,
		fingerprint,
		raw.PublicID(),
		digest[:],
		strings.TrimSpace(request.TenantID),
		strings.TrimSpace(request.SubjectID),
		request.Role,
		request.ExpiresAt.UTC(),
	), false)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return IssueTokenReceipt{}, fmt.Errorf("commit API token issue: %w", err)
		}
		return IssueTokenReceipt{Token: raw, Inspection: inspection}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return IssueTokenReceipt{}, fmt.Errorf("issue API token: %w", err)
	}

	var existingFingerprint, publicID string
	if err := tx.QueryRow(ctx, `
SELECT request_fingerprint, public_id
FROM vermory_auth.api_tokens
WHERE tenant_id = $1 AND issue_operation_id = $2
FOR UPDATE`, strings.TrimSpace(request.TenantID), request.OperationID).Scan(&existingFingerprint, &publicID); err != nil {
		return IssueTokenReceipt{}, fmt.Errorf("inspect API token issue replay: %w", err)
	}
	if existingFingerprint != fingerprint {
		return IssueTokenReceipt{}, ErrIdempotencyConflict
	}
	inspection, err = inspectToken(ctx, tx, publicID)
	if err != nil {
		return IssueTokenReceipt{}, err
	}
	inspection.Replayed = true
	if err := tx.Commit(ctx); err != nil {
		return IssueTokenReceipt{}, fmt.Errorf("commit API token issue replay: %w", err)
	}
	return IssueTokenReceipt{Inspection: inspection, Replayed: true}, nil
}

func InspectToken(ctx context.Context, pool *pgxpool.Pool, publicID string) (TokenInspection, error) {
	if pool == nil {
		return TokenInspection{}, errors.New("inspect API token: database pool is required")
	}
	if !validPublicID(publicID) {
		return TokenInspection{}, ErrInvalidIdentityRequest
	}
	return inspectToken(ctx, pool, publicID)
}

func RevokeToken(ctx context.Context, pool *pgxpool.Pool, request RevokeTokenRequest) (TokenInspection, error) {
	if pool == nil {
		return TokenInspection{}, errors.New("revoke API token: database pool is required")
	}
	if err := request.Validate(); err != nil {
		return TokenInspection{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return TokenInspection{}, fmt.Errorf("begin API token revoke: %w", err)
	}
	defer tx.Rollback(ctx)

	inspection, revokeOperationID, err := inspectTokenForUpdate(ctx, tx, request.PublicID)
	if err != nil {
		return TokenInspection{}, err
	}
	if inspection.Status == TokenStatusRevoked {
		if revokeOperationID != request.OperationID {
			return TokenInspection{}, ErrIdempotencyConflict
		}
		inspection.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return TokenInspection{}, fmt.Errorf("commit API token revoke replay: %w", err)
		}
		return inspection, nil
	}
	inspection, err = scanTokenInspection(tx.QueryRow(ctx, `
UPDATE vermory_auth.api_tokens
SET status = 'revoked', revoked_at = now(), revoke_operation_id = $2
WHERE public_id = $1
RETURNING id::text, public_id, tenant_id, subject_id, role, status,
          expires_at, created_at, revoked_at`, request.PublicID, request.OperationID), false)
	if err != nil {
		return TokenInspection{}, fmt.Errorf("revoke API token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TokenInspection{}, fmt.Errorf("commit API token revoke: %w", err)
	}
	return inspection, nil
}

type tokenQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func inspectToken(ctx context.Context, query tokenQuerier, publicID string) (TokenInspection, error) {
	inspection, err := scanTokenInspection(query.QueryRow(ctx, `
SELECT id::text, public_id, tenant_id, subject_id, role, status,
       expires_at, created_at, revoked_at
FROM vermory_auth.api_tokens
WHERE public_id = $1`, publicID), false)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenInspection{}, ErrTokenNotFound
	}
	if err != nil {
		return TokenInspection{}, fmt.Errorf("inspect API token: %w", err)
	}
	return inspection, nil
}

func inspectTokenForUpdate(ctx context.Context, tx pgx.Tx, publicID string) (TokenInspection, string, error) {
	var inspection TokenInspection
	var revokeOperationID string
	err := tx.QueryRow(ctx, `
SELECT id::text, public_id, tenant_id, subject_id, role, status,
       expires_at, created_at, revoked_at, revoke_operation_id
FROM vermory_auth.api_tokens
WHERE public_id = $1
FOR UPDATE`, publicID).Scan(
		&inspection.TokenID,
		&inspection.PublicID,
		&inspection.TenantID,
		&inspection.SubjectID,
		&inspection.Role,
		&inspection.Status,
		&inspection.ExpiresAt,
		&inspection.CreatedAt,
		&inspection.RevokedAt,
		&revokeOperationID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return TokenInspection{}, "", ErrTokenNotFound
	}
	if err != nil {
		return TokenInspection{}, "", fmt.Errorf("inspect API token for revoke: %w", err)
	}
	return inspection, revokeOperationID, nil
}

func scanTokenInspection(row pgx.Row, replayed bool) (TokenInspection, error) {
	inspection := TokenInspection{Replayed: replayed}
	err := row.Scan(
		&inspection.TokenID,
		&inspection.PublicID,
		&inspection.TenantID,
		&inspection.SubjectID,
		&inspection.Role,
		&inspection.Status,
		&inspection.ExpiresAt,
		&inspection.CreatedAt,
		&inspection.RevokedAt,
	)
	return inspection, err
}

func issueRequestFingerprint(request IssueTokenRequest) string {
	canonical := strings.Join([]string{
		strings.TrimSpace(request.TenantID),
		strings.TrimSpace(request.SubjectID),
		request.Role.String(),
		request.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}
