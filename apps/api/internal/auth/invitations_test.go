package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestInvitationToken(t *testing.T) {
	token, hash := NewInvitationToken()
	if !strings.HasPrefix(token, "sign_inv_") || len(token) != len("sign_inv_")+43 {
		t.Fatalf("token %q", token)
	}
	if !IsInvitationToken(token) {
		t.Fatal("a new token isn't one")
	}
	if !bytes.Equal(hash, HashInvitationToken(token)) {
		t.Fatal("the hash isn't the token's")
	}
	other, _ := NewInvitationToken()
	if other == token {
		t.Fatal("two tokens are equal")
	}
	for _, bad := range []string{"", "sign_inv_", "sign_test_" + token[9:], token + "x", token[:len(token)-1] + "-"} {
		if IsInvitationToken(bad) {
			t.Errorf("%q passed", bad)
		}
	}
	// An API key isn't an invitation's token.
	key, _, _ := NewKey("test")
	if IsInvitationToken(key) {
		t.Error("a key passed")
	}
}
