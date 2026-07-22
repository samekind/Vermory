package reality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndValidatePublicCase(t *testing.T) {
	c, err := LoadCase("../../reality/testdata/valid-public")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid case, got violations: %#v", got)
	}
}

func TestRejectsLocalCaseClaimingSealedEvidence(t *testing.T) {
	_, err := LoadCase("../../reality/testdata/invalid-local-sealed")
	if err == nil || !strings.Contains(err.Error(), "sealed evidence cannot be loaded from a readable local case") {
		t.Fatalf("expected sealed-evidence rejection, got %v", err)
	}
}

func TestRequiresExpectedAndForbiddenBehavior(t *testing.T) {
	c := loadValidCase(t)
	c.Manifest.Expectations.ForbiddenFacts = nil
	violations := ValidateCase(c)
	assertViolationCode(t, violations, "forbidden_facts_required")
}

func TestRejectsUnknownManifestFields(t *testing.T) {
	dir := copyCaseFixture(t, "../../reality/testdata/valid-public")
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"version": 1,`, `"version": 1, "unknown": true,`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = LoadCase(dir)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field rejection, got %v", err)
	}
}

func TestRejectsInvalidEventGraph(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Case)
		code string
	}{
		{
			name: "duplicate source",
			edit: func(c *Case) { c.Manifest.Sources = append(c.Manifest.Sources, c.Manifest.Sources[0]) },
			code: "duplicate_source_id",
		},
		{
			name: "duplicate event",
			edit: func(c *Case) {
				c.Events = append(c.Events, Event{ID: c.Events[0].ID, Sequence: 2, Actor: "user", Channel: "chat", SourceID: c.Events[0].SourceID, Content: "duplicate"})
			},
			code: "duplicate_event_id",
		},
		{
			name: "unknown event source",
			edit: func(c *Case) { c.Events[0].SourceID = "missing" },
			code: "event_source_unknown",
		},
		{
			name: "noncontiguous sequence",
			edit: func(c *Case) { c.Events[0].Sequence = 2 },
			code: "event_sequence_invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := loadValidCase(t)
			tt.edit(&c)
			assertViolationCode(t, ValidateCase(c), tt.code)
		})
	}
}

func TestRejectsInvalidFixtureEvidence(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Case)
		code string
	}{
		{
			name: "absolute path",
			edit: func(c *Case) { c.Manifest.Sources[0].FixturePath = "/tmp/source.md" },
			code: "fixture_path_invalid",
		},
		{
			name: "path traversal",
			edit: func(c *Case) { c.Manifest.Sources[0].FixturePath = "../source.md" },
			code: "fixture_path_invalid",
		},
		{
			name: "unauthorized source",
			edit: func(c *Case) { c.Manifest.Sources[0].Authorized = false },
			code: "source_not_authorized",
		},
		{
			name: "missing anonymization",
			edit: func(c *Case) { c.Manifest.Sources[0].Anonymization = "" },
			code: "source_anonymization_required",
		},
		{
			name: "hash mismatch",
			edit: func(c *Case) { c.Manifest.Sources[0].SHA256 = strings.Repeat("0", 64) },
			code: "fixture_hash_mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := loadValidCase(t)
			tt.edit(&c)
			assertViolationCode(t, ValidateCase(c), tt.code)
		})
	}
}

func TestRejectsFactDeclaredCurrentAndForbidden(t *testing.T) {
	c := loadValidCase(t)
	c.Manifest.Expectations.ForbiddenFacts = append(c.Manifest.Expectations.ForbiddenFacts, c.Manifest.Expectations.CurrentFacts[0])
	assertViolationCode(t, ValidateCase(c), "fact_expectation_conflict")
}

func TestO01OpenClawCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/O01-openclaw-home-maintenance")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid O01 case, got violations: %#v", got)
	}

	foundConversation := false
	for _, line := range c.Manifest.ContinuityLines {
		if line == LineConversation {
			foundConversation = true
			break
		}
	}
	if !foundConversation {
		t.Fatalf("O01 does not declare conversation continuity: %#v", c.Manifest.ContinuityLines)
	}
	requireStrings(t, c.Manifest.Pressures,
		"restart",
		"cross_channel_link",
		"correction",
		"deletion",
		"global_default_override",
		"link_reversal",
	)
	for _, anchor := range []string{
		"agent:main:home-maintenance-a",
		"agent:main:home-maintenance-b",
		"agent:main:unrelated-c",
	} {
		found := false
		for _, candidate := range c.Manifest.Anchors {
			if candidate.Value == anchor && !candidate.Ambiguous {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing unambiguous O01 anchor %q", anchor)
		}
	}
}

func TestF01ConversationFormationCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/F01-conversation-formation-loop")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid F01 case, got violations: %#v", got)
	}

	for _, expected := range []ContinuityLine{LineConversation, LineGlobalDefaults, LineBridge, LineSecurity} {
		found := false
		for _, line := range c.Manifest.ContinuityLines {
			if line == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("F01 does not declare continuity line %q: %#v", expected, c.Manifest.ContinuityLines)
		}
	}
	requireStrings(t, c.Manifest.Pressures,
		"real_client_observations",
		"bounded_formation_window",
		"exact_observation_evidence",
		"transient_noise_exclusion",
		"explicit_candidate_governance",
		"correction",
		"deletion",
		"input_manifest_drift",
		"active_snapshot_drift",
		"idempotent_replay",
		"cross_continuity_isolation",
		"global_default_non_promotion",
	)
	for _, anchor := range []string{
		"agent:main:formation-home-maintenance-a",
		"agent:main:formation-home-maintenance-b",
		"agent:main:formation-unrelated-c",
	} {
		found := false
		for _, candidate := range c.Manifest.Anchors {
			if candidate.Value == anchor && !candidate.Ambiguous {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing unambiguous F01 anchor %q", anchor)
		}
	}
	requireStrings(t, c.Manifest.Expectations.ForbiddenFacts,
		"The current appointment is Friday at 15:30.",
		"The temporary access code is CEDAR-4826.",
		"Rain and lunch chatter are governed memory.",
		"Assistant output is authoritative formation evidence.",
		"Session B or Session C raw observations enter Session A formation.",
		"A one-turn English request becomes a Global Default.",
		"A replay invokes the provider or duplicates candidates.",
	)
}

func TestH01HermesCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/H01-hermes-linked-sessions")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid H01 case, got violations: %#v", got)
	}

	for _, expected := range []ContinuityLine{LineConversation, LineBridge, LineSecurity} {
		found := false
		for _, line := range c.Manifest.ContinuityLines {
			if line == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("H01 does not declare continuity line %q: %#v", expected, c.Manifest.ContinuityLines)
		}
	}
	requireStrings(t, c.Manifest.Pressures,
		"real_hermes_cli",
		"domestic_openai_compatible_model",
		"resume_semantics",
		"built_in_memory_isolation",
		"explicit_link",
		"transcript_isolation",
		"unrelated_continuity_isolation",
		"link_reversal",
		"fail_open",
		"credential_hygiene",
	)
	for _, anchor := range []string{
		"session:<runtime-session-a>",
		"session:<runtime-session-b>",
		"session:<unrelated-session-c>",
	} {
		found := false
		for _, candidate := range c.Manifest.Anchors {
			if candidate.Value == anchor && !candidate.Ambiguous {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing unambiguous H01 anchor %q", anchor)
		}
	}
}

func TestW04CanonicalRepositoryCrossClientCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/W04-canonical-repository-cross-client")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid W04 case, got violations: %#v", got)
	}

	for _, expected := range []ContinuityLine{LineWorkspace, LineSecurity} {
		found := false
		for _, line := range c.Manifest.ContinuityLines {
			if line == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("W04 does not declare continuity line %q: %#v", expected, c.Manifest.ContinuityLines)
		}
	}
	requireStrings(t, c.Manifest.Pressures,
		"real_cursor_agent",
		"cross_client_continuity",
		"canonical_repository_migration",
		"source_revision",
		"same_name_workspace_isolation",
		"current_only_delivery",
		"proposed_only_writeback",
		"client_failure_retention",
	)
	requireStrings(t, c.Manifest.Expectations.CurrentFacts,
		"The canonical repository is https://github.com/samekind/Vermory.",
		"The current continuation marker is samekind-w25-current.",
	)
	requireStrings(t, c.Manifest.Expectations.ForbiddenFacts,
		"https://github.com/jstar0/Vermory is the current canonical repository.",
		"The canonical workspace continuation marker is distractor-w25-only.",
		"A coding client may activate, correct, delete, rebind, or change the tenant of its own write-back.",
	)
}

func TestI01AuthenticatedMultiTenantCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/I01-authenticated-multitenant-rls")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid I01 case, got violations: %#v", got)
	}

	for _, expected := range []ContinuityLine{LineConversation, LineGlobalDefaults, LineBridge, LineSecurity} {
		found := false
		for _, line := range c.Manifest.ContinuityLines {
			if line == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("I01 does not declare continuity line %q: %#v", expected, c.Manifest.ContinuityLines)
		}
	}
	requireStrings(t, c.Manifest.Pressures,
		"token_expiry",
		"token_revocation",
		"role_denial",
		"same_anchor_cross_tenant",
		"filter_omission",
		"cross_tenant_foreign_key",
		"pool_reuse",
		"authenticated_openclaw",
	)
	for _, anchor := range []string{
		"identity-a/openclaw/agent:main:shared-anchor",
		"identity-b/openclaw/agent:main:shared-anchor",
	} {
		found := false
		for _, candidate := range c.Manifest.Anchors {
			if candidate.Value == anchor && !candidate.Ambiguous {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing unambiguous I01 anchor %q", anchor)
		}
	}
}

func TestI02PostgreSQLOperationsRecoveryCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/I02-postgresql-operations-recovery")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid I02 case, got violations: %#v", got)
	}

	requireStrings(t, c.Manifest.Pressures,
		"migration_replay",
		"backup_restore",
		"projection_rebuild",
		"runtime_role_reprovision",
		"database_outage",
		"linux_amd64",
		"linux_arm64",
	)
	for _, source := range c.Manifest.Sources {
		data, err := os.ReadFile(filepath.Join("../../reality/cases/I02-postgresql-operations-recovery", source.FixturePath))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, forbidden := range []string{"vmt_", "sk-", "password=", "postgresql://"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("fixture %s contains credential-shaped value %q", source.FixturePath, forbidden)
			}
		}
	}
}

func TestI03PostgreSQLHAPITRCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/I03-postgresql-ha-pitr")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid I03 case, got violations: %#v", got)
	}

	requireStrings(t, c.Manifest.Pressures,
		"physical_streaming_replication",
		"primary_immediate_failure",
		"standby_promotion",
		"multi_host_runtime_reconnect",
		"wal_archive",
		"pitr_target_lsn",
		"historical_deletion_resurrection",
		"historical_token_resurrection",
		"post_recovery_projection_rebuild",
		"post_recovery_credential_governance",
	)
	for _, source := range c.Manifest.Sources {
		data, err := os.ReadFile(filepath.Join("../../reality/cases/I03-postgresql-ha-pitr", source.FixturePath))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(data))
		for _, forbidden := range []string{"vmt_", "sk-", "password=", "postgresql://", "/users/", "/volumes/"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("fixture %s contains forbidden value %q", source.FixturePath, forbidden)
			}
		}
	}
}

func TestI05DurableLinuxServiceLifecycleCaseIsFrozen(t *testing.T) {
	c, err := LoadCase("../../reality/cases/I05-durable-linux-service-lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	if got := ValidateCase(c); len(got) != 0 {
		t.Fatalf("expected valid I05 case, got violations: %#v", got)
	}

	requireStrings(t, c.Manifest.Pressures,
		"systemd_system_service",
		"versioned_release_install",
		"restricted_service_identity",
		"environment_file_secret_boundary",
		"in_place_upgrade",
		"failed_upgrade_auto_rollback",
		"manual_release_rollback",
		"native_backup",
		"empty_target_restore",
		"runtime_role_reprovision",
		"projection_rebuild",
		"restored_authenticated_probe",
		"artifact_checksum_verification",
		"credential_hygiene",
	)
	requireStrings(t, c.Manifest.Expectations.ForbiddenFacts,
		"A portable Linux ELF or successful CLI help command alone proves a durable service installation.",
		"Vermory deployment scripts invoke sudo internally or trigger repeated privilege prompts.",
		"A failed release remains the active current target after the installer reports rollback.",
		"Binary pointer rollback automatically reverses forward PostgreSQL migrations.",
		"Restore overwrites a non-empty target or accepts a digest mismatch.",
		"An ephemeral Ubuntu qualification is described as measured long-duration uptime or a cloud SLA.",
	)
}

func loadValidCase(t *testing.T) Case {
	t.Helper()
	c, err := LoadCase("../../reality/testdata/valid-public")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func assertViolationCode(t *testing.T, violations []Violation, code string) {
	t.Helper()
	for _, violation := range violations {
		if violation.Code == code {
			return
		}
	}
	t.Fatalf("expected violation %q, got %#v", code, violations)
}

func requireStrings(t *testing.T, values []string, required ...string) {
	t.Helper()
	for _, expected := range required {
		found := false
		for _, value := range values {
			if value == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in %#v", expected, values)
		}
	}
}

func copyCaseFixture(t *testing.T, source string) string {
	t.Helper()
	destination := t.TempDir()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return destination
}
