package packet

import (
	"strings"
	"testing"

	"vermory/internal/domain"
)

func TestBuildPacketRedactsAndAdaptsProfile(t *testing.T) {
	secret := "sk-" + strings.Repeat("a", 20)
	claims := []domain.Claim{
		{ID: "c1", Type: domain.ClaimTypeGoal, Content: "ContextMesh 面向个人与小团队。", Status: domain.ClaimStatusConfirmed, VerifiedByUser: true},
		{ID: "c2", Type: domain.ClaimTypeConstraint, Content: "不要泄露 " + secret + "。", Status: domain.ClaimStatusConfirmed, VerifiedByUser: true},
	}
	packet := Build(ProfileTeamHandoff, "ContextMesh user@example.com", "交接当前项目状态", claims)
	if !strings.Contains(packet.Body, "团队交接") {
		t.Fatal("expected team handoff wording")
	}
	if strings.Contains(packet.Body, secret) || strings.Contains(packet.Title, "user@example.com") {
		t.Fatal("expected packet output to be redacted")
	}
	if packet.RedactionCount != 3 {
		t.Fatalf("expected 3 redactions, got %d", packet.RedactionCount)
	}
}

func TestBuildPacketIncludesOnlyVerifiedActiveClaims(t *testing.T) {
	claims := []domain.Claim{
		{ID: "confirmed", Type: domain.ClaimTypeFact, Content: "confirmed context", Status: domain.ClaimStatusConfirmed, VerifiedByUser: true},
		{ID: "active", Type: domain.ClaimTypeDecision, Content: "active context", Status: domain.ClaimStatusActive, VerifiedByUser: true},
		{ID: "draft", Type: domain.ClaimTypeRisk, Content: "draft context", Status: domain.ClaimStatusDraft, VerifiedByUser: true},
		{ID: "unverified", Type: domain.ClaimTypeGoal, Content: "unverified context", Status: domain.ClaimStatusConfirmed, VerifiedByUser: false},
		{ID: "archived", Type: domain.ClaimTypeFact, Content: "archived context", Status: domain.ClaimStatusArchived, VerifiedByUser: true},
	}

	packet := Build(ProfileCodingAgent, "ContextMesh", "继续开发", claims)

	for _, expected := range []string{"confirmed context", "active context"} {
		if !strings.Contains(packet.Body, expected) {
			t.Fatalf("expected packet body to contain %q", expected)
		}
	}
	for _, unexpected := range []string{"draft context", "unverified context", "archived context"} {
		if strings.Contains(packet.Body, unexpected) {
			t.Fatalf("expected packet body to exclude %q", unexpected)
		}
	}
}

func TestCodingAgentPacketOmitsRetiredGeminiCLI(t *testing.T) {
	packet := Build(ProfileCodingAgent, "Vermory", "继续开发", nil)
	if strings.Contains(packet.Body, "Gemini CLI") {
		t.Fatalf("coding-agent packet must not advertise retired Gemini CLI: %q", packet.Body)
	}
	if !strings.Contains(packet.Body, "Grok") {
		t.Fatalf("coding-agent packet must list Grok as a supported current target: %q", packet.Body)
	}
}
