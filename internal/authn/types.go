package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleClient   Role = "client"
	RoleOperator Role = "operator"
	RoleOwner    Role = "owner"
)

var (
	ErrInvalidToken           = errors.New("invalid API token")
	ErrInvalidIdentityRequest = errors.New("invalid identity request")
	ErrAuthenticationFailed   = errors.New("authentication failed")
	ErrTokenNotFound          = errors.New("token not found")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
)

type Principal struct {
	TokenID   string    `json:"token_id"`
	TenantID  string    `json:"tenant_id"`
	SubjectID string    `json:"subject_id"`
	Role      Role      `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type IssueTokenRequest struct {
	OperationID string
	TenantID    string
	SubjectID   string
	Role        Role
	ExpiresAt   time.Time
}

func (request IssueTokenRequest) Validate(now time.Time) error {
	if strings.TrimSpace(request.OperationID) == "" ||
		strings.TrimSpace(request.TenantID) == "" ||
		strings.TrimSpace(request.SubjectID) == "" ||
		!request.Role.Valid() ||
		request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) {
		return ErrInvalidIdentityRequest
	}
	return nil
}

func (role Role) Valid() bool {
	switch role {
	case RoleClient, RoleOperator, RoleOwner:
		return true
	default:
		return false
	}
}

func (role Role) String() string {
	return string(role)
}

type TokenStatus string

const (
	TokenStatusActive  TokenStatus = "active"
	TokenStatusRevoked TokenStatus = "revoked"
)

type TokenInspection struct {
	TokenID   string      `json:"token_id"`
	PublicID  string      `json:"public_id"`
	TenantID  string      `json:"tenant_id"`
	SubjectID string      `json:"subject_id"`
	Role      Role        `json:"role"`
	Status    TokenStatus `json:"status"`
	ExpiresAt time.Time   `json:"expires_at"`
	CreatedAt time.Time   `json:"created_at"`
	RevokedAt *time.Time  `json:"revoked_at,omitempty"`
	Replayed  bool        `json:"replayed,omitempty"`
}

type IssueTokenReceipt struct {
	Token      RawToken        `json:"-"`
	Inspection TokenInspection `json:"inspection"`
	Replayed   bool            `json:"replayed"`
}

type RevokeTokenRequest struct {
	OperationID string
	PublicID    string
}

func (request RevokeTokenRequest) Validate() error {
	if strings.TrimSpace(request.OperationID) == "" || !validPublicID(request.PublicID) {
		return ErrInvalidIdentityRequest
	}
	return nil
}

func invalidRequest(field string) error {
	return fmt.Errorf("%w: %s", ErrInvalidIdentityRequest, field)
}
