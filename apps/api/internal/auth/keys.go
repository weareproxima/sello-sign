// Package auth is who is calling: an API key (X-API-Key) or a person with
// the panel's token (Authorization: Bearer). It reads and checks
// credentials; what each one may do is the HTTP layer's, with model's
// rules.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"strings"

	"proxima.la/sello/sign/internal/model"
)

// KeyPrefix is how every key starts; its environment follows it:
// sign_test_… or sign_prod_….
const KeyPrefix = "sign_"

// keySecretLen is how many base62 characters follow the environment: 43
// of them are about 256 bits.
const keySecretLen = 43

// displayLen is how many secret characters the stored prefix keeps, to
// recognize a key in a list.
const displayLen = 8

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// NewKey makes a key for env. It returns the key, which is shown once, its
// SHA-256 (what's stored) and its display prefix.
func NewKey(env model.Environment) (key string, hash []byte, prefix string) {
	var b strings.Builder
	b.WriteString(KeyPrefix)
	b.WriteString(string(env))
	b.WriteByte('_')
	b.WriteString(randomBase62(keySecretLen))
	key = b.String()
	h := HashKey(key)
	return key, h, key[:len(KeyPrefix)+len(env)+1+displayLen]
}

// randomBase62 is n characters of base62 from crypto/rand.
func randomBase62(n int) string {
	var b strings.Builder
	max := big.NewInt(int64(len(base62)))
	for range n {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err)
		}
		b.WriteByte(base62[r.Int64()])
	}
	return b.String()
}

// HashKey is what the database keeps of a key.
func HashKey(key string) []byte {
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

// KeyEnvironment reads the environment of a key from its prefix; ok is
// false if it isn't a key's shape, which is refused without a lookup.
func KeyEnvironment(key string) (model.Environment, bool) {
	rest, ok := strings.CutPrefix(key, KeyPrefix)
	if !ok {
		return "", false
	}
	envPart, secret, ok := strings.Cut(rest, "_")
	if !ok || len(secret) != keySecretLen {
		return "", false
	}
	for i := 0; i < len(secret); i++ {
		if !strings.ContainsRune(base62, rune(secret[i])) {
			return "", false
		}
	}
	return model.ParseEnvironment(envPart)
}

// Key is what an API key is, as the cache and the database keep it.
type Key struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	// OrganizationID is set when the key is of a single organization.
	OrganizationID string            `json:"organization_id,omitempty"`
	Environment    model.Environment `json:"environment"`
	Scopes         []model.Scope     `json:"scopes"`
	// RequestsPerMinute is the key's own signing rate, on top of its
	// space's; nil is none.
	RequestsPerMinute *int `json:"requests_per_minute,omitempty"`
}

// User is a person, from the panel's token.
type User struct {
	Claims
}

// Principal is who is calling: exactly one of Key and User.
type Principal struct {
	Key  *Key
	User *User
}
