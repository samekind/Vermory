package main

import (
	"context"
	"fmt"

	"vermory/internal/brand"
	"vermory/internal/runtime"
)

func openStandaloneRuntimeStore(ctx context.Context, databaseURL, runtimeName string) (*runtime.Store, error) {
	store, err := runtime.OpenStoreWithOptions(
		ctx,
		databaseURL,
		runtime.StoreOptions{EnforceTenantContext: true},
	)
	if err != nil {
		return nil, fmt.Errorf("open %s runtime store", runtimeName)
	}
	compatibility, err := store.RuntimeSchemaCompatibility(ctx, brand.Revision)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("%s runtime schema compatibility preflight: %w", runtimeName, err)
	}
	if err := compatibility.ErrorIfIncompatible(); err != nil {
		store.Close()
		return nil, fmt.Errorf("%s runtime schema compatibility preflight: %w", runtimeName, err)
	}
	if err := store.ValidateRuntimeRole(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("%s runtime role validation: %w", runtimeName, err)
	}
	return store, nil
}
