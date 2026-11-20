package gt

import (
	"crypto/x509/pkix"
	"strings"

	"proxima.la/sello/sign/internal/signing/xades"
)

// NIT reads the Guatemalan NIT from a certificate's subject, normalized to
// its bare alphanumeric form (1234567-8 → 12345678, 12345678-k →
// 12345678K): from serialNumber (a NIT-like token, else the whole value),
// then a NIT-like token in the CN, then in the OU; "" when none.
func NIT(subject []pkix.AttributeTypeAndValue) string {
	serial, hasSerial := field(subject, xades.OIDSerialNumber)
	if t := nitToken(serial); t != "" {
		return t
	}
	if hasSerial && serial != "" {
		return normalize(serial)
	}
	cn, _ := field(subject, xades.OIDCommonName)
	if t := nitToken(cn); t != "" {
		return t
	}
	ou, _ := field(subject, xades.OIDOrganizationalUnit)
	return nitToken(ou)
}

func field(subject []pkix.AttributeTypeAndValue, oid []int) (string, bool) {
	for _, atv := range subject {
		if atv.Type.Equal(oid) {
			s, ok := atv.Value.(string)
			return s, ok
		}
	}
	return "", false
}

func normalize(s string) string {
	var b strings.Builder
	for _, c := range s {
		if isAlnum(c) {
			b.WriteRune(c)
		}
	}
	return strings.ToUpper(b.String())
}

func isAlnum(c rune) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// nitToken is the first match of sign-old's
// /(?<![A-Za-z0-9])[0-9][0-9-]{4,12}[0-9kK](?![A-Za-z0-9])/, normalized.
func nitToken(s string) string {
	r := []rune(s)
	for i := range r {
		if r[i] < '0' || r[i] > '9' || (i > 0 && isAlnum(r[i-1])) {
			continue
		}
		for n := 12; n >= 4; n-- {
			end := i + 1 + n // index of the check character
			if end >= len(r) {
				continue
			}
			ok := true
			for _, c := range r[i+1 : end] {
				if c != '-' && (c < '0' || c > '9') {
					ok = false
					break
				}
			}
			last := r[end]
			if !ok || !(last >= '0' && last <= '9' || last == 'k' || last == 'K') {
				continue
			}
			if end+1 < len(r) && isAlnum(r[end+1]) {
				continue
			}
			return normalize(string(r[i : end+1]))
		}
	}
	return ""
}
