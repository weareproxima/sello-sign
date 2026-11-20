package sv

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/signingtest"
)

// crt is an MH .crt as the Ministerio de Hacienda hands it out, with the
// fixture key.
func crt(t *testing.T, key *rsa.PrivateKey) []byte {
	priv, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	b64 := base64.StdEncoding.EncodeToString
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<CertificadoMH>
  <_id>06141804941035</_id>
  <nit>06141804941035</nit>
  <publicKey><keyType>PUBLIC</keyType><algorithm>RSA</algorithm><encodied>` + b64(pub) + `</encodied><format>X.509</format></publicKey>
  <privateKey><keyType>PRIVATE</keyType><algorithm>RSA</algorithm><encodied>
    ` + b64(priv)[:64] + `
    ` + b64(priv)[64:] + `
  </encodied><format>PKCS#8</format><clave>x</clave></privateKey>
  <activo>true</activo>
  <certificado><basicEstructure><version>3</version><serial>10104757676808110649</serial>
  <issuer><countryName>SV</countryName><organizationalName>MINISTERIO DE HACIENDA</organizationalName><commonName>UNIDAD DE INNOVACION</commonName></issuer><validity><notBefore>1767225600.000</notBefore><notAfter>1830297600.5</notAfter></validity>
  <subject><countryName>SV</countryName><organizationName>EMPRESA DE PRUEBA SA DE CV</organizationName><givenName>JUAN PEREZ</givenName><commonName>EMPRESA SA</commonName></subject></basicEstructure></certificado>
</CertificadoMH>`)
}

func TestParseCertificate(t *testing.T) {
	key := signingtest.Material(t).Key.(*rsa.PrivateKey)
	m, info, err := New().ParseCertificate(signing.Upload{Data: crt(t, key)})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Key.(*rsa.PrivateKey).Equal(key) || m.Certificate != nil {
		t.Error("wrong material")
	}
	want := signing.CertificateInfo{
		TaxID:     "06141804941035",
		Subject:   "EMPRESA SA, JUAN PEREZ, EMPRESA DE PRUEBA SA DE CV",
		Issuer:    "UNIDAD DE INNOVACION, MINISTERIO DE HACIENDA",
		Serial:    "8C3B4F91DB8C8239",
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2028, 1, 1, 0, 0, 0, 500e6, time.UTC),
	}
	if info != want {
		t.Errorf("info = %+v\nwant   %+v", info, want)
	}
	// What it reads signs as the fixture does.
	c := signingtest.Load(t, "sv").Case(t, "dte")
	got, err := New().Sign(m, c.Input(t), c.ContentType, c.Options(t))
	if err != nil || string(got.Signature) != string(c.Output(t)) {
		t.Errorf("signature with the .crt key differs: %v", err)
	}

	for name, data := range map[string]string{
		"not a .crt":  "-----BEGIN CERTIFICATE-----",
		"no key":      strings.Replace(string(crt(t, key)), "<privateKey>", "<otherKey>", 1),
		"bad base64":  strings.Replace(string(crt(t, key)), "<encodied>\n", "<encodied>!!", 1),
		"no validity": strings.Replace(string(crt(t, key)), "<notAfter>", "<notafter>", 1),
	} {
		if _, _, err := New().ParseCertificate(signing.Upload{Data: []byte(data)}); !errors.Is(err, signing.ErrInvalidCertificate) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSmallKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	_, err = New().Sign(signing.Material{Key: key}, []byte(`{}`), "application/json", signing.Options{})
	if !errors.Is(err, signing.ErrInvalidCertificate) {
		t.Errorf("a 1024-bit key: %v", err)
	}
}

func TestSignErrors(t *testing.T) {
	m := signingtest.Material(t)
	for body, want := range map[string]error{
		`null`:             signing.ErrInvalidDocument,
		`{"a":1,}`:         signing.ErrInvalidDocument,
		`{"a":01}`:         signing.ErrInvalidDocument,
		"{\"a\":\"\x01\"}": signing.ErrInvalidDocument,
	} {
		if _, err := New().Sign(m, []byte(body), "application/json", signing.Options{}); !errors.Is(err, want) {
			t.Errorf("%q: %v", body, err)
		}
	}
	if _, err := New().Sign(m, []byte(`{}`), "text/plain", signing.Options{}); !errors.Is(err, signing.ErrUnsupportedContentType) {
		t.Errorf("text/plain: %v", err)
	}
}
