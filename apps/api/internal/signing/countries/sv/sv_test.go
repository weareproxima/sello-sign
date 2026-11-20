package sv

import (
	"strings"
	"testing"

	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestVectors(t *testing.T) {
	// A JWS has no XMLDSig to check with helium; Verify is plain crypto/rsa.
	signingtest.Run(t, New(), "sv", signingtest.Expect{NoHelium: true})
}

func TestTamper(t *testing.T) {
	c := signingtest.Load(t, "sv").Case(t, "dte")
	jws := string(c.Output(t))
	parts := strings.Split(jws, ".")
	payload := []byte(parts[1])
	payload[10] ^= 1
	bad := parts[0] + "." + string(payload) + "." + parts[2]
	m := signingtest.Material(t)
	if v, _ := New().Verify(m, []byte(bad)); v.Valid {
		t.Error("Verify accepts a changed payload")
	}
	hs256 := "eyJhbGciOiJIUzI1NiJ9." + parts[1] + "." + parts[2]
	if v, _ := New().Verify(m, []byte(hs256)); v.Valid {
		t.Error("Verify accepts another alg")
	}
	if v, _ := New().Verify(m, []byte(jws)); !v.Valid || v.Document.ID != c.DocumentID || !strings.HasPrefix(string(v.Payload), `{"identificacion"`) {
		t.Errorf("Verify = %+v", v)
	}
}
