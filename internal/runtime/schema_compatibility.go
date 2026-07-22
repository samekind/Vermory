package runtime

import (
	"context"
	"errors"
	"fmt"
)

const (
	MinimumSupportedSchemaVersion int64 = 24
	MaximumSupportedSchemaVersion int64 = 24
)

type SchemaCompatibilityStatus string

const (
	SchemaCompatibilityCompatible        SchemaCompatibilityStatus = "compatible"
	SchemaCompatibilityMigrationRequired SchemaCompatibilityStatus = "migration_required"
	SchemaCompatibilityBinaryTooOld      SchemaCompatibilityStatus = "binary_too_old"
	SchemaCompatibilityPreflightFailed   SchemaCompatibilityStatus = "preflight_failed"
)

var (
	ErrIncompatibleSchema           = errors.New("database schema is incompatible with this binary")
	ErrSchemaCompatibilityPreflight = errors.New("database schema compatibility preflight failed")
)

type SchemaCompatibilityReport struct {
	Status                 SchemaCompatibilityStatus `json:"status"`
	SchemaVersion          int64                     `json:"schema_version"`
	MinimumSupportedSchema int64                     `json:"minimum_supported_schema"`
	MaximumSupportedSchema int64                     `json:"maximum_supported_schema"`
	BinaryRevision         string                    `json:"binary_revision"`
	MigrationRequired      bool                      `json:"migration_required"`
}

func EvaluateSchemaCompatibility(schemaVersion int64, binaryRevision string) SchemaCompatibilityReport {
	report := SchemaCompatibilityReport{
		Status:                 SchemaCompatibilityCompatible,
		SchemaVersion:          schemaVersion,
		MinimumSupportedSchema: MinimumSupportedSchemaVersion,
		MaximumSupportedSchema: MaximumSupportedSchemaVersion,
		BinaryRevision:         binaryRevision,
	}
	switch {
	case schemaVersion < MinimumSupportedSchemaVersion:
		report.Status = SchemaCompatibilityMigrationRequired
		report.MigrationRequired = true
	case schemaVersion > MaximumSupportedSchemaVersion:
		report.Status = SchemaCompatibilityBinaryTooOld
	}
	return report
}

func NewSchemaCompatibilityPreflightReport(binaryRevision string) SchemaCompatibilityReport {
	return SchemaCompatibilityReport{
		Status:                 SchemaCompatibilityPreflightFailed,
		MinimumSupportedSchema: MinimumSupportedSchemaVersion,
		MaximumSupportedSchema: MaximumSupportedSchemaVersion,
		BinaryRevision:         binaryRevision,
	}
}

func (report SchemaCompatibilityReport) Compatible() bool {
	return report.Status == SchemaCompatibilityCompatible
}

func (report SchemaCompatibilityReport) ErrorIfIncompatible() error {
	if report.Compatible() {
		return nil
	}
	if report.Status == SchemaCompatibilityPreflightFailed {
		return ErrSchemaCompatibilityPreflight
	}
	return &SchemaCompatibilityError{report: report}
}

type SchemaCompatibilityError struct {
	report SchemaCompatibilityReport
}

func (err *SchemaCompatibilityError) Error() string {
	return fmt.Sprintf(
		"%s: status=%s schema=%d supported=%d..%d",
		ErrIncompatibleSchema,
		err.report.Status,
		err.report.SchemaVersion,
		err.report.MinimumSupportedSchema,
		err.report.MaximumSupportedSchema,
	)
}

func (err *SchemaCompatibilityError) Is(target error) bool {
	return target == ErrIncompatibleSchema
}

func (err *SchemaCompatibilityError) Report() SchemaCompatibilityReport {
	return err.report
}

func (s *Store) RuntimeSchemaVersion(ctx context.Context) (int64, error) {
	queryCtx, err := withTenantContext(ctx, "__schema_compatibility__")
	if err != nil {
		return 0, ErrSchemaCompatibilityPreflight
	}
	var version int64
	if err := s.pool.QueryRow(queryCtx, `SELECT vermory_auth.schema_version()`).Scan(&version); err != nil {
		return 0, ErrSchemaCompatibilityPreflight
	}
	return version, nil
}

func (s *Store) RuntimeSchemaCompatibility(ctx context.Context, binaryRevision string) (SchemaCompatibilityReport, error) {
	version, err := s.RuntimeSchemaVersion(ctx)
	if err != nil {
		return NewSchemaCompatibilityPreflightReport(binaryRevision), ErrSchemaCompatibilityPreflight
	}
	return EvaluateSchemaCompatibility(version, binaryRevision), nil
}

// SchemaCompatibility supports both the restricted runtime function and the
// direct migration table access available to an operator before migration 24.
func (s *Store) SchemaCompatibility(ctx context.Context, binaryRevision string) (SchemaCompatibilityReport, error) {
	if version, err := s.RuntimeSchemaVersion(ctx); err == nil {
		return EvaluateSchemaCompatibility(version, binaryRevision), nil
	}
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return NewSchemaCompatibilityPreflightReport(binaryRevision), ErrSchemaCompatibilityPreflight
	}
	return EvaluateSchemaCompatibility(version, binaryRevision), nil
}
