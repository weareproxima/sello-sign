package xades_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"math/big"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/xades"
	"proxima.la/sello/sign/internal/signing/xades/xadestest"
)

// TestIssuerName checks the X509IssuerName: node-forge's layout (the
// attributes reversed, short names where forge has them, long names or the
// OID otherwise, joined by "," without escaping) with each value decoded by
// its string type. forge read UTF-8 values byte by byte ("CÃ­a"); that bug
// was dropped on 2026-11-19, so only ASCII names match forge byte for byte.
func TestIssuerName(t *testing.T) {
	name := pkix.Name{
		Country:            []string{"CR"},
		Organization:       []string{"Banco & Cía <SA>"},
		OrganizationalUnit: []string{"Señor José"},
		CommonName:         "CA $& $' test",
		SerialNumber:       "CPJ-3-101-123456",
		ExtraNames: []pkix.AttributeTypeAndValue{
			{Type: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}, Value: "ca@example.com"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 97}, Value: "VATCR-123"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 42}, Value: "Ñandú"},
			// A BMPString (UTF-16, big-endian), which pkix never writes.
			{Type: asn1.ObjectIdentifier{2, 5, 4, 7}, Value: asn1.RawValue{Tag: 30, Bytes: []byte{0, 'L', 0, 'e', 0, 0xf3, 0, 'n'}}},
		},
	}
	raw, err := asn1.Marshal(name.ToRDNSequence())
	if err != nil {
		t.Fatal(err)
	}
	got, err := xades.IssuerName(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := "L=León,givenName=Ñandú,2.5.4.97=VATCR-123,E=ca@example.com,serialNumber=CPJ-3-101-123456,CN=CA $& $' test,OU=Señor José,O=Banco & Cía <SA>,C=CR"
	if got != want {
		t.Errorf("IssuerName = %q\nwant         %q", got, want)
	}

	// An ASCII name comes out as forge wrote it.
	ascii := pkix.Name{Country: []string{"SV"}, Organization: []string{"Ministerio de Hacienda"}, CommonName: "CA MH"}
	raw, err = asn1.Marshal(ascii.ToRDNSequence())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := xades.IssuerName(raw); got != "CN=CA MH,O=Ministerio de Hacienda,C=SV" {
		t.Errorf("IssuerName(ASCII) = %q", got)
	}
}

func TestCertificateOf(t *testing.T) {
	m := xadestest.Material(t)
	c, err := xades.CertificateOf(m.Certificate)
	if err != nil {
		t.Fatal(err)
	}
	if c.IssuerName != "C=SV,O=Pruebas,CN=EMPRESA DE PRUEBA" {
		t.Errorf("IssuerName = %q", c.IssuerName)
	}
	if c.Serial != m.Certificate.SerialNumber.String() {
		t.Errorf("Serial = %s", c.Serial)
	}
}

func TestReadUpload(t *testing.T) {
	m := xadestest.Material(t)
	key := m.Key.(*rsa.PrivateKey)
	ca := caCertificate(t)
	for name, enc := range map[string]*pkcs12.Encoder{"modern": pkcs12.Modern, "legacy": pkcs12.LegacyRC2} {
		// The CA first: the signer's certificate is the one of the key.
		p12, err := enc.Encode(key, m.Certificate, []*x509.Certificate{ca}, " pin con espacios ")
		if err != nil {
			t.Fatal(err)
		}
		got, err := xades.ReadUpload(signing.Upload{Data: p12, Password: " pin con espacios "})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !got.Certificate.Equal(m.Certificate) || len(got.Chain) != 1 || !got.Chain[0].Equal(ca) {
			t.Errorf("%s: wrong certificate or chain", name)
		}
		if _, err := xades.ReadUpload(signing.Upload{Data: p12, Password: "pin con espacios"}); !errors.Is(err, signing.ErrInvalidCertificate) {
			t.Errorf("%s: a wrong PIN gave %v", name, err)
		}
	}

	pemBundle, err := signing.EncodeMaterial(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := xades.ReadUpload(signing.Upload{Data: pemBundle})
	if err != nil || !got.Certificate.Equal(m.Certificate) {
		t.Errorf("PEM: %v", err)
	}
	if _, err := xades.ReadUpload(signing.Upload{Data: xadestest.PEM(m.Certificate)}); !errors.Is(err, signing.ErrInvalidCertificate) {
		t.Errorf("a PEM without its key gave %v", err)
	}
	if _, err := xades.ReadUpload(signing.Upload{Data: []byte("not a certificate")}); !errors.Is(err, signing.ErrInvalidCertificate) {
		t.Errorf("garbage gave %v", err)
	}
}

func caCertificate(t *testing.T) *x509.Certificate {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "CA"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	return c
}

// TestRandomness checks UUID and RandomInt against what the vectors'
// harness recorded from Node (tools/vectors, random_calls).
func TestRandomness(t *testing.T) {
	stream, _ := hex.DecodeString("23555aef1bda49f7898921cd0788ff60e9fbbf843e56242d66faaa303ff0f3af")
	r := bytes.NewReader(stream)
	for _, want := range []string{"23555aef-1bda-49f7-8989-21cd0788ff60", "e9fbbf84-3e56-442d-a6fa-aa303ff0f3af"} {
		if got, err := xades.UUID(r); err != nil || got != want {
			t.Errorf("UUID = %s, %v; want %s", got, err, want)
		}
	}
	// 6 bytes big-endian, modulo the span, with Node's rejection limit.
	r = bytes.NewReader([]byte{0, 0, 0, 0, 0x03, 0xe8})
	if got, _ := xades.RandomInt(r, 990, 999990); got != 990+1000 {
		t.Errorf("RandomInt = %d", got)
	}
	r = bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 1})
	if got, _ := xades.RandomInt(r, 990, 999990); got != 991 {
		t.Errorf("RandomInt did not reject a draw over the limit: %d", got)
	}
}
