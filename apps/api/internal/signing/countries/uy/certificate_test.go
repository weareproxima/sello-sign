package uy

import (
	"crypto/x509/pkix"
	"testing"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestParseCertificate(t *testing.T) {
	name := func(attrs ...pkix.AttributeTypeAndValue) pkix.Name { return pkix.Name{ExtraNames: attrs} }
	cases := map[string]struct {
		subject pkix.Name
		rut     string
	}{
		"serialNumber token":       {name(signingtest.ExtraName(signing.OIDSerialNumber, "RUT 211234567890")), "211234567890"},
		"serialNumber digits":      {name(signingtest.ExtraName(signing.OIDSerialNumber, "RUC211234567890")), "211234567890"},
		"CN":                       {name(signingtest.ExtraName(signing.OIDCommonName, "EMPRESA TEST SA - 211234567890")), "211234567890"},
		"OU":                       {name(signingtest.ExtraName(signing.OIDOrganizationalUnit, "211234567890")), "211234567890"},
		"none, a 13-digit run too": {name(signingtest.ExtraName(signing.OIDCommonName, "X 2112345678901")), ""},
	}
	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			signingtest.CheckParse(t, New(), signingtest.Issue(t, c.subject), c.rut)
		})
	}
}
