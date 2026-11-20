package br

import (
	"crypto/x509"
	"encoding/asn1"
	"regexp"
	"strings"

	"proxima.la/sello/sign/internal/signing"
)

// ICP-Brasil otherName types of the subjectAltName (DOC-ICP-04).
var (
	OIDCPFHolder      = asn1.ObjectIdentifier{2, 16, 76, 1, 3, 1}
	OIDCNPJ           = asn1.ObjectIdentifier{2, 16, 76, 1, 3, 3}
	OIDResponsibleCPF = asn1.ObjectIdentifier{2, 16, 76, 1, 3, 4}
	oidSubjectAltName = asn1.ObjectIdentifier{2, 5, 29, 17}
)

var (
	cnCNPJ = regexp.MustCompile(`:([0-9]{14})`)
	cnCPF  = regexp.MustCompile(`:([0-9]{11})`)
)

// ParseCertificate reads a PKCS #12 (an e-CNPJ or e-CPF A1) or a PEM key
// with its certificate. The tax id is the CNPJ (from the CN's
// "<NOME>:<CNPJ>", else the 2.16.76.1.3.3 otherName) or, for an e-CPF, the
// CPF (the CN's ":<CPF>", else the 2.16.76.1.3.1 or 2.16.76.1.3.4 otherName).
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := signing.ParseKeyAndCertificate(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := signing.Describe(m.Certificate)
	cnpj, cpf := TaxIDs(m.Certificate)
	info.TaxID = cnpj
	if info.TaxID == "" {
		info.TaxID = cpf
	}
	return m, info, nil
}

// TaxIDs reads the CNPJ and the CPF of an ICP-Brasil certificate, "" for
// the one it does not carry.
func TaxIDs(c *x509.Certificate) (cnpj, cpf string) {
	cn := strings.TrimSpace(signing.SubjectValue(c.Subject, signing.OIDCommonName))
	others := otherNames(c)
	cnpj = notGlued(cnCNPJ, cn)
	if cnpj == "" {
		if d := signing.Digits(others[OIDCNPJ.String()]); len(d) == 14 {
			cnpj = d
		}
	}
	cpf = notGlued(cnCPF, cn)
	for _, oid := range []asn1.ObjectIdentifier{OIDCPFHolder, OIDResponsibleCPF} {
		if cpf != "" {
			break
		}
		// The CPF follows the birth date (ddmmaaaa).
		if d := signing.Digits(others[oid.String()]); len(d) >= 19 {
			cpf = d[8:19]
		}
	}
	return cnpj, cpf
}

// notGlued is the first submatch of re whose match is not followed by
// another digit.
func notGlued(re *regexp.Regexp, s string) string {
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		if m[1] == len(s) || s[m[1]] < '0' || s[m[1]] > '9' {
			return s[m[2]:m[3]]
		}
	}
	return ""
}

// otherNames decodes the otherName entries of the subjectAltName, by OID,
// the first string of each; anything undecodable is skipped.
func otherNames(c *x509.Certificate) map[string]string {
	out := map[string]string{}
	for _, ext := range c.Extensions {
		if !ext.Id.Equal(oidSubjectAltName) {
			continue
		}
		var names []asn1.RawValue
		if _, err := asn1.Unmarshal(ext.Value, &names); err != nil {
			return out
		}
		for _, n := range names {
			if n.Class != asn1.ClassContextSpecific || n.Tag != 0 {
				continue
			}
			var other struct {
				Type  asn1.ObjectIdentifier
				Value asn1.RawValue `asn1:"explicit,tag:0"`
			}
			if _, err := asn1.UnmarshalWithParams(n.FullBytes, &other, "tag:0"); err != nil {
				continue
			}
			if v, ok := firstString(other.Value.Bytes); ok {
				if _, seen := out[other.Type.String()]; !seen {
					out[other.Type.String()] = strings.TrimSpace(v)
				}
			}
		}
	}
	return out
}

// firstString finds the first string in DER, depth first.
func firstString(der []byte) (string, bool) {
	for len(der) > 0 {
		var v asn1.RawValue
		rest, err := asn1.Unmarshal(der, &v)
		if err != nil {
			return "", false
		}
		if v.IsCompound {
			if s, ok := firstString(v.Bytes); ok {
				return s, true
			}
		} else if v.Class == asn1.ClassUniversal {
			switch v.Tag {
			case asn1.TagUTF8String, asn1.TagPrintableString, asn1.TagIA5String, asn1.TagT61String, 26 /* VisibleString */ :
				return string(v.Bytes), true
			case 4: // OCTET STRING, which some issuers use
				return string(v.Bytes), true
			}
		}
		der = rest
	}
	return "", false
}
