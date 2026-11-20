package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/cache"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/secrets"
	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/store"
)

func testKeyring(t *testing.T) *secrets.Keyring {
	t.Helper()
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	k, err := secrets.NewKeyring(base64.StdEncoding.EncodeToString(b))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSignerWithAFakeCountry(t *testing.T) {
	ctx := context.Background()
	keyring := testKeyring(t)
	material := cache.NewMaterial(10, time.Minute)

	var (
		cert   store.Certificate
		sealed []byte
		loads  int
	)
	load := func(_ context.Context, org string, env model.Environment) (store.Certificate, []byte, error) {
		loads++
		if sealed == nil || env != model.Test {
			return store.Certificate{}, nil, store.ErrNotFound
		}
		return cert, sealed, nil
	}
	s := NewSigner(keyring, material, testRegistry(), []string{"sv", "ZZ"}, load)
	org := store.Organization{ID: ids.New(), Country: "SV", TaxID: "0614-1"}

	if _, err := s.Sign(ctx, org, model.Test, []byte(`{"id":"1"}`), "application/json"); !errors.Is(err, ErrNoActiveCertificate) {
		t.Fatalf("without a certificate: %v", err)
	}

	country, err := s.Country("SV")
	if err != nil {
		t.Fatal(err)
	}
	m, info, err := country.ParseCertificate(signing.Upload{Data: []byte("fake:0614-1")})
	if err != nil {
		t.Fatal(err)
	}
	cert = store.Certificate{ID: ids.New(), NotAfter: &info.NotAfter}
	if sealed, err = s.Seal(cert.ID, m); err != nil {
		t.Fatal(err)
	}

	signed, err := s.Sign(ctx, org, model.Test, []byte(`{"id":"abc","type":"01"}`), "application/json")
	if err != nil {
		t.Fatal(err)
	}
	if signed.Document.ID != "abc" || signed.Document.Type != "01" {
		t.Fatalf("document = %+v", signed.Document)
	}
	if material.Len() != 1 {
		t.Fatalf("material kept: %d", material.Len())
	}
	v, err := s.Verify(ctx, org, model.Test, signed.Signature)
	if err != nil || !v.Valid {
		t.Fatalf("verify: %+v %v", v, err)
	}

	// The material is bound to its certificate's id: another row can't use it.
	other := cert
	other.ID = ids.New()
	cert = other
	if _, err := s.Sign(ctx, org, model.Test, []byte(`{"id":"1"}`), "application/json"); err == nil {
		t.Fatal("material sealed for another certificate opened")
	}

	// Expired.
	cert.ID = ids.New()
	sealed, _ = s.Seal(cert.ID, m)
	past := time.Now().Add(-time.Hour)
	cert.NotAfter = &past
	if _, err := s.Sign(ctx, org, model.Test, []byte(`{"id":"1"}`), "application/json"); !errors.Is(err, ErrCertificateExpired) {
		t.Fatalf("expired: %v", err)
	}

	// Errors of the country and its availability map to their problems.
	if _, err := s.Sign(ctx, store.Organization{Country: "CR"}, model.Test, nil, "application/json"); !errors.Is(err, ErrCountryNotEnabled) {
		t.Fatalf("CR isn't enabled: %v", err)
	}
	if _, err := s.Country("ZZ"); err != nil {
		t.Fatalf("ZZ: %v", err)
	}
	zz, _ := s.Country("ZZ")
	_, err = zz.Sign(signing.Material{}, nil, "", signing.Options{})
	if p := signingProblem(err); p == nil || p.Status != 501 || p.Type != "country-not-implemented" {
		t.Fatalf("not implemented → %+v", p)
	}
	if p := signingProblem(signing.ErrInvalidDocument); p.Status != 422 {
		t.Fatalf("invalid document → %+v", p)
	}
	if p := signingProblem(ErrCountryNotEnabled); p.Status != 409 || p.Type != "country-not-enabled" {
		t.Fatalf("country not enabled → %+v", p)
	}
}
