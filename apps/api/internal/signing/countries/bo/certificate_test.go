package bo

import (
	"crypto/x509/pkix"
	"testing"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestParseCertificate(t *testing.T) {
	// The SIN's CSR specification: serialNumber "Número de NIT {numeral}".
	cert := signingtest.Issue(t, pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{
		signingtest.ExtraName(signing.OIDCommonName, "JUAN PEREZ"),
		signingtest.ExtraName(signing.OIDSerialNumber, "Número de NIT 1023456028"),
	}})
	signingtest.CheckParse(t, New(), cert, "1023456028")
}
