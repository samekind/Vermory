package authn

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokenGenerationUsesInjectedEntropyAndSafeRepresentations(t *testing.T) {
	entropy := bytes.NewReader(bytes.Repeat([]byte{0x42}, publicIDBytes+secretBytes))
	token, err := GenerateToken(entropy)
	if err != nil {
		t.Fatal(err)
	}
	raw := token.Reveal()
	if !strings.HasPrefix(raw, tokenPrefix) || strings.Count(raw, "_") != 2 {
		t.Fatalf("unexpected token format %q", raw)
	}
	secret := strings.Split(raw, "_")[2]
	parsed, err := ParseToken(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PublicID != token.PublicID() || parsed.Digest != token.Digest() {
		t.Fatalf("parse mismatch: parsed=%#v token_public_id=%q", parsed, token.PublicID())
	}
	if strings.Contains(token.String(), raw) || strings.Contains(token.String(), secret) {
		t.Fatalf("String leaked token material: %q", token.String())
	}
	encoded, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(raw)) || bytes.Contains(encoded, []byte(secret)) {
		t.Fatalf("JSON leaked token material: %s", encoded)
	}
}

func TestTokenDigestIsDeterministicAndSecretSensitive(t *testing.T) {
	firstToken, err := GenerateToken(bytes.NewReader(bytes.Repeat([]byte{0x31}, publicIDBytes+secretBytes)))
	if err != nil {
		t.Fatal(err)
	}
	first, err := ParseToken(firstToken.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseToken(firstToken.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	differentEntropy := bytes.Repeat([]byte{0x31}, publicIDBytes+secretBytes)
	differentEntropy[len(differentEntropy)-1] = 0x32
	differentToken, err := GenerateToken(bytes.NewReader(differentEntropy))
	if err != nil {
		t.Fatal(err)
	}
	different, err := ParseToken(differentToken.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatal("same token produced different digests")
	}
	if first.Digest == different.Digest {
		t.Fatal("different secrets produced the same digest")
	}
}

func TestTokenParsingRejectsMalformedOrUnboundedInputWithoutEcho(t *testing.T) {
	inputs := []string{
		"",
		"bearer secret",
		"vmt_only-two-parts",
		"vmt_bad!id_c2VjcmV0",
		"vmt_public12345_not+base64",
		"vmt_" + strings.Repeat("a", maxPublicIDLength+1) + "_c2VjcmV0",
		"vmt_public12345_" + strings.Repeat("a", maxSecretLength+1),
		strings.Repeat("x", maxTokenLength+1),
	}
	for _, input := range inputs {
		_, err := ParseToken(input)
		if !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("ParseToken(%q) error=%v", input, err)
		}
		if input != "" && strings.Contains(err.Error(), input) {
			t.Fatalf("error echoed token input: %v", err)
		}
	}
}

func TestIssueTokenRequestValidationRejectsUnsafeIdentityAndExpiry(t *testing.T) {
	now := time.Date(2026, 7, 14, 5, 0, 0, 0, time.UTC)
	valid := IssueTokenRequest{
		OperationID: "issue-a-1",
		TenantID:    "identity-a",
		SubjectID:   "alice-client",
		Role:        RoleClient,
		ExpiresAt:   now.Add(time.Hour),
	}
	if err := valid.Validate(now); err != nil {
		t.Fatal(err)
	}
	tests := []IssueTokenRequest{
		{TenantID: valid.TenantID, SubjectID: valid.SubjectID, Role: valid.Role, ExpiresAt: valid.ExpiresAt},
		{OperationID: valid.OperationID, SubjectID: valid.SubjectID, Role: valid.Role, ExpiresAt: valid.ExpiresAt},
		{OperationID: valid.OperationID, TenantID: valid.TenantID, Role: valid.Role, ExpiresAt: valid.ExpiresAt},
		{OperationID: valid.OperationID, TenantID: valid.TenantID, SubjectID: valid.SubjectID, Role: Role("admin"), ExpiresAt: valid.ExpiresAt},
		{OperationID: valid.OperationID, TenantID: valid.TenantID, SubjectID: valid.SubjectID, Role: valid.Role, ExpiresAt: now},
	}
	for _, request := range tests {
		if err := request.Validate(now); !errors.Is(err, ErrInvalidIdentityRequest) {
			t.Fatalf("request=%#v error=%v", request, err)
		}
	}
}
