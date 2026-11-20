package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/model"
)

func TestNewKey(t *testing.T) {
	key, hash, prefix := NewKey(model.Test)
	if !strings.HasPrefix(key, "sign_test_") || len(key) != len("sign_test_")+keySecretLen {
		t.Fatalf("key = %q", key)
	}
	if !strings.HasPrefix(key, prefix) || len(prefix) != len("sign_test_")+displayLen {
		t.Fatalf("prefix = %q", prefix)
	}
	if string(hash) != string(HashKey(key)) {
		t.Fatal("hash mismatch")
	}
	env, ok := KeyEnvironment(key)
	if !ok || env != model.Test {
		t.Fatalf("KeyEnvironment = %q %v", env, ok)
	}
	other, _, _ := NewKey(model.Prod)
	if env, ok := KeyEnvironment(other); !ok || env != model.Prod {
		t.Fatalf("prod: %q %v", env, ok)
	}
	for _, bad := range []string{"", "sign_dev_" + key[10:], "sello_test_" + key[10:], key[:len(key)-1], key[:len(key)-1] + "-"} {
		if _, ok := KeyEnvironment(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

// panel is a fake panel: its JWKS and a signer for its tokens.
type panel struct {
	priv    ed25519.PrivateKey
	kid     string
	fetches atomic.Int32
	srv     *httptest.Server
}

func newPanel(t *testing.T) *panel {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := &panel{priv: priv, kid: "k1"}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.fetches.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "OKP", "crv": "Ed25519", "kid": p.kid, "x": base64.RawURLEncoding.EncodeToString(pub),
		}}})
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *panel) sign(t *testing.T, header, payload map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(header)
	b, _ := json.Marshal(payload)
	signing := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(b)
	return signing + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(p.priv, []byte(signing)))
}

func TestJWKSVerifier(t *testing.T) {
	p := newPanel(t)
	const iss, aud = "http://sign.sello.test", "http://api.sign.sello.test"
	now := time.Unix(1_800_000_000, 0)
	v := NewJWKSVerifierFrom(iss, aud, p.srv.URL).WithClock(func() time.Time { return now })
	header := map[string]any{"alg": "EdDSA", "kid": "k1", "typ": "JWT"}
	good := map[string]any{
		"iss": iss, "aud": aud, "sub": "user-1", "exp": now.Add(15 * time.Minute).Unix(),
		"email": "a@example.com", "name": "Ana", "sello_sub": "sello-1",
		"sello_workspaces": []map[string]string{{"id": "w1", "name": "Empresa", "country": "SV", "role": "admin"}},
	}
	c, err := v.Verify(context.Background(), p.sign(t, header, good))
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "user-1" || c.SelloSub != "sello-1" || len(c.SelloWorkspaces) != 1 || c.SelloWorkspaces[0].Role != "admin" {
		t.Fatalf("claims = %+v", c)
	}

	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range good {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	cases := []struct {
		name    string
		header  map[string]any
		payload map[string]any
		reason  string
	}{
		{"issuer", header, with("iss", "http://evil"), "issuer"},
		{"audience", header, with("aud", "http://other"), "audience"},
		{"audience list", header, with("aud", []string{"a", "b"}), "audience"},
		{"expired", header, with("exp", now.Add(-time.Minute).Unix()), "expired"},
		{"no sub", header, with("sub", ""), "malformed"},
		{"hs256", map[string]any{"alg": "HS256", "kid": "k1"}, good, "malformed"},
		{"access token", map[string]any{"alg": "EdDSA", "kid": "k1", "typ": "at+jwt"}, good, "type"},
	}
	for _, tc := range cases {
		_, err := v.Verify(context.Background(), p.sign(t, tc.header, tc.payload))
		if got := RefusalReason(err); got != tc.reason {
			t.Errorf("%s: reason %q (%v), want %q", tc.name, got, err, tc.reason)
		}
	}
	if _, err := v.Verify(context.Background(), p.sign(t, header, with("aud", []string{"x", aud}))); err != nil {
		t.Errorf("an audience list that names the API: %v", err)
	}
	// A forged signature.
	tok := p.sign(t, header, good)
	forged := tok[:strings.LastIndex(tok, ".")+1] + base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	if _, err := v.Verify(context.Background(), forged); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("forged: %v", err)
	}
	// An unknown kid fetches again, at most once a minute.
	before := p.fetches.Load()
	unknown := map[string]any{"alg": "EdDSA", "kid": "k2"}
	for range 3 {
		if _, err := v.Verify(context.Background(), p.sign(t, unknown, good)); RefusalReason(err) != "signature" {
			t.Fatalf("unknown kid: %v", err)
		}
	}
	if got := p.fetches.Load() - before; got != 0 {
		t.Errorf("unknown kid within the minute fetched %d times", got)
	}
	now = now.Add(2 * time.Minute)
	p.kid = "k2"
	good["exp"] = now.Add(time.Minute).Unix()
	if _, err := v.Verify(context.Background(), p.sign(t, unknown, good)); err != nil {
		t.Errorf("rotated key: %v", err)
	}
}
