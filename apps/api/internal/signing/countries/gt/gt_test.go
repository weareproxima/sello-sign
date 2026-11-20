package gt

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"strings"
	"testing"

	"proxima.la/sello/sign/internal/signing/xades/xadestest"
)

func TestVectors(t *testing.T) { xadestest.RunVectors(t, New(), "gt", false) }

func TestVerifyRejectsTampering(t *testing.T) {
	vectors := xadestest.Load(t, "gt")
	m := xadestest.Material(t)
	signed := string(vectors.Cases[0].Output)
	for name, edit := range map[string][2]string{
		"datos-emision": {"<dte:GranTotal>100.00", "<dte:GranTotal>100.01"},
		"signing-time":  {"<xades:SigningTime>2026", "<xades:SigningTime>2025"},
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

func TestResignReplacesOwnSignature(t *testing.T) {
	vectors := xadestest.Load(t, "gt")
	m := xadestest.Material(t)
	for _, v := range vectors.Cases {
		if v.Name != "certificado" {
			continue
		}
		again, err := New().Sign(m, v.Output, "application/xml", vectors.Options(v))
		if err != nil {
			t.Fatal(err)
		}
		s := string(again.Signature)
		if strings.Count(s, `Id="FirmaEmisor"`) != 1 || strings.Count(s, `Id="FirmaCertificador"`) != 1 {
			t.Errorf("re-signing should keep one FirmaEmisor and the certifier's: %s", s)
		}
		if strings.Index(s, `Id="FirmaEmisor"`) > strings.Index(s, `Id="FirmaCertificador"`) {
			t.Error("FirmaEmisor must come before FirmaCertificador")
		}
		if res, _ := New().Verify(m, again.Signature); !res.Valid {
			t.Error("the re-signed document does not verify")
		}
	}
}

func TestNIT(t *testing.T) {
	sn := asn1.ObjectIdentifier{2, 5, 4, 5}
	cn := asn1.ObjectIdentifier{2, 5, 4, 3}
	ou := asn1.ObjectIdentifier{2, 5, 4, 11}
	for _, c := range []struct {
		subject []pkix.AttributeTypeAndValue
		want    string
	}{
		{[]pkix.AttributeTypeAndValue{{Type: sn, Value: "1234567-8"}}, "12345678"},
		{[]pkix.AttributeTypeAndValue{{Type: sn, Value: "NIT 1234567-k"}}, "1234567K"},
		{[]pkix.AttributeTypeAndValue{{Type: sn, Value: "ABC"}}, "ABC"},
		{[]pkix.AttributeTypeAndValue{{Type: cn, Value: "EMPRESA"}, {Type: ou, Value: "NIT: 9876543-K"}}, "9876543K"},
		{[]pkix.AttributeTypeAndValue{{Type: cn, Value: "EMPRESA SA"}}, ""},
	} {
		if got := NIT(c.subject); got != c.want {
			t.Errorf("NIT(%v) = %q, want %q", c.subject, got, c.want)
		}
	}
}

func TestEscapesTemplateValues(t *testing.T) {
	xadestest.CheckOddIssuer(t, New(), xadestest.Load(t, "gt").Cases[0].Input, false)
}

func TestParseCertificate(t *testing.T) {
	xadestest.CheckTaxID(t, New(), pkix.Name{CommonName: "EMPRESA", SerialNumber: "1234567-8"}, "12345678")
	xadestest.CheckTaxID(t, New(), pkix.Name{CommonName: "EMPRESA", OrganizationalUnit: []string{"NIT 9876543-K"}}, "9876543K")
}

func TestOddText(t *testing.T) {
	xadestest.CheckOddText(t, New(), xadestest.Load(t, "gt").Cases[0].Input, "dte:Direccion", false)
}
