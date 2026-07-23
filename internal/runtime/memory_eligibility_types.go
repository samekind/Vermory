package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type MemoryEffectiveState string

const (
	MemoryEffectiveProposed   MemoryEffectiveState = "proposed"
	MemoryEffectiveCurrent    MemoryEffectiveState = "current"
	MemoryEffectiveScheduled  MemoryEffectiveState = "scheduled"
	MemoryEffectiveExpired    MemoryEffectiveState = "expired"
	MemoryEffectiveArchived   MemoryEffectiveState = "archived"
	MemoryEffectiveSuperseded MemoryEffectiveState = "superseded"
	MemoryEffectiveRejected   MemoryEffectiveState = "rejected"
	MemoryEffectiveDeleted    MemoryEffectiveState = "deleted"
)

type EligibilitySnapshot struct {
	AsOf time.Time `json:"as_of"`
}

type MemoryValidity struct {
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

func (v MemoryValidity) Normalized() (MemoryValidity, error) {
	normalized := MemoryValidity{
		ValidFrom:  normalizedEligibilityTime(v.ValidFrom),
		ValidUntil: normalizedEligibilityTime(v.ValidUntil),
	}
	if normalized.ValidFrom != nil && normalized.ValidUntil != nil && !normalized.ValidUntil.After(*normalized.ValidFrom) {
		return MemoryValidity{}, fmt.Errorf("valid_until must be after valid_from")
	}
	return normalized, nil
}

func normalizedEligibilityTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func normalizeEligibilityAsOf(asOf time.Time) (time.Time, error) {
	if asOf.IsZero() {
		return time.Time{}, fmt.Errorf("eligibility as_of is required")
	}
	return asOf.UTC(), nil
}

func EffectiveMemoryState(lifecycle, content string, validity MemoryValidity, asOf time.Time) MemoryEffectiveState {
	lifecycle = strings.TrimSpace(lifecycle)
	content = strings.TrimSpace(content)
	switch lifecycle {
	case "deleted":
		return MemoryEffectiveDeleted
	case "superseded":
		return MemoryEffectiveSuperseded
	case "rejected":
		return MemoryEffectiveRejected
	case "proposed":
		return MemoryEffectiveProposed
	case "archived":
		return MemoryEffectiveArchived
	case "active":
		if content == "[redacted]" {
			return MemoryEffectiveDeleted
		}
		asOf = asOf.UTC()
		if validity.ValidFrom != nil && asOf.Before(validity.ValidFrom.UTC()) {
			return MemoryEffectiveScheduled
		}
		if validity.ValidUntil != nil && !asOf.Before(validity.ValidUntil.UTC()) {
			return MemoryEffectiveExpired
		}
		return MemoryEffectiveCurrent
	default:
		return MemoryEffectiveProposed
	}
}

func (s *Store) CurrentEligibilitySnapshot(ctx context.Context, tenantID string) (EligibilitySnapshot, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return EligibilitySnapshot{}, err
	}
	var asOf time.Time
	if err := s.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&asOf); err != nil {
		return EligibilitySnapshot{}, fmt.Errorf("read eligibility clock: %w", err)
	}
	return EligibilitySnapshot{AsOf: asOf.UTC()}, nil
}

func currentEligibilitySnapshotTx(ctx context.Context, tx pgx.Tx) (EligibilitySnapshot, error) {
	var asOf time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&asOf); err != nil {
		return EligibilitySnapshot{}, fmt.Errorf("read transaction eligibility clock: %w", err)
	}
	return EligibilitySnapshot{AsOf: asOf.UTC()}, nil
}
