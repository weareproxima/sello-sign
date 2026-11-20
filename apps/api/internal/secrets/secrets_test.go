package secrets

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func newKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, KeySize)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestSealOpen(t *testing.T) {
	k, err := NewKeyring(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("-----BEGIN PRIVATE KEY-----")
	sealed, err := k.Seal(plain, []byte("cert-1"))
	if err != nil {
		t.Fatal(err)
	}
	if sealed[0] != 1 {
		t.Fatalf("version byte = %d, want 1", sealed[0])
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("the sealed value contains the plaintext")
	}
	got, err := k.Open(sealed, []byte("cert-1"))
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, err := k.Open(sealed, []byte("cert-2")); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("another row's ad opened it: %v", err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := k.Open(sealed, []byte("cert-1")); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("a changed value opened: %v", err)
	}
}

func TestRotation(t *testing.T) {
	oldKey, newKeyS := newKey(t), "v2:"+newKey(t)
	old, err := NewKeyring(oldKey)
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := old.Seal([]byte("x"), nil)
	rotated, err := NewKeyring(newKeyS, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Current() != 2 {
		t.Fatalf("current = %d", rotated.Current())
	}
	if got, err := rotated.Open(sealed, nil); err != nil || string(got) != "x" {
		t.Fatalf("the previous key doesn't open: %q %v", got, err)
	}
	again, _ := rotated.Seal([]byte("y"), nil)
	if again[0] != 2 {
		t.Fatalf("sealed with version %d", again[0])
	}
	if _, err := old.Open(again, nil); !errors.Is(err, ErrUnknownVersion) {
		t.Fatalf("old keyring opened a v2 value: %v", err)
	}
}

func TestParseKey(t *testing.T) {
	if _, _, err := ParseKey("not base64!"); err == nil {
		t.Fatal("accepted garbage")
	}
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, _, err := ParseKey(short); err == nil {
		t.Fatal("accepted a short key")
	}
	k := newKey(t)
	_, _, err := ParseKey("v0:" + k)
	if err == nil {
		t.Fatal("accepted version 0")
	}
	_, _, err = ParseKey(k + "x")
	if err != nil && strings.Contains(err.Error(), k) {
		t.Fatal("the error echoes the key")
	}
}
