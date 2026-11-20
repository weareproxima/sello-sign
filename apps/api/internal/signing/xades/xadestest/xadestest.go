// Package xadestest loads the reference vectors of tools/vectors
// (internal/signing/testdata/vectors/<cc>) for the XAdES countries' tests,
// and checks signatures with an independent XMLDSig verifier (helium's
// xmldsig1).
package xadestest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/xmldsig1"
	"software.sslmate.com/src/go-pkcs12"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

// Case is one vector: what the TypeScript signer signed and what it wrote.
type Case struct {
	Name         string `json:"name"`
	ContentType  string `json:"content_type"`
	InputFile    string `json:"input"`
	OutputFile   string `json:"output"`
	DocumentID   string `json:"document_id"`
	DocumentType string `json:"document_type"`
	SelfVerified bool   `json:"self_verified"`
	Random       string `json:"random"`
	Error        string `json:"error"`

	Input  []byte `json:"-"`
	Output []byte `json:"-"`
}

// Manifest is a country's vectors.
type Manifest struct {
	Country string    `json:"country"`
	Now     time.Time `json:"now"`
	Cases   []Case    `json:"cases"`
}

// Options fixes the time and the random bytes the case was signed with.
func (m Manifest) Options(c Case) signing.Options {
	r, err := hex.DecodeString(c.Random)
	if err != nil {
		panic(err)
	}
	return signing.Options{Now: m.Now, Rand: bytes.NewReader(r)}
}

