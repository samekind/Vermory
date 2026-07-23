package redaction

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	secret := "sk-" + strings.Repeat("a", 20)
	input := "key " + secret + " email user@example.com phone 13800138000"
	result := Redact(input)
	if result.Text == input {
		t.Fatal("expected text to be redacted")
	}
	if result.Count != 3 {
		t.Fatalf("expected 3 redactions, got %d", result.Count)
	}
	if ContainsSensitive(result.Text) {
		t.Fatal("redacted text still contains sensitive content")
	}
}

func TestContainsSensitiveRejectsCredentialAssignmentsAndPrivateKeys(t *testing.T) {
	for _, input := range []string{
		"api_token=W23_SYNTHETIC_SECRET_MUST_NOT_PERSIST",
		"api_key: synthetic-value-123456",
		"secret = synthetic-value-123456",
		"token=synthetic-value-123456",
		"-----BEGIN PRIVATE KEY-----",
	} {
		if !ContainsSensitive(input) {
			t.Fatalf("credential-shaped input was not detected: %q", input)
		}
	}
}
