package pa

import (
	"crypto/x509/pkix"
	"testing"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/signingtest"
)

func TestParseCertificate(t *testing.T) {
	t.Run("RUC OID", func(t *testing.T) {
		cert := signingtest.Issue(t, pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{
			signingtest.ExtraName(signing.OIDCommonName, "EMPRESA DE PRUEBA S.A."),
			signingtest.ExtraName(signing.OIDSerialNumber, "8-123-456"),
			signingtest.ExtraName(OIDRUC, " 155600000-2-2015 "),
			signingtest.ExtraName([]int{2, 16, 591, 1, 1, 6, 1}, "86"),
		}})
		signingtest.CheckParse(t, New(), cert, "155600000-2-2015")
	})
	t.Run("without the OID", func(t *testing.T) {
		cert := signingtest.Issue(t, pkix.Name{ExtraNames: []pkix.AttributeTypeAndValue{
			signingtest.ExtraName(signing.OIDSerialNumber, "8-123-456"),
		}})
		signingtest.CheckParse(t, New(), cert, "8123456")
	})
}
