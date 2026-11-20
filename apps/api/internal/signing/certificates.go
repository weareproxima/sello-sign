package signing

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// ParseKeyAndCertificate reads what the PKCS #12 countries accept: a
// PKCS #12 file (.p12/.pfx) opened with u.Password, or a PEM bundle with a
// private key (PKCS #8, PKCS #1 or SEC 1) and certificates. The signer's
// certificate is the one whose public key is the key's; the rest are its
// chain. Errors wrap ErrInvalidCertificate.
func ParseKeyAndCertificate(u Upload) (Material, error) {
	var key any
	var certs []*x509.Certificate
	if bytes.Contains(u.Data, []byte("-----BEGIN")) {
		rest := u.Data
		for {
			var block *pem.Block
			block, rest = pem.Decode(rest)
			if block == nil {
				break
			}
			switch {
			case block.Type == "CERTIFICATE":
				c, err := x509.ParseCertificate(block.Bytes)
				if err != nil {
					return Material{}, fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
				}
				certs = append(certs, c)
			case strings.HasSuffix(block.Type, "PRIVATE KEY"):
				if key != nil {
					return Material{}, fmt.Errorf("%w: more than one private key", ErrInvalidCertificate)
				}
				k, err := parsePrivateKey(block.Bytes)
				if err != nil {
					return Material{}, fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
				}
				key = k
			}
		}
	} else {
		k, leaf, chain, err := pkcs12.DecodeChain(u.Data, u.Password)
		if err != nil {
			return Material{}, fmt.Errorf("%w: %v", ErrInvalidCertificate, err)
		}
		key, certs = k, append([]*x509.Certificate{leaf}, chain...)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return Material{}, fmt.Errorf("%w: no private key", ErrInvalidCertificate)
	}
	m := Material{Key: signer}
	pub, ok := signer.Public().(interface{ Equal(crypto.PublicKey) bool })
	for _, c := range certs {
		if m.Certificate == nil && ok && pub.Equal(c.PublicKey) {
			m.Certificate = c
		} else {
			m.Chain = append(m.Chain, c)
		}
	}
	if m.Certificate == nil {
		return Material{}, fmt.Errorf("%w: no certificate matches the private key", ErrInvalidCertificate)
	}
	return m, nil
}

func parsePrivateKey(der []byte) (any, error) {
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k, nil
	}
	return x509.ParseECPrivateKey(der)
}

// Describe is what a certificate says of itself; the country fills TaxID.
// Serial is upper-case hexadecimal.
func Describe(c *x509.Certificate) CertificateInfo {
	return CertificateInfo{
		Subject:   c.Subject.String(),
		Issuer:    c.Issuer.String(),
		Serial:    strings.ToUpper(c.SerialNumber.Text(16)),
		NotBefore: c.NotBefore,
		NotAfter:  c.NotAfter,
	}
}

// Well-known subject attributes.
var (
	OIDCommonName             = asn1.ObjectIdentifier{2, 5, 4, 3}
	OIDSerialNumber           = asn1.ObjectIdentifier{2, 5, 4, 5}
	OIDOrganization           = asn1.ObjectIdentifier{2, 5, 4, 10}
	OIDOrganizationalUnit     = asn1.ObjectIdentifier{2, 5, 4, 11}
	OIDOrganizationIdentifier = asn1.ObjectIdentifier{2, 5, 4, 97}
)

// SubjectValues are the string values of name's attributes of type oid, in
// the certificate's order.
func SubjectValues(name pkix.Name, oid asn1.ObjectIdentifier) []string {
	var out []string
	for _, a := range name.Names {
		if a.Type.Equal(oid) {
			if s, ok := a.Value.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// SubjectValue is the first of SubjectValues, or "".
func SubjectValue(name pkix.Name, oid asn1.ObjectIdentifier) string {
	if v := SubjectValues(name, oid); len(v) > 0 {
		return v[0]
	}
	return ""
}

// Time is o.Now, or the current time.
func (o Options) Time() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// Random is o.Rand, or crypto/rand.
func (o Options) Random() io.Reader {
	if o.Rand == nil {
		return rand.Reader
	}
	return o.Rand
}

// NewUUID reads a version 4 UUID from r, in lower case, as
// crypto.randomUUID() writes it: 16 bytes with b[6] = b[6]&0x0f|0x40 and
// b[8] = b[8]&0x3f|0x80.
func NewUUID(r io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// DocumentIDOr is id, or the upper-case random UUID the TypeScript signers
// fell back to when the document has none.
func DocumentIDOr(id string, o Options) (string, error) {
	if id != "" {
		return id, nil
	}
	u, err := NewUUID(o.Random())
	if err != nil {
		return "", err
	}
	return strings.ToUpper(u), nil
}

// PublicKey is the material's public key: its certificate's, or its key's.
func (m Material) PublicKey() crypto.PublicKey {
	if m.Certificate != nil {
		return m.Certificate.PublicKey
	}
	if m.Key != nil {
		return m.Key.Public()
	}
	return nil
}

// SerialOrCNTaxID is how sign-old read the tax id of a PKCS #12 country
// without a rule of its own: the digits of the subject's first
// serialNumber, else the first run of 9 to 11 digits of its first CN.
func SerialOrCNTaxID(subject pkix.Name) string {
	if serial := SubjectValue(subject, OIDSerialNumber); serial != "" {
		if digits := Digits(serial); digits != "" {
			return digits
		}
	}
	return cnDigits.FindString(SubjectValue(subject, OIDCommonName))
}

var cnDigits = regexp.MustCompile(`[0-9]{9,11}`)

// Digits keeps the ASCII digits of s.
func Digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
