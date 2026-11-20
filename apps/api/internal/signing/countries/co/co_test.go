package co

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"strings"
	"testing"

	"proxima.la/sello/sign/internal/signing/xades/xadestest"
)

func TestVectors(t *testing.T) { xadestest.RunVectors(t, New(), "co", false) }

func TestVerifyRejectsTampering(t *testing.T) {
	vectors := xadestest.Load(t, "co")
	m := xadestest.Material(t)
	signed := string(vectors.Cases[0].Output)
	for name, edit := range map[string][2]string{
		"document":       {"<sts:Prefix>SETP", "<sts:Prefix>SETQ"},
		"signing-time":   {"<xades:SigningTime>2026", "<xades:SigningTime>2025"},
		"claimed-role":   {"<xades:ClaimedRole>supplier", "<xades:ClaimedRole>third party"},
		"policy-digest":  {"dMoMvtcG5aIzgYo0", "dMoMvtcG5aIzgYo1"},
		"two-signatures": {"</ext:UBLExtensions>", "</ext:UBLExtensions><ds:Signature xmlns:ds=\"http://www.w3.org/2000/09/xmldsig#\"/>"},
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
	vectors := xadestest.Load(t, "co")
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
		t.Error("the re-signed document does not verify")
	}
}

func TestNIT(t *testing.T) {
	attr := func(oid asn1.ObjectIdentifier, v string) pkix.AttributeTypeAndValue {
		return pkix.AttributeTypeAndValue{Type: oid, Value: v}
	}
	sn, ou, o, cn := asn1.ObjectIdentifier{2, 5, 4, 5}, asn1.ObjectIdentifier{2, 5, 4, 11}, asn1.ObjectIdentifier{2, 5, 4, 10}, asn1.ObjectIdentifier{2, 5, 4, 3}
	for _, c := range []struct {
		names []pkix.AttributeTypeAndValue
		want  string
	}{
		{[]pkix.AttributeTypeAndValue{attr(sn, "900123456-7")}, "900123456"},
		{[]pkix.AttributeTypeAndValue{attr(sn, "11111111"), attr(ou, "Certificado emitido por Andes SCD Calle 26 69C 03")}, "11111111"},
		{[]pkix.AttributeTypeAndValue{attr(sn, "900.123.456-7")}, "900123456"},
		{[]pkix.AttributeTypeAndValue{attr(ou, "NIT 800197268-4"), attr(cn, "EMPRESA")}, "800197268"},
		{[]pkix.AttributeTypeAndValue{attr(o, "EMPRESA SAS"), attr(cn, "Usuario")}, ""},
		{[]pkix.AttributeTypeAndValue{attr(sn, "123456789012345")}, ""},
	} {
		cert := &x509.Certificate{Subject: pkix.Name{Names: c.names}}
		if got := NIT(cert); got != c.want {
			t.Errorf("NIT(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}

func TestEscapesTemplateValues(t *testing.T) {
	xadestest.CheckOddIssuer(t, New(), xadestest.Load(t, "co").Cases[0].Input, false)
}

func TestParseCertificate(t *testing.T) {
	xadestest.CheckTaxID(t, New(), pkix.Name{CommonName: "Usuario de Pruebas", SerialNumber: "900123456-7"}, "900123456")
}

func TestOddText(t *testing.T) {
	xadestest.CheckOddText(t, New(), xadestest.Load(t, "co").Cases[0].Input, "cbc:RegistrationName", false)
}
