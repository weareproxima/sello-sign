package pe

import (
	"crypto/x509/pkix"
	"regexp"
	"strings"

	"proxima.la/sello/sign/internal/signing"
)

// rucToken is eleven digits opening with a taxpayer-type prefix, not glued
// to other digits (Go's regexp has no lookbehind: the edges are checked by
// hand).
var rucToken = regexp.MustCompile(`(?:10|15|16|17|20)[0-9]{9}`)

var rucWeights = [10]int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}

// ValidRUC says whether s is eleven digits with a valid prefix and módulo 11
// check digit.
func ValidRUC(s string) bool {
	if len(s) != 11 || !rucToken.MatchString(s) {
		return false
	}
	sum := 0
	for i, w := range rucWeights {
		sum += int(s[i]-'0') * w
	}
	check := 11 - sum%11
	switch check {
	case 10:
		check = 0
	case 11:
		check = 1
	}
	return int(s[10]-'0') == check
}

func rucTokens(values []string) []string {
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		for i := 0; i+11 <= len(v); i++ {
			m := rucToken.FindStringIndex(v[i:])
			if m == nil {
				break
			}
			start, end := i+m[0], i+m[1]
			if (start > 0 && isDigit(v[start-1])) || (end < len(v) && isDigit(v[end])) {
				i = start
				continue
			}
			out = append(out, v[start:end])
			i = end - 1
		}
	}
	return out
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// RUC reads the taxpayer's RUC from a certificate subject as sign-old's
// extractPeruCertInfo did.
func RUC(subject pkix.Name) string {
	candidates := rucTokens(signing.SubjectValues(subject, signing.OIDOrganizationalUnit))
	candidates = append(candidates, rucTokens(signing.SubjectValues(subject, signing.OIDCommonName))...)
	candidates = append(candidates, rucTokens(signing.SubjectValues(subject, signing.OIDOrganizationIdentifier))...)
	for _, c := range candidates {
		if ValidRUC(c) {
			return c
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}
