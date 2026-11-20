package pe

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
		ruc     string
	}{
		"RENIEC CDT, RUC in the second OU, DNI in serialNumber": {name(
			signingtest.ExtraName(signing.OIDCommonName, "||USO TRIBUTARIO|| EMPRESA DE PRUEBA CDT 20123456786"),
			signingtest.ExtraName(signing.OIDSerialNumber, "10456789"),
			signingtest.ExtraName(signing.OIDOrganizationalUnit, "EREP_SUNAT_2026"),
			signingtest.ExtraName(signing.OIDOrganizationalUnit, "20123456786"),
		), "20123456786"},
		"Llama.pe": {name(
			signingtest.ExtraName(signing.OIDOrganizationalUnit, "FACTURA ELECTRONICA RUC: 20123456786"),
		), "20123456786"},
		"the valid check digit wins": {name(
			signingtest.ExtraName(signing.OIDOrganizationalUnit, "20123456780"),
			signingtest.ExtraName(signing.OIDCommonName, "X 20123456786"),
		), "20123456786"},
		"organizationIdentifier": {name(
			signingtest.ExtraName(signing.OIDOrganizationIdentifier, "NTRPE-20123456786"),
		), "20123456786"},
		"glued to other digits": {name(
			signingtest.ExtraName(signing.OIDOrganizationalUnit, "120123456786"),
		), ""},
	}
	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			signingtest.CheckParse(t, New(), signingtest.Issue(t, c.subject), c.ruc)
		})
	}
}

func TestValidRUC(t *testing.T) {
	for s, want := range map[string]bool{"20123456786": true, "20123456780": false, "30123456786": false, "2012345678": false} {
		if ValidRUC(s) != want {
			t.Errorf("ValidRUC(%s) = %v", s, !want)
		}
	}
}
