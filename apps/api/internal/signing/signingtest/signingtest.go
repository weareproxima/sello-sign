// Package signingtest is what the countries' tests share: the fixed test key
// and certificate, the reference signatures tools/vectors makes with the
// TypeScript signers (testdata/vectors/<cc>), and an independent XMLDSig
// verifier (helium) to check that a signature holds outside our code. Only
// tests import it.
package signingtest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/xmldsig1"
	"software.sslmate.com/src/go-pkcs12"

	"proxima.la/sello/sign/internal/signing"
)

// Dir is internal/signing/testdata.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata")
}

// KeyPEM and CertificatePEM are the fixtures every vector is signed with.
func KeyPEM(t testing.TB) []byte         { return read(t, filepath.Join(Dir(), "key.pem")) }
func CertificatePEM(t testing.TB) []byte { return read(t, filepath.Join(Dir(), "cert.pem")) }

// Material is the fixture key with its certificate.
func Material(t testing.TB) signing.Material {
	t.Helper()
	m, err := signing.ParseKeyAndCertificate(signing.Upload{Data: append(KeyPEM(t), CertificatePEM(t)...)})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Certificate is the fixture certificate.
func Certificate(t testing.TB) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(CertificatePEM(t))
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Manifest is a country's testdata/vectors/<cc>/manifest.json.
type Manifest struct {
	Country string `json:"country"`
	Now     string `json:"now"`
	Cases   []Case `json:"cases"`
	dir     string
}

// Case is one reference document and what the TypeScript signer made of it.
type Case struct {
	Name         string `json:"name"`
	Note         string `json:"note"`
	ContentType  string `json:"content_type"`
	InputFile    string `json:"input"`
	OutputFile   string `json:"output"`
	DocumentID   string `json:"document_id"`
	DocumentType string `json:"document_type"`
	SelfVerified bool   `json:"self_verified"`
	Error        string `json:"error"`
	Random       string `json:"random"`
	manifest     *Manifest
}

// Load reads a country's manifest.
func Load(t testing.TB, cc string) *Manifest {
	t.Helper()
	dir := filepath.Join(Dir(), "vectors", cc)
	var m Manifest
	if err := json.Unmarshal(read(t, filepath.Join(dir, "manifest.json")), &m); err != nil {
		t.Fatal(err)
	}
	m.dir = dir
	for i := range m.Cases {
		m.Cases[i].manifest = &m
	}
	return &m
}

// Case finds a case by name.
func (m *Manifest) Case(t testing.TB, name string) Case {
	t.Helper()
	for _, c := range m.Cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no case %s in %s", name, m.dir)
	return Case{}
}

// Input is the document as it was sent.
func (c Case) Input(t testing.TB) []byte { return read(t, filepath.Join(c.manifest.dir, c.InputFile)) }

// Output is what the TypeScript signer answered.
func (c Case) Output(t testing.TB) []byte {
	return read(t, filepath.Join(c.manifest.dir, c.OutputFile))
}

// Options fixes the time and randomness the TypeScript signer had.
func (c Case) Options(t testing.TB) signing.Options {
	t.Helper()
	now, err := time.Parse(time.RFC3339Nano, c.manifest.Now)
	if err != nil {
		t.Fatal(err)
	}
	random, err := hex.DecodeString(c.Random)
	if err != nil {
		t.Fatal(err)
	}
	return signing.Options{Now: now, Rand: &strictReader{r: bytes.NewReader(random)}}
}

// strictReader fails the read that goes past what the TypeScript signer
// consumed, so a test notices a signer reading more randomness than it did.
type strictReader struct{ r *bytes.Reader }

func (s *strictReader) Read(p []byte) (int, error) {
	if s.r.Len() < len(p) {
		return 0, errors.New("signingtest: more randomness read than the TypeScript signer used")
	}
	return s.r.Read(p)
}

var _ io.Reader = (*strictReader)(nil)

// Helium verifies every ds:Signature of signed with helium's xmldsig1
// verifier (SHA-1 allowed) against cert, and says how many it checked.
func Helium(signed []byte, cert *x509.Certificate) (int, error) {
	ctx := context.Background()
	doc, err := helium.NewParser().Parse(ctx, signed)
	if err != nil {
		return 0, fmt.Errorf("helium: parse: %w", err)
	}
	var sigs []*helium.Element
	var walk func(helium.Node)
	walk = func(n helium.Node) {
		e, ok := helium.AsNode[*helium.Element](n)
		if !ok {
			return
		}
		if e.LocalName() == "Signature" && e.URI() == "http://www.w3.org/2000/09/xmldsig#" {
			sigs = append(sigs, e)
			return
		}
		for c := e.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(doc.DocumentElement())
	if len(sigs) == 0 {
		return 0, errors.New("helium: no signature")
	}
	v := xmldsig1.NewVerifier(xmldsig1.X509CertKeySource(cert)).AllowSHA1(true)
	for i, s := range sigs {
		if _, err := v.VerifyElement(ctx, doc, s); err != nil {
			return i, fmt.Errorf("helium: signature %d: %w", i, err)
		}
	}
	return len(sigs), nil
}

func read(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Expect tunes Run for a country.
type Expect struct {
	// Fixed are the cases where the TypeScript signer was wrong (its own
	// verify rejected what it signed), by name, with why: the Go signature
	// must differ from it, and verify where it did not.
	Fixed map[string]string
	// Helium checks a signature independently; nil means Helium over the
	// whole document, and NoHelium skips the check.
	Helium   func(t *testing.T, name string, signed []byte) error
	NoHelium bool
	// TSStillValid are the fixed cases whose TypeScript signature does
	// verify, by name, with why it was changed anyway.
	TSStillValid map[string]string
	// Tamper is a change to signed content, {old, new}: Verify must reject
	// every signature it applies to, and it must apply to one at least.
	Tamper [2]string
}

func (e Expect) check(t *testing.T, name string, signed []byte) error {
	if e.Helium != nil {
		return e.Helium(t, name, signed)
	}
	_, err := Helium(signed, Certificate(t))
	return err
}

// Run checks country against every vector of cc: the Go signature is the
// TypeScript one byte for byte (or, for a fixed case, a different one that
// verifies), the document id and type are the same, Verify accepts both
// signatures (the TypeScript one when it is right), and an independent
// verifier accepts the Go one and rejects a wrong TypeScript one.
func Run(t *testing.T, country signing.Country, cc string, e Expect) {
	m := Load(t, cc)
	material := Material(t)
	tampered := 0
	defer func() {
		if e.Tamper[0] != "" && tampered == 0 {
			t.Errorf("the tamper %q applies to no case", e.Tamper[0])
		}
	}()
	for _, c := range m.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := country.Sign(material, c.Input(t), c.ContentType, c.Options(t))
			if c.Error != "" {
				if err == nil {
					t.Fatalf("signed what the TypeScript signer refused (%s)", c.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := c.Output(t)
			why, fixed := e.Fixed[c.Name]
			switch {
			case fixed && bytes.Equal(got.Signature, want):
				t.Errorf("same bytes as the TypeScript signer, which was wrong here: %s", why)
			case !fixed && !bytes.Equal(got.Signature, want):
				t.Errorf("differs from the TypeScript signer\n got: %s\nwant: %s", got.Signature, want)
			}
			if got.Document.ID != c.DocumentID || got.Document.Type != c.DocumentType {
				t.Errorf("document = %+v, want {ID:%s Type:%s}", got.Document, c.DocumentID, c.DocumentType)
			}
			v, err := country.Verify(material, got.Signature)
			if err != nil || !v.Valid {
				t.Errorf("Verify rejects our signature: %v", err)
			} else if c.Random == "" && v.Document != got.Document {
				// (A random document id is not in the document.)
				t.Errorf("Verify document = %+v, want %+v", v.Document, got.Document)
			}
			switch {
			case fixed:
				// The TypeScript signature is the wrong one: an independent
				// verifier must reject it too, unless it says otherwise.
				if !e.NoHelium && e.TSStillValid[c.Name] == "" {
					if e.check(t, c.Name, want) == nil {
						t.Errorf("the independent verifier accepts the TypeScript signature: is it really wrong?")
					}
				}
			case c.SelfVerified:
				if v, err := country.Verify(material, want); err != nil || !v.Valid {
					t.Errorf("Verify rejects the TypeScript signature: %v", err)
				}
			default:
				t.Errorf("the TypeScript signer's verify rejected this case, but it is not in Fixed")
			}
			if e.Tamper[0] != "" && bytes.Contains(got.Signature, []byte(e.Tamper[0])) {
				tampered++
				bad := bytes.Replace(got.Signature, []byte(e.Tamper[0]), []byte(e.Tamper[1]), 1)
				if v, _ := country.Verify(material, bad); v.Valid {
					t.Errorf("Verify accepts the signature with %q changed", e.Tamper[0])
				}
			}
			if e.NoHelium {
				return
			}
			if err := e.check(t, c.Name, got.Signature); err != nil {
				t.Errorf("independent verification: %v", err)
			}
		})
	}
}

// Issue makes a certificate for the fixture key with subject (whose
// ExtraNames go in as given) and extra extensions, self-signed.
func Issue(t testing.TB, subject pkix.Name, extensions ...pkix.Extension) *x509.Certificate {
	t.Helper()
	key := Material(t).Key
	tmpl := &x509.Certificate{
		SerialNumber:    big.NewInt(20261119),
		Subject:         subject,
		Issuer:          pkix.Name{CommonName: "CA DE PRUEBA"},
		NotBefore:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:        time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
		ExtraExtensions: extensions,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// PKCS12 packs the fixture key with cert (and a chain) as a PKCS #12, with
// the modern (AES) encryption and the legacy (3DES/RC2) one.
func PKCS12(t testing.TB, cert *x509.Certificate, password string, legacy bool, chain ...*x509.Certificate) []byte {
	t.Helper()
	enc := pkcs12.Modern2023
	if legacy {
		enc = pkcs12.LegacyRC2
	}
	data, err := enc.Encode(Material(t).Key, cert, chain, password)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// ExtraName is a subject attribute of type oid with a string value.
func ExtraName(oid asn1.ObjectIdentifier, value string) pkix.AttributeTypeAndValue {
	return pkix.AttributeTypeAndValue{Type: oid, Value: value}
}

// CheckParse uploads cert as a PKCS #12 (modern and legacy, with a CA in
// the chain) and as PEM, and checks what the country reads: the tax id, the
// certificate, the key that signs with it.
func CheckParse(t *testing.T, country signing.Country, cert *x509.Certificate, taxID string) {
	t.Helper()
	ca := Issue(t, pkix.Name{CommonName: "CA INTERMEDIA"})
	uploads := map[string]signing.Upload{
		"p12":        {Data: PKCS12(t, cert, "clave secreta", false, ca), Password: "clave secreta"},
		"p12-legacy": {Data: PKCS12(t, cert, "1234", true), Password: "1234"},
		"pem":        {Data: append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), KeyPEM(t)...)},
	}
	for name, u := range uploads {
		m, info, err := country.ParseCertificate(u)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if info.TaxID != taxID {
			t.Errorf("%s: tax id %q, want %q", name, info.TaxID, taxID)
		}
		if !m.Certificate.Equal(cert) || m.Key == nil {
			t.Errorf("%s: wrong certificate or no key", name)
		}
		if !info.NotAfter.Equal(cert.NotAfter) || info.Subject != cert.Subject.String() || info.Serial != "13528FF" {
			t.Errorf("%s: info %+v", name, info)
		}
	}
	if _, _, err := country.ParseCertificate(signing.Upload{Data: uploads["p12"].Data, Password: "otra"}); !errors.Is(err, signing.ErrInvalidCertificate) {
		t.Errorf("wrong password: %v", err)
	}
}
