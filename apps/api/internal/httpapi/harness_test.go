package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/cache"
	"proxima.la/sello/sign/internal/store"
)

// testStore creates a database of its own in POSTGRES_TEST_URL's server,
// migrates it and drops it when the test ends. Without the variable, the
// test is skipped.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		t.Skip("POSTGRES_TEST_URL is unset")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "sign_test_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	if _, err := store.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// Migrating twice applies nothing.
	if again, err := store.Migrate(ctx, pool); err != nil || len(again) != 0 {
		t.Fatalf("second migration: %v %v", again, err)
	}
	return store.New(pool)
}

// fakeVerifier takes tokens that are a key of its map.
type fakeVerifier map[string]auth.Claims

func (f fakeVerifier) Verify(_ context.Context, token string) (auth.Claims, error) {
	c, ok := f[token]
	if !ok {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	return c, nil
}

type harness struct {
	t      *testing.T
	db     *store.Store
	server *Server
	http   *httptest.Server
	tokens fakeVerifier
}

func newHarness(t *testing.T, configure func(*Deps)) *harness {
	t.Helper()
	db := testStore(t)
	h := &harness{t: t, db: db, tokens: fakeVerifier{}}
	signer := NewSigner(testKeyring(t), cache.NewMaterial(100, time.Minute), testRegistry(), []string{"SV", "CR", "ZZ"}, db.ActiveMaterial)
	d := Deps{Store: db, Keys: cache.NewMemoryKeys(), Signer: signer, Verifier: h.tokens, CORSOrigins: []string{"http://sign.sello.test"}}
	if configure != nil {
		configure(&d)
	}
	h.server = New(d)
	h.http = httptest.NewServer(h.server.Handler())
	t.Cleanup(h.http.Close)
	return h
}

// person registers a token for a panel user and returns it.
func (h *harness) person(sub string, c auth.Claims) string {
	c.Subject = sub
	if c.Email == "" {
		c.Email = sub + "@example.com"
	}
	token := "token-" + sub
	h.tokens[token] = c
	return token
}

type response struct {
	status int
	body   []byte
	header http.Header
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("not JSON (%d): %s", r.status, r.body)
	}
	return m
}

func (r response) items(t *testing.T) []map[string]any {
	t.Helper()
	var l struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(r.body, &l); err != nil {
		t.Fatalf("not a list (%d): %s", r.status, r.body)
	}
	return l.Items
}

// as is who calls: "Bearer <token>" or a key ("sign_…").
func (h *harness) do(method, path, as string, body any, headers ...string) response {
	h.t.Helper()
	var r io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case string:
		r, ct = strings.NewReader(b), "application/json"
	case []byte:
		r, ct = bytes.NewReader(b), "application/json"
	default:
		j, _ := json.Marshal(b)
		r, ct = bytes.NewReader(j), "application/json"
	}
	req, err := http.NewRequest(method, h.http.URL+path, r)
	if err != nil {
		h.t.Fatal(err)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	switch {
	case strings.HasPrefix(as, "sign_"):
		req.Header.Set("X-API-Key", as)
	case as != "":
		req.Header.Set("Authorization", "Bearer "+as)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, body: b, header: res.Header}
}

// expect fails unless the response has status (and, for a problem, the
// type, if given).
func (h *harness) expect(r response, status int, problemType ...string) response {
	h.t.Helper()
	if r.status != status {
		h.t.Fatalf("status %d, want %d: %s", r.status, status, r.body)
	}
	if len(problemType) > 0 {
		var p Problem
		_ = json.Unmarshal(r.body, &p)
		if p.Type != problemType[0] {
			h.t.Fatalf("problem %q, want %q: %s", p.Type, problemType[0], r.body)
		}
		if ct := r.header.Get("Content-Type"); ct != "application/problem+json" {
			h.t.Fatalf("problem served as %q", ct)
		}
	}
	return r
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func storeOrg(ws, country string) store.NewOrganization {
	return store.NewOrganization{WorkspaceID: ws, Name: "Org " + country, TaxID: country + "-1", Country: country}
}

func monthNow() string { return time.Now().UTC().Format("2006-01") }
