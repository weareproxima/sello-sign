package httpapi

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/signing"
)

// fakeCountry signs with Ed25519: a test's stand-in for a real signer.
// A certificate is the bytes "fake:<tax id>"; a document is JSON with an
// "id" and a "type"; a signature is "<base64 body>.<base64 signature>".
type fakeCountry struct{ code string }

func (f fakeCountry) Code() string { return f.code }

func (f fakeCountry) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	tax, ok := strings.CutPrefix(string(u.Data), "fake:")
	if !ok {
		return signing.Material{}, signing.CertificateInfo{}, signing.ErrInvalidCertificate
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	now := time.Now()
	return signing.Material{Key: priv}, signing.CertificateInfo{
		TaxID: tax, Subject: "CN=" + tax, Issuer: "CN=Fake CA", Serial: "01",
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(365 * 24 * time.Hour),
	}, nil
}

func (f fakeCountry) Sign(m signing.Material, body []byte, contentType string, _ signing.Options) (signing.Signed, error) {
	if contentType != "application/json" {
		return signing.Signed{}, signing.ErrUnsupportedContentType
	}
	var doc struct{ ID, Type string }
	if err := json.Unmarshal(body, &doc); err != nil || doc.ID == "" {
		return signing.Signed{}, signing.ErrInvalidDocument
	}
	sig := ed25519.Sign(m.Key.(ed25519.PrivateKey), body)
	out := base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig)
	return signing.Signed{Signature: []byte(out), ContentType: "application/jose", Document: signing.Document{ID: doc.ID, Type: doc.Type}}, nil
}

func (f fakeCountry) Verify(m signing.Material, signature []byte) (signing.Verification, error) {
	b, s, ok := bytes.Cut(signature, []byte("."))
	if !ok {
		return signing.Verification{}, signing.ErrInvalidDocument
	}
	body, err1 := base64.RawURLEncoding.DecodeString(string(b))
	sig, err2 := base64.RawURLEncoding.DecodeString(string(s))
	if err1 != nil || err2 != nil {
		return signing.Verification{}, signing.ErrInvalidDocument
	}
	var doc struct{ ID, Type string }
	_ = json.Unmarshal(body, &doc)
	pub := m.Key.(ed25519.PrivateKey).Public().(ed25519.PublicKey)
	return signing.Verification{Valid: ed25519.Verify(pub, body, sig), Document: signing.Document{ID: doc.ID, Type: doc.Type}, Payload: body}, nil
}

// testRegistry has the fake as SV and CR, and ZZ without a signer.
func testRegistry() *signing.Registry {
	return signing.NewRegistry(fakeCountry{"SV"}, fakeCountry{"CR"}, signing.NotImplemented("ZZ"))
}
