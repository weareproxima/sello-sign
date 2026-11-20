package co

import (
	"crypto/x509"
	"regexp"
	"strings"

	"proxima.la/sello/sign/internal/signing/xades"
)

// bareNIT is a subject value that is nothing but the identifier, with an
// optional check digit: 900123456 or 900123456-7.
var bareNIT = regexp.MustCompile(`^([0-9]{6,10})(?:\s*-\s*[0-9])?$`)

// NIT reads the Colombian NIT, without its check digit, as
// extractColombiaCertInfo of sign-old: the subject serialNumber when it is
// nothing but the identifier, else the first NIT-shaped token (8 to 10
// digits not glued to other digits) in serialNumber, OU, O and CN, after
// dropping thousands separators; "" when none carries one.
func NIT(c *x509.Certificate) string {
	serial := strings.TrimSpace(xades.SubjectValue(c, xades.OIDSerialNumber))
	if m := bareNIT.FindStringSubmatch(stripThousands(serial)); m != nil {
		return m[1]
	}
	for _, v := range []string{
		serial,
		xades.SubjectValue(c, xades.OIDOrganizationalUnit),
		xades.SubjectValue(c, xades.OIDOrganization),
		xades.SubjectValue(c, xades.OIDCommonName),
	} {
		if n := nitToken(stripThousands(strings.TrimSpace(v))); n != "" {
			return n
		}
	}
	return ""
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// stripThousands drops a "." between a digit and exactly three digits:
// 900.123.456-7 → 900123456-7.
func stripThousands(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '.' && i > 0 && isDigit(s[i-1]) && i+3 < len(s) &&
			isDigit(s[i+1]) && isDigit(s[i+2]) && isDigit(s[i+3]) && (i+4 == len(s) || !isDigit(s[i+4])) {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// nitToken is the first run of 8 to 10 digits not glued to other digits,
// which is what /(?<![0-9])([0-9]{8,10})(?:\s*-\s*([0-9]))?(?![0-9])/
// captures.
func nitToken(s string) string {
	for i := 0; i < len(s); {
		if !isDigit(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		if n := j - i; n >= 8 && n <= 10 {
			return s[i:j]
		}
		i = j
	}
	return ""
}
