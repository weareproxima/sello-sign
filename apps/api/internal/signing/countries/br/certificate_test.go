package br

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/signingtest"
)

// san builds a subjectAltName with ICP-Brasil otherNames, the value as
// [0] EXPLICIT OCTET STRING, as the issuers write it.
func san(t *testing.T, names map[string]string) pkix.Extension {
	var general []asn1.RawValue
	for oid, value := range names {
		var id asn1.ObjectIdentifier
		for _, n := range splitOID(oid) {
			id = append(id, n)
		}
		inner, _ := asn1.Marshal([]byte(value))
		wrapped, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: inner})
		oidDER, _ := asn1.Marshal(id)
		general = append(general, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: append(oidDER, wrapped...)})
	}
	der, err := asn1.Marshal(general)
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: oidSubjectAltName, Value: der}
}

func splitOID(s string) []int {
	var out []int
	n := 0
	for _, c := range s + "." {
		if c == '.' {
			out = append(out, n)
			n = 0
		} else {
			n = n*10 + int(c-'0')
		}
	}
	return out
}

func TestParseCertificate(t *testing.T) {
	cn := func(v string) pkix.Name {
		return pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{signingtest.ExtraName(signing.OIDCommonName, v)}}
	}
	t.Run("e-CNPJ CN", func(t *testing.T) {
		signingtest.CheckParse(t, New(), signingtest.Issue(t, cn("EMPRESA TESTE LTDA:12345678000199")), "12345678000199")
	})
	t.Run("e-CNPJ otherName", func(t *testing.T) {
		cert := signingtest.Issue(t, cn("EMPRESA TESTE LTDA"), san(t, map[string]string{
			"2.16.76.1.3.3": "12345678000199",
			"2.16.76.1.3.4": "0101198012345678901000000000000000000000000000",
		}))
		signingtest.CheckParse(t, New(), cert, "12345678000199")
		cnpj, cpf := TaxIDs(cert)
		if cnpj != "12345678000199" || cpf != "12345678901" {
			t.Errorf("TaxIDs = %s, %s", cnpj, cpf)
		}
	})
	t.Run("e-CPF", func(t *testing.T) {
		signingtest.CheckParse(t, New(), signingtest.Issue(t, cn("FULANO DE TAL:12345678901")), "12345678901")
	})
}
