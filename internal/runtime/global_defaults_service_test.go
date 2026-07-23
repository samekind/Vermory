package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestGlobalDefaultsServiceCompletesExplicitLifecycle(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewGlobalDefaultsService(store, "local")

	created, err := service.Set(ctx, SetGlobalDefaultRequest{
		OperationID: "service-default-set",
		Key:         "reply_language",
		Content:     "Default user-facing replies to Chinese unless the active task explicitly requests another language.",
	})
	requireNoError(t, err)
	if created.ContinuityID == "" || created.MemoryID == "" || created.MemoryStatus != "active" || created.Replayed {
		t.Fatalf("unexpected set receipt: %#v", created)
	}

	replay, err := service.Set(ctx, SetGlobalDefaultRequest{
		OperationID: "service-default-set",
		Key:         "reply_language",
		Content:     "Default user-facing replies to Chinese unless the active task explicitly requests another language.",
	})
	requireNoError(t, err)
	if !replay.Replayed || replay.MemoryID != created.MemoryID || replay.ContinuityID != created.ContinuityID {
		t.Fatalf("set replay changed the receipt: first=%#v replay=%#v", created, replay)
	}

	inspection, err := service.Inspect(ctx)
	requireNoError(t, err)
	if inspection.ContinuityID != created.ContinuityID || len(inspection.Defaults) != 1 {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if inspection.Defaults[0].MemoryKey != "reply_language" || inspection.Defaults[0].LifecycleStatus != "active" {
		t.Fatalf("default was not inspectable: %#v", inspection.Defaults[0])
	}

	corrected, err := service.Correct(ctx, CorrectGlobalDefaultRequest{
		OperationID: "service-default-correct",
		MemoryID:    created.MemoryID,
		Content:     "Default user-facing replies to Chinese.",
	})
	requireNoError(t, err)
	if corrected.MemoryStatus != "active" || corrected.MemoryID == created.MemoryID {
		t.Fatalf("unexpected correction receipt: %#v", corrected)
	}

	forgotten, err := service.Forget(ctx, ForgetGlobalDefaultRequest{
		OperationID: "service-default-forget",
		MemoryID:    corrected.MemoryID,
	})
	requireNoError(t, err)
	if forgotten.MemoryStatus != "deleted" || forgotten.MemoryID != corrected.MemoryID {
		t.Fatalf("unexpected forget receipt: %#v", forgotten)
	}

	inspection, err = service.Inspect(ctx)
	requireNoError(t, err)
	if len(inspection.Defaults) != 2 || inspection.Defaults[0].LifecycleStatus != "superseded" || inspection.Defaults[1].LifecycleStatus != "deleted" {
		t.Fatalf("lifecycle history was not retained: %#v", inspection.Defaults)
	}
}

func TestGlobalDefaultsServiceRejectsInvalidOrImplicitMutations(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewGlobalDefaultsService(store, "local")

	for _, key := range []string{"", "ReplyLanguage", "reply-language", "reply language", strings.Repeat("a", 65)} {
		_, err := service.Set(ctx, SetGlobalDefaultRequest{
			OperationID: "invalid-key-" + key,
			Key:         key,
			Content:     "Must not persist.",
		})
		if err == nil {
			t.Fatalf("invalid key %q was accepted", key)
		}
	}

	created, err := service.Set(ctx, SetGlobalDefaultRequest{
		OperationID: "explicit-only-set",
		Key:         "reply_language",
		Content:     "Default replies to Chinese.",
	})
	requireNoError(t, err)

	_, err = NewGlobalDefaultsService(store, "other").Correct(ctx, CorrectGlobalDefaultRequest{
		OperationID: "cross-tenant-default-correction",
		MemoryID:    created.MemoryID,
		Content:     "Must not cross tenant.",
	})
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cross-tenant mutation was accepted: %v", err)
	}

	if _, err := NewGlobalDefaultsService(nil, "local").Inspect(ctx); err == nil {
		t.Fatal("unconfigured service was accepted")
	}
	if _, err := NewGlobalDefaultsService(store, "").Inspect(ctx); err == nil {
		t.Fatal("service without server-owned tenant was accepted")
	}
}
