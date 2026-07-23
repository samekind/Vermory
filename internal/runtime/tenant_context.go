package runtime

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

var (
	ErrTenantContextRequired = errors.New("runtime tenant context is required")
	ErrUnsafeRuntimeRole     = errors.New("database role is unsafe for authenticated runtime use")
	ErrUnsafePruneRole       = errors.New("database role is unsafe for projection pruning")
)

type tenantContextKey struct{}

func withTenantContext(ctx context.Context, tenantID string) (context.Context, error) {
	normalized := strings.TrimSpace(tenantID)
	if normalized == "" || len(normalized) > 256 || strings.IndexFunc(normalized, unicode.IsControl) >= 0 {
		return nil, ErrTenantContextRequired
	}
	return context.WithValue(ctx, tenantContextKey{}, normalized), nil
}

func tenantFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	tenantID, ok := ctx.Value(tenantContextKey{}).(string)
	return tenantID, ok && tenantID != ""
}
