package authn

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	tokenPrefix       = "vmt_"
	publicIDBytes     = 12
	secretBytes       = 32
	maxPublicIDLength = 64
	maxSecretLength   = 64
	maxTokenLength    = len(tokenPrefix) + maxPublicIDLength + 1 + maxSecretLength
)

var publicIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

type RawToken struct {
	publicID string
	secret   string
}

type ParsedToken struct {
	PublicID string
	Digest   [sha256.Size]byte
}

func NewToken() (RawToken, error) {
	return GenerateToken(rand.Reader)
}

func GenerateToken(entropy io.Reader) (RawToken, error) {
	if entropy == nil {
		return RawToken{}, errorsWithoutMaterial("entropy source is required")
	}
	material := make([]byte, publicIDBytes+secretBytes)
	if _, err := io.ReadFull(entropy, material); err != nil {
		return RawToken{}, errorsWithoutMaterial("entropy source failed")
	}
	return RawToken{
		publicID: hex.EncodeToString(material[:publicIDBytes]),
		secret:   base64.RawURLEncoding.EncodeToString(material[publicIDBytes:]),
	}, nil
}

func ParseToken(raw string) (ParsedToken, error) {
	if len(raw) == 0 || len(raw) > maxTokenLength || !strings.HasPrefix(raw, tokenPrefix) {
		return ParsedToken{}, ErrInvalidToken
	}
	parts := strings.SplitN(raw[len(tokenPrefix):], "_", 2)
	if len(parts) != 2 || !validPublicID(parts[0]) || len(parts[1]) == 0 || len(parts[1]) > maxSecretLength {
		return ParsedToken{}, ErrInvalidToken
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(secret) != secretBytes {
		return ParsedToken{}, ErrInvalidToken
	}
	return ParsedToken{PublicID: parts[0], Digest: sha256.Sum256(secret)}, nil
}

func (token RawToken) Reveal() string {
	if token.publicID == "" || token.secret == "" {
		return ""
	}
	return tokenPrefix + token.publicID + "_" + token.secret
}

func (token RawToken) PublicID() string {
	return token.publicID
}

func (token RawToken) Digest() [sha256.Size]byte {
	secret, err := base64.RawURLEncoding.DecodeString(token.secret)
	if err != nil {
		return [sha256.Size]byte{}
	}
	return sha256.Sum256(secret)
}

func (token RawToken) String() string {
	if token.publicID == "" {
		return "[empty API token]"
	}
	return fmt.Sprintf("[redacted API token %s]", token.publicID)
}

func (token RawToken) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PublicID string `json:"public_id,omitempty"`
	}{PublicID: token.publicID})
}

func validPublicID(value string) bool {
	return publicIDPattern.MatchString(value)
}

func errorsWithoutMaterial(message string) error {
	return fmt.Errorf("generate API token: %s", message)
}
