package cr

import (
	"crypto/x509/pkix"
	"strings"
	"testing"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/xades/xadestest"
)

func TestVectors(t *testing.T) { xadestest.RunVectors(t, New(), "cr", false) }

func TestVerifyRejectsTampering(t *testing.T) {
	vectors := xadestest.Load(t, "cr")
	m := xadestest.Material(t)
	signed := string(vectors.Cases[0].Output)
	for name, edit := range map[string][2]string{
		"document":     {"<TotalComprobante>11800.00", "<TotalComprobante>11801.00"},
		"signing-time": {"<xades:SigningTime>2026", "<xades:SigningTime>2025"},
	} {
		tampered := strings.Replace(signed, edit[0], edit[1], 1)
		if tampered == signed {
			t.Fatalf("%s: edit did not apply", name)
		}
		if res, _ := New().Verify(m, []byte(tampered)); res.Valid {
			t.Errorf("%s: tampered signature verified", name)
		}
	}
	if res, _ := New().Verify(m, vectors.Cases[0].Input); res.Valid {
		t.Error("an unsigned document verified")
	}
}

func TestSignRejects(t *testing.T) {
	m := xadestest.Material(t)
	if _, err := New().Sign(m, []byte(`{}`), "application/json", signing.Options{}); err != signing.ErrUnsupportedContentType {
		t.Errorf("JSON: %v", err)
	}
	if _, err := New().Sign(signing.Material{Key: m.Key}, []byte(`<a/>`), "application/xml", signing.Options{}); err == nil {
		t.Error("signed without a certificate")
	}
}

func TestEscapesTemplateValues(t *testing.T) {
	xadestest.CheckOddIssuer(t, New(), xadestest.Load(t, "cr").Cases[0].Input, false)
}

func TestParseCertificate(t *testing.T) {
	xadestest.CheckTaxID(t, New(), pkix.Name{CommonName: "EMPRESA TEST SA", SerialNumber: "CPJ-3-101-123456"}, "3101123456")
	xadestest.CheckTaxID(t, New(), pkix.Name{CommonName: "JUAN PEREZ (FIRMA) 0112340567"}, "0112340567")
}

func TestOddText(t *testing.T) {
	xadestest.CheckOddText(t, New(), xadestest.Load(t, "cr").Cases[0].Input, "Nombre", false)
}