// Testdata is internal/signing/testdata.
func Testdata() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// Load reads a country's manifest and its files.
func Load(t testing.TB, cc string) Manifest {
	t.Helper()
	dir := filepath.Join(Testdata(), "vectors", cc)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("vectors for %s: %v (cd tools/vectors && bun run gen.ts %s)", cc, err, cc)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for i := range m.Cases {
		c := &m.Cases[i]
		if c.Input, err = os.ReadFile(filepath.Join(dir, c.InputFile)); err != nil {
			t.Fatal(err)
		}
		if c.OutputFile != "" {
			if c.Output, err = os.ReadFile(filepath.Join(dir, c.OutputFile)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return m
}

// Material is the fixed key and certificate of the vectors.
func Material(t testing.TB) signing.Material {
	t.Helper()
	var bundle []byte
	for _, f := range []string{"key.pem", "cert.pem"} {
		b, err := os.ReadFile(filepath.Join(Testdata(), f))
		if err != nil {
			t.Fatal(err)
		}
		bundle = append(bundle, b...)
	}
	m, err := signing.DecodeMaterial(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Helium verifies a signed document with helium's xmldsig1, trusting only
// cert, and returns its error. sha1 allows SHA-1 (Ecuador).
func Helium(signed []byte, cert *x509.Certificate, sha1 bool) error {
	ctx := context.Background()
	doc, err := helium.NewParser().Parse(ctx, signed)
	if err != nil {
		return err
	}
	v := xmldsig1.NewVerifier(xmldsig1.X509CertKeySource(cert)).AllowSHA1(sha1)
	// With a certifier's signature after it (Guatemala), the issuer's is
	// the first one.
	for n := range helium.Descendants(doc) {
		if e, ok := n.(*helium.Element); ok && e.LocalName() == "Signature" && e.URI() == xmldsig1.NamespaceDSig {
			_, err = v.VerifyElement(ctx, doc, e)
			return err
		}
	}
	_, err = v.Verify(ctx, doc)
	return err
}

// PEM encodes a certificate, for tests that build uploads.
func PEM(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}

// RunVectors signs every vector of cc with c and checks that the output is
// byte-equal to the TS signer's, with the same document id and type, and
// that both signatures pass c.Verify and helium's verifier.
func RunVectors(t *testing.T, c signing.Country, cc string, sha1 bool) {
	vectors := Load(t, cc)
	m := Material(t)
	for _, v := range vectors.Cases {
		t.Run(v.Name, func(t *testing.T) {
			if v.Error != "" {
				t.Skipf("the TS signer failed: %s", v.Error)
			}
			got, err := c.Sign(m, v.Input, v.ContentType, vectors.Options(v))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Signature, v.Output) {
				t.Errorf("not byte-equal to the TS signer\n go: %s\n ts: %s", got.Signature, v.Output)
			}
			if got.Document.ID != v.DocumentID || got.Document.Type != v.DocumentType {
				t.Errorf("document = %+v, want %q/%q", got.Document, v.DocumentID, v.DocumentType)
			}
			for name, signed := range map[string][]byte{"ts": v.Output, "go": got.Signature} {
				res, err := c.Verify(m, signed)
				if err != nil || !res.Valid {
					t.Errorf("Verify(%s) = %v, %v", name, res.Valid, err)
				} else if res.Document.ID != v.DocumentID && v.SelfVerified {
					// A random id is not in the document: only a real one comes back.
					if !strings.EqualFold(res.Document.ID, v.DocumentID) && res.Document.ID != "" {
						t.Errorf("Verify(%s) document id = %q", name, res.Document.ID)
					}
				}
				if err := Helium(signed, m.Certificate, sha1); err != nil {
					t.Errorf("helium rejects the %s signature: %v", name, err)
				}
			}
		})
	}
}

// OddIssuer is an issuer whose values break a template that does not
// escape them (& and <) or that goes through String.prototype.replace ($&
// and $' are replacement patterns there).
var OddIssuer = pkix.Name{
	Country:      []string{"CR"},
	Organization: []string{"Banco & Cía <SA>"},
	CommonName:   "CA $& $' $` test",
}

// OddMaterial is a fresh key with a certificate from OddIssuer.
func OddMaterial(t testing.TB) signing.Material {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(4242),
		Subject:      pkix.Name{CommonName: "EMPRESA", SerialNumber: "3-101-123456"},
		Issuer:       OddIssuer,
		NotBefore:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	parent := *tmpl
	parent.Subject = OddIssuer
	der, err := x509.CreateCertificate(rand.Reader, tmpl, &parent, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return signing.Material{Key: key, Certificate: cert}
}

// CheckOddIssuer signs body with OddMaterial and checks that the issuer
// comes out escaped and that the signature verifies, with c.Verify and
// with helium.
func CheckOddIssuer(t *testing.T, c signing.Country, body []byte, sha1 bool) {
	m := OddMaterial(t)
	got, err := c.Sign(m, body, "application/xml", signing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "<ds:X509IssuerName>CN=CA $&amp; $' $` test,O=Banco &amp; Cía &lt;SA&gt;,C=CR</ds:X509IssuerName>"
	if !strings.Contains(string(got.Signature), want) {
		t.Errorf("issuer not escaped; want %s in\n%s", want, got.Signature)
	}
	if res, err := c.Verify(m, got.Signature); err != nil || !res.Valid {
		t.Errorf("Verify = %v, %v", res.Valid, err)
	}
	if err := Helium(got.Signature, m.Certificate, sha1); err != nil {
		t.Errorf("helium: %v", err)
	}
}

// CheckOddText puts, at the start of the text of the first <tag> of body, a
// carriage return (only "&#13;" can carry one) and a bare "&" (AT&T, which
// xmldom keeps as text), signs it with the vectors' material and checks
// that the signed document says the same, escaped so that a parser reads it
// back unchanged, and that the signature verifies with c.Verify and with
// helium. tag is the qualified name, and must be inside what is signed.
func CheckOddText(t *testing.T, c signing.Country, body []byte, tag string, sha1 bool) {
	t.Helper()
	open := "<" + tag + ">"
	if !bytes.Contains(body, []byte(open)) {
		t.Fatalf("no %s in the document", open)
	}
	odd := bytes.Replace(body, []byte(open), []byte(open+"AT&T one&#13;two "), 1)
	m := Material(t)
	got, err := c.Sign(m, odd, "application/xml", signing.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if want := open + "AT&amp;T one&#13;two "; !bytes.Contains(got.Signature, []byte(want)) {
		t.Errorf("want %s in\n%s", want, got.Signature)
	}
	doc, err := xmldsig.Parse(got.Signature)
	if err != nil {
		t.Fatal(err)
	}
	local := tag[strings.IndexByte(tag, ':')+1:]
	if text := xmldsig.TextContent(xmldsig.ElementByLocalName(doc.Root(), local)); !strings.HasPrefix(text, "AT&T one\rtwo ") {
		t.Errorf("the signed document reads %q", text)
	}
	if res, err := c.Verify(m, got.Signature); err != nil || !res.Valid {
		t.Errorf("Verify = %v, %v", res.Valid, err)
	}
	if err := Helium(got.Signature, m.Certificate, sha1); err != nil {
		t.Errorf("helium: %v", err)
	}
}

// P12 is a PKCS#12 upload, PIN "1234", for a fresh key and a self-signed
// certificate with that subject.
func P12(t testing.TB, subject pkix.Name) signing.Upload {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99), Subject: subject,
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	data, err := pkcs12.Modern.Encode(key, cert, nil, "1234")
	if err != nil {
		t.Fatal(err)
	}
	return signing.Upload{Data: data, Password: "1234"}
}

// CheckTaxID parses P12(subject) with c and checks the tax id it reads, and
// that the material signs.
func CheckTaxID(t *testing.T, c signing.Country, subject pkix.Name, want string) {
	t.Helper()
	m, info, err := c.ParseCertificate(P12(t, subject))
	if err != nil {
		t.Fatal(err)
	}
	if info.TaxID != want {
		t.Errorf("TaxID = %q, want %q", info.TaxID, want)
	}
	if m.Key == nil || m.Certificate == nil || info.NotAfter.Year() != 2028 {
		t.Errorf("material or info incomplete: %+v", info)
	}
	if _, _, err := c.ParseCertificate(signing.Upload{Data: P12(t, subject).Data, Password: "nope"}); !errors.Is(err, signing.ErrInvalidCertificate) {
		t.Errorf("a wrong PIN gave %v", err)
	}
}
