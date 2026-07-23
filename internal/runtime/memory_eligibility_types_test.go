package runtime

import (
	"context"
	"testing"
	"time"
)

func TestEffectiveMemoryState(t *testing.T) {
	asOf := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	before := asOf.Add(-time.Nanosecond)
	after := asOf.Add(time.Nanosecond)
	for name, test := range map[string]struct {
		lifecycle string
		content   string
		validity  MemoryValidity
		want      MemoryEffectiveState
	}{
		"unbounded current":                  {lifecycle: "active", content: "durable", want: MemoryEffectiveCurrent},
		"scheduled":                          {lifecycle: "active", content: "future", validity: MemoryValidity{ValidFrom: &after}, want: MemoryEffectiveScheduled},
		"valid from boundary":                {lifecycle: "active", content: "starts now", validity: MemoryValidity{ValidFrom: &asOf}, want: MemoryEffectiveCurrent},
		"before valid until":                 {lifecycle: "active", content: "still current", validity: MemoryValidity{ValidUntil: &asOf}, want: MemoryEffectiveExpired},
		"one nanosecond before valid until":  {lifecycle: "active", content: "still current", validity: MemoryValidity{ValidUntil: &asOf}, want: MemoryEffectiveExpired},
		"future interval current before end": {lifecycle: "active", content: "bounded", validity: MemoryValidity{ValidFrom: &before, ValidUntil: &after}, want: MemoryEffectiveCurrent},
		"archived":                           {lifecycle: "archived", content: "history", want: MemoryEffectiveArchived},
		"superseded":                         {lifecycle: "superseded", content: "old", want: MemoryEffectiveSuperseded},
		"rejected":                           {lifecycle: "rejected", content: "rejected", want: MemoryEffectiveRejected},
		"proposed":                           {lifecycle: "proposed", content: "draft", want: MemoryEffectiveProposed},
		"deleted lifecycle":                  {lifecycle: "deleted", content: "[redacted]", want: MemoryEffectiveDeleted},
		"redacted content":                   {lifecycle: "active", content: "[redacted]", want: MemoryEffectiveDeleted},
	} {
		t.Run(name, func(t *testing.T) {
			if got := EffectiveMemoryState(test.lifecycle, test.content, test.validity, asOf); got != test.want {
				t.Fatalf("state=%q want %q", got, test.want)
			}
		})
	}

	if got := EffectiveMemoryState("active", "before", MemoryValidity{ValidUntil: &after}, asOf); got != MemoryEffectiveCurrent {
		t.Fatalf("memory before valid_until state=%q want current", got)
	}
}

func TestMemoryValidityValidation(t *testing.T) {
	china := time.FixedZone("CST", 8*60*60)
	from := time.Date(2026, 7, 20, 14, 0, 0, 0, china)
	until := from.Add(time.Hour)
	normalized, err := (MemoryValidity{ValidFrom: &from, ValidUntil: &until}).Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ValidFrom == nil || normalized.ValidUntil == nil ||
		normalized.ValidFrom.Location() != time.UTC || normalized.ValidUntil.Location() != time.UTC {
		t.Fatalf("validity was not normalized to UTC: %#v", normalized)
	}
	if !normalized.ValidFrom.Equal(time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("normalized valid_from=%s", normalized.ValidFrom)
	}
	for name, validity := range map[string]MemoryValidity{
		"equal":   {ValidFrom: &from, ValidUntil: &from},
		"reverse": {ValidFrom: &until, ValidUntil: &from},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validity.Normalized(); err == nil {
				t.Fatal("invalid interval was accepted")
			}
		})
	}
}

func TestCurrentEligibilitySnapshot(t *testing.T) {
	store := openTestStore(t)
	snapshot, err := store.CurrentEligibilitySnapshot(context.Background(), "eligibility-clock-tenant")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AsOf.IsZero() || snapshot.AsOf.Location() != time.UTC {
		t.Fatalf("invalid eligibility snapshot: %#v", snapshot)
	}
	if _, err := store.CurrentEligibilitySnapshot(context.Background(), " "); err == nil {
		t.Fatal("blank tenant was accepted")
	}
}
