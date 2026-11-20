package do

import (
	"crypto/x509/pkix"
	"testing"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestParseCertificate(t *testing.T) {
	t.Run("serialNumber", func(t *testing.T) {
		cert := signingtest.Issue(t, pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{
			signingtest.ExtraName(signing.OIDCommonName, "EMPRESA TEST SRL"),
			signingtest.ExtraName(signing.OIDSerialNumber, "RNC-131-23456-7"),
		}})
		signingtest.CheckParse(t, New(), cert, "131234567")
	})
	t.Run("CN", func(t *testing.T) {
		cert := signingtest.Issue(t, pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{
			signingtest.ExtraName(signing.OIDCommonName, "EMPRESA 101234567 SRL"),
		}})
		signingtest.CheckParse(t, New(), cert, "101234567")
	})
}
