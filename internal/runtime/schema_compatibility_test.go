package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestEvaluateSchemaCompatibilityUsesDeterministicSupportInterval(t *testing.T) {
	tests := []struct {
		name              string
		schemaVersion     int64
		status            SchemaCompatibilityStatus
		migrationRequired bool
		compatible        bool
	}{
		{name: "older", schemaVersion: MinimumSupportedSchemaVersion - 1, status: SchemaCompatibilityMigrationRequired, migrationRequired: true},
		{name: "minimum", schemaVersion: MinimumSupportedSchemaVersion, status: SchemaCompatibilityCompatible, compatible: true},
		{name: "maximum", schemaVersion: MaximumSupportedSchemaVersion, status: SchemaCompatibilityCompatible, compatible: true},
		{name: "future", schemaVersion: MaximumSupportedSchemaVersion + 1, status: SchemaCompatibilityBinaryTooOld},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := EvaluateSchemaCompatibility(test.schemaVersion, "revision-test")
			if report.Status != test.status || report.SchemaVersion != test.schemaVersion ||
				report.MinimumSupportedSchema != MinimumSupportedSchemaVersion ||
				report.MaximumSupportedSchema != MaximumSupportedSchemaVersion ||
				report.BinaryRevision != "revision-test" ||
				report.MigrationRequired != test.migrationRequired || report.Compatible() != test.compatible {
				t.Fatalf("unexpected compatibility report: %#v", report)
			}
		})
	}
}

func TestStoreReportsCurrentAdminAndRestrictedSchemaCompatibility(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	adminReport, err := store.SchemaCompatibility(ctx, "admin-test")
	if err != nil {
		t.Fatal(err)
	}
	if !adminReport.Compatible() || adminReport.SchemaVersion != MaximumSupportedSchemaVersion {
		t.Fatalf("unexpected admin compatibility report: %#v", adminReport)
	}

	runtimeReport, err := store.RuntimeSchemaCompatibility(ctx, "runtime-test")
	if err != nil {
		t.Fatal(err)
	}
	if !runtimeReport.Compatible() || runtimeReport.SchemaVersion != MaximumSupportedSchemaVersion {
		t.Fatalf("unexpected runtime compatibility report: %#v", runtimeReport)
	}
}

func TestSchemaCompatibilityReturnsTypedIncompatibility(t *testing.T) {
	for _, schemaVersion := range []int64{MinimumSupportedSchemaVersion - 1, MaximumSupportedSchemaVersion + 1} {
		report := EvaluateSchemaCompatibility(schemaVersion, "revision-test")
		err := report.ErrorIfIncompatible()
		if !errors.Is(err, ErrIncompatibleSchema) {
			t.Fatalf("schema %d did not return typed incompatibility: %v", schemaVersion, err)
		}
		var compatibilityError *SchemaCompatibilityError
		if !errors.As(err, &compatibilityError) || compatibilityError.Report() != report {
			t.Fatalf("schema %d did not preserve its report: %#v %v", schemaVersion, compatibilityError, err)
		}
	}
}
