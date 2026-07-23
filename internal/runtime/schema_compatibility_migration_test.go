package runtime

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestSchemaCompatibilityMigrationExposesOnlyBoundedFunction(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != MaximumSupportedSchemaVersion {
		t.Fatalf("schema version mismatch: got %d want %d", version, MaximumSupportedSchemaVersion)
	}

	var functionVersion int64
	if err := store.pool.QueryRow(ctx, `SELECT vermory_auth.schema_version()`).Scan(&functionVersion); err != nil {
		t.Fatal(err)
	}
	if functionVersion != version {
		t.Fatalf("restricted function returned %d, want %d", functionVersion, version)
	}

	var publicCanExecute bool
	if err := store.pool.QueryRow(ctx, `
SELECT has_function_privilege('public', 'vermory_auth.schema_version()', 'EXECUTE')`).Scan(&publicCanExecute); err != nil {
		t.Fatal(err)
	}
	if publicCanExecute {
		t.Fatal("PUBLIC can execute the restricted schema version function")
	}
}

func TestSchemaCompatibilityDistinguishesRealOldAndFutureDatabaseStates(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	db := openProjectionRetentionMigrationDB(t)

	if err := goose.DownToContext(ctx, db, "migrations", MinimumSupportedSchemaVersion-1); err != nil {
		t.Fatal(err)
	}
	oldReport, oldErr := store.SchemaCompatibility(ctx, "compatibility-test")
	if oldErr != nil || oldReport.Status != SchemaCompatibilityMigrationRequired || oldReport.SchemaVersion != MinimumSupportedSchemaVersion-1 {
		t.Fatalf("old schema compatibility mismatch: report=%#v err=%v", oldReport, oldErr)
	}
	runtimeOldReport, err := store.RuntimeSchemaCompatibility(ctx, "compatibility-test")
	if err != nil || runtimeOldReport.Status != SchemaCompatibilityMigrationRequired || runtimeOldReport.SchemaVersion != MinimumSupportedSchemaVersion-1 {
		t.Fatalf("restricted old schema compatibility mismatch: report=%#v err=%v", runtimeOldReport, err)
	}
	if err := runtimeOldReport.ErrorIfIncompatible(); err == nil {
		t.Fatal("restricted runtime compatibility accepted an old schema")
	}
	if err := goose.UpToContext(ctx, db, "migrations", MaximumSupportedSchemaVersion); err != nil {
		t.Fatal(err)
	}

	futureVersion := MaximumSupportedSchemaVersion + 1
	if _, err := store.pool.Exec(ctx, `DELETE FROM goose_db_version WHERE version_id = $1`, futureVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO goose_db_version (version_id, is_applied, tstamp)
VALUES ($1, true, now())`, futureVersion); err != nil {
		t.Fatal(err)
	}
	defer store.pool.Exec(context.Background(), `DELETE FROM goose_db_version WHERE version_id = $1`, futureVersion)
	futureReport, futureErr := store.RuntimeSchemaCompatibility(ctx, "compatibility-test")
	if futureErr != nil || futureReport.Status != SchemaCompatibilityBinaryTooOld || futureReport.SchemaVersion != futureVersion {
		t.Fatalf("future schema compatibility mismatch: report=%#v err=%v", futureReport, futureErr)
	}
	if err := futureReport.ErrorIfIncompatible(); err == nil {
		t.Fatal("future schema did not return typed incompatibility")
	}
}
