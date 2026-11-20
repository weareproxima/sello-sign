package auth

import (
	"crypto/sha256"
	"strings"
)

// InvitationPrefix is how every invitation token starts: sign_inv_….
const InvitationPrefix = "sign_inv_"

// NewInvitationToken makes the token of an invitation, which goes in the
// link of its e-mail and is shown once, and its SHA-256, what's stored.
func NewInvitationToken() (token string, hash []byte) {
	token = InvitationPrefix + randomBase62(keySecretLen)
	return token, HashInvitationToken(token)
}

// HashInvitationToken is what the database keeps of an invitation's token.
func HashInvitationToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// IsInvitationToken says whether token has an invitation token's shape;
// anything else is refused without a lookup.
func IsInvitationToken(token string) bool {
	secret, ok := strings.CutPrefix(token, InvitationPrefix)
	if !ok || len(secret) != keySecretLen {
		return false
	}
	for i := 0; i < len(secret); i++ {
		if !strings.ContainsRune(base62, rune(secret[i])) {
			return false
		}
	}
	return true
}
