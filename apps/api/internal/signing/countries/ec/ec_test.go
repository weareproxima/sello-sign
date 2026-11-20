package ec

import (
	"crypto/x509/pkix"
	"strings"
	"testing"

	"proxima.la/sello/sign/internal/signing/xades/xadestest"
)

func TestVectors(t *testing.T) { xadestest.RunVectors(t, New(), "ec", true) }

func TestVerifyRejectsTampering(t *testing.T) {
	vectors := xadestest.Load(t, "ec")
	m := xadestest.Material(t)
	signed := string(vectors.Cases[0].Output)
	for name, edit := range map[string][2]string{
		"comprobante":  {"<importeTotal>115.00", "<importeTotal>115.01"},
		"signing-time": {"<etsi:SigningTime>2026", "<etsi:SigningTime>2025"},
		"key-info":     {"<ds:Exponent>AQAB", "<ds:Exponent>AQAA"},
	} {
		tampered := strings.Replace(signed, edit[0], edit[1], 1)
		if tampered == signed {
			t.Fatalf("%s: edit did not apply", name)
		}
		if res, _ := New().Verify(m, []byte(tampered)); res.Valid {
			t.Errorf("%s: tampered signature verified", name)
		}
	}
}

func TestResignReplacesSignature(t *testing.T) {
	vectors := xadestest.Load(t, "ec")
	m := xadestest.Material(t)
	v := vectors.Cases[0]
	again, err := New().Sign(m, v.Output, "application/xml", vectors.Options(v))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(again.Signature), "<ds:Signature "); n != 1 {
		t.Errorf("%d signatures after re-signing", n)
	}
	if res, _ := New().Verify(m, again.Signature); !res.Valid {
		t.Error("the re-signed comprobante does not verify")
	}
}

func TestEscapesTemplateValues(t *testing.T) {
	xadestest.CheckOddIssuer(t, New(), xadestest.Load(t, "ec").Cases[0].Input, true)
}

func TestParseCertificate(t *testing.T) {
	xadestest.CheckTaxID(t, New(), pkix.Name{CommonName: "EMPRESA", SerialNumber: "1791234567001"}, "1791234567001")
}

func TestOddText(t *testing.T) {
	xadestest.CheckOddText(t, New(), xadestest.Load(t, "ec").Cases[0].Input, "razonSocial", true)
}
