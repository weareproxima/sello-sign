package xades

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/crypto/cryptobyte"
	cbasn1 "golang.org/x/crypto/cryptobyte/asn1"
	"software.sslmate.com/src/go-pkcs12"

	"proxima.la/sello/sign/internal/signing"
)

// Certificate is what a XAdES signature says of its signer's certificate.
type Certificate struct {
	// Base64 is the DER in one line of standard base64 (ds:X509Certificate).
	Base64 string
	// DER is the certificate itself; CertDigest hashes it.
	DER []byte
	// IssuerName is the ds:X509IssuerName the TypeScript signers wrote.
	IssuerName string
	// Serial is the serial number in decimal (ds:X509SerialNumber).
	Serial string
}

// CertificateOf reads what the XAdES properties need from a certificate.
func CertificateOf(c *x509.Certificate) (Certificate, error) {
	if c == nil {
		return Certificate{}, errors.New("xades: XAdES needs the signer's certificate")
	}
	issuer, err := IssuerName(c.RawIssuer)
	if err != nil {
		return Certificate{}, err
	}
	serial, err := rawSerial(c.RawTBSCertificate)
	if err != nil {
		return Certificate{}, err
	}
	return Certificate{
		Base64:     base64.StdEncoding.EncodeToString(c.Raw),
		DER:        c.Raw,
		IssuerName: issuer,
		Serial:     serial,
	}, nil
}

// CertDigest is the base64 digest of the certificate's DER.
func (c Certificate) CertDigest(h crypto.Hash) string {
	d := h.New()
	d.Write(c.DER)
	return base64.StdEncoding.EncodeToString(d.Sum(nil))
}

// rawSerial reads the serial's content octets as an unsigned number, as
// node-forge did (BigInt('0x' + cert.serialNumber)).
func rawSerial(tbs []byte) (string, error) {
	in := cryptobyte.String(tbs)
	var body cryptobyte.String
	if !in.ReadASN1(&body, cbasn1.SEQUENCE) {
		return "", errors.New("xades: malformed certificate")
	}
	// The optional [0] version comes first.
	if !body.SkipOptionalASN1(cbasn1.Tag(0).Constructed().ContextSpecific()) {
		return "", errors.New("xades: malformed certificate version")
	}
	var serial cryptobyte.String
	if !body.ReadASN1(&serial, cbasn1.INTEGER) {
		return "", errors.New("xades: malformed certificate serial")
	}
	return new(big.Int).SetBytes(serial).String(), nil
}

// IssuerName builds the X509IssuerName exactly as the TypeScript signers
// did with node-forge: every attribute of the DN, flattened in order and
// then reversed, written as name=value and joined by "," with no space and
// no RFC 4514 escaping. The name is node-forge's short name (CN, C, L, ST,
// O, OU, E), else its long name (serialNumber, givenName…), else the dotted
// OID. The value is decoded by its ASN.1 string type. node-forge read every
// value byte by byte, so a UTF-8 name such as "Cía" came out as "CÃ­a"; that
// was a bug and is not kept (decided on 2026-11-19): ASCII names, the ones
// the authorities' CAs use, come out byte for byte as before.
func IssuerName(rawDN []byte) (string, error) {
	var rdns pkix.RDNSequence
	rest, err := asn1.Unmarshal(rawDN, &rdns)
	if err != nil || len(rest) > 0 {
		return "", fmt.Errorf("xades: malformed issuer name: %v", err)
	}
	// encoding/asn1 decodes the values; forge kept the octets. Read them
	// again, raw.
	raw, err := rawAttributeValues(rawDN)
	if err != nil {
		return "", err
	}
	var parts []string
	i := 0
	for _, set := range rdns {
		for _, atv := range set {
			parts = append(parts, forgeName(atv.Type)+"="+raw[i].text())
			i++
		}
	}
	for l, r := 0, len(parts)-1; l < r; l, r = l+1, r-1 {
		parts[l], parts[r] = parts[r], parts[l]
	}
	return strings.Join(parts, ","), nil
}

// attributeValue is an attribute's value as the certificate encodes it.
type attributeValue struct {
	tag   cbasn1.Tag
	bytes []byte
}

// ASN.1 string tags that cryptobyte/asn1 doesn't name.
const (
	universalStringTag = cbasn1.Tag(28)
	bmpStringTag       = cbasn1.Tag(30)
)

// text decodes the value by its string type: UTF8String as UTF-8, BMPString
// as UTF-16 and UniversalString as UTF-32 (big-endian both), and the rest
// (PrintableString, IA5String, TeletexString…) one character per byte.
func (v attributeValue) text() string {
	switch v.tag {
	case cbasn1.UTF8String:
		if utf8.Valid(v.bytes) {
			return string(v.bytes)
		}
	case bmpStringTag:
		if len(v.bytes)%2 == 0 {
			units := make([]uint16, len(v.bytes)/2)
			for i := range units {
				units[i] = uint16(v.bytes[2*i])<<8 | uint16(v.bytes[2*i+1])
			}
			return string(utf16.Decode(units))
		}
	case universalStringTag:
		if len(v.bytes)%4 == 0 {
			r := make([]rune, len(v.bytes)/4)
			for i := range r {
				b := v.bytes[4*i:]
				r[i] = rune(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]))
			}
			return string(r)
		}
	}
	return latin1(v.bytes)
}

func rawAttributeValues(rawDN []byte) ([]attributeValue, error) {
	bad := errors.New("xades: malformed issuer name")
	in := cryptobyte.String(rawDN)
	var seq cryptobyte.String
	if !in.ReadASN1(&seq, cbasn1.SEQUENCE) {
		return nil, bad
	}
	var out []attributeValue
	for !seq.Empty() {
		var set cryptobyte.String
		if !seq.ReadASN1(&set, cbasn1.SET) {
			return nil, bad
		}
		for !set.Empty() {
			var atv cryptobyte.String
			var oid cryptobyte.String
			var value cryptobyte.String
			var tag cbasn1.Tag
			if !set.ReadASN1(&atv, cbasn1.SEQUENCE) || !atv.ReadASN1(&oid, cbasn1.OBJECT_IDENTIFIER) ||
				!atv.ReadAnyASN1(&value, &tag) {
				return nil, bad
			}
			out = append(out, attributeValue{tag: tag, bytes: value})
		}
	}
	return out, nil
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

var forgeShortNames = map[string]string{
	"commonName":             "CN",
	"countryName":            "C",
	"localityName":           "L",
	"stateOrProvinceName":    "ST",
	"organizationName":       "O",
	"organizationalUnitName": "OU",
	"emailAddress":           "E",
}

func forgeName(oid asn1.ObjectIdentifier) string {
	s := oid.String()
	name, ok := forgeOIDs[s]
	if !ok {
		return s
	}
	if short, ok := forgeShortNames[name]; ok {
		return short
	}
	return name
}

// RSAKeyValue is ds:RSAKeyValue's Modulus and Exponent as CryptoBinary:
// the minimal big-endian octets, in standard base64.
func RSAKeyValue(c *x509.Certificate) (modulus, exponent string, err error) {
	pub, ok := c.PublicKey.(*rsa.PublicKey)
	if !ok {
		return "", "", errors.New("xades: the certificate's key is not RSA")
	}
	e := big.NewInt(int64(pub.E))
	return base64.StdEncoding.EncodeToString(pub.N.Bytes()), base64.StdEncoding.EncodeToString(e.Bytes()), nil
}

// Wrap76 cuts a base64 string into lines of 76, joined by "\n", as
// Ecuador's signer did.
func Wrap76(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76])
		b.WriteByte('\n')
		s = s[76:]
	}
	b.WriteString(s)
	return b.String()
}

// ReadUpload reads a certificate upload: a PKCS#12 with its PIN or a PEM
// bundle with the certificate and its private key. Every XAdES country
// gets .p12 files from its authority; PEM is there for self-hosting and
// tests.
func ReadUpload(u signing.Upload) (signing.Material, error) {
	if len(u.Data) == 0 {
		return signing.Material{}, fmt.Errorf("%w: empty file", signing.ErrInvalidCertificate)
	}
	if strings.Contains(string(u.Data), "-----BEGIN") {
		m, err := readPEM(u.Data)
		if err != nil {
			return signing.Material{}, fmt.Errorf("%w: %v", signing.ErrInvalidCertificate, err)
		}
		return m, nil
	}
	key, leaf, chain, err := pkcs12.DecodeChain(u.Data, u.Password)
	if err != nil {
		return signing.Material{}, fmt.Errorf("%w: %v", signing.ErrInvalidCertificate, err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return signing.Material{}, fmt.Errorf("%w: the key cannot sign", signing.ErrInvalidCertificate)
	}
	return pickLeaf(signer, append([]*x509.Certificate{leaf}, chain...))
}

func readPEM(data []byte) (signing.Material, error) {
	var key crypto.Signer
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			c, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return signing.Material{}, err
			}
			certs = append(certs, c)
		case "PRIVATE KEY", "RSA PRIVATE KEY":
			var k any
			var err error
			if block.Type == "PRIVATE KEY" {
				k, err = x509.ParsePKCS8PrivateKey(block.Bytes)
			} else {
				k, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			}
			if err != nil {
				return signing.Material{}, err
			}
			s, ok := k.(crypto.Signer)
			if !ok {
				return signing.Material{}, errors.New("the key cannot sign")
			}
			key = s
		}
	}
	if key == nil {
		return signing.Material{}, errors.New("no private key in the PEM")
	}
	return pickLeaf(key, certs)
}

// pickLeaf takes as the signer's certificate the one whose public key is
// the key's, and keeps the rest as its chain.
func pickLeaf(key crypto.Signer, certs []*x509.Certificate) (signing.Material, error) {
	type equaler interface{ Equal(crypto.PublicKey) bool }
	pub, ok := key.Public().(equaler)
	if !ok {
		return signing.Material{}, fmt.Errorf("%w: unsupported key", signing.ErrInvalidCertificate)
	}
	m := signing.Material{Key: key}
	for _, c := range certs {
		if c == nil {
			continue
		}
		if m.Certificate == nil && pub.Equal(c.PublicKey) {
			m.Certificate = c
			continue
		}
		m.Chain = append(m.Chain, c)
	}
	if m.Certificate == nil {
		return signing.Material{}, fmt.Errorf("%w: no certificate for the private key", signing.ErrInvalidCertificate)
	}
	if _, ok := key.(*rsa.PrivateKey); !ok {
		return signing.Material{}, fmt.Errorf("%w: the key is not RSA", signing.ErrInvalidCertificate)
	}
	return m, nil
}

// Info is the CertificateInfo of an upload, without the tax id, which
// each country reads its own way.
func Info(c *x509.Certificate) signing.CertificateInfo {
	return signing.CertificateInfo{
		Subject:   c.Subject.String(),
		Issuer:    c.Issuer.String(),
		Serial:    c.SerialNumber.String(),
		NotBefore: c.NotBefore,
		NotAfter:  c.NotAfter,
	}
}

// SubjectValue is the first value of a subject attribute, as text.
func SubjectValue(c *x509.Certificate, oid asn1.ObjectIdentifier) string {
	for _, atv := range c.Subject.Names {
		if atv.Type.Equal(oid) {
			switch v := atv.Value.(type) {
			case string:
				return v
			case []byte:
				return string(v)
			}
		}
	}
	return ""
}

// Subject attribute OIDs the countries read.
var (
	OIDCommonName         = asn1.ObjectIdentifier{2, 5, 4, 3}
	OIDSerialNumber       = asn1.ObjectIdentifier{2, 5, 4, 5}
	OIDOrganization       = asn1.ObjectIdentifier{2, 5, 4, 10}
	OIDOrganizationalUnit = asn1.ObjectIdentifier{2, 5, 4, 11}
)

var nonDigits = regexp.MustCompile(`\D`)
var digitRun = regexp.MustCompile(`\d{9,11}`)

// GenericTaxID is how the TypeScript upload read the tax id of a PKCS#12
// country without its own rule (Costa Rica, Ecuador): the digits of the
// subject serialNumber, else a run of 9 to 11 digits in the CN.
func GenericTaxID(c *x509.Certificate) string {
	if id := nonDigits.ReplaceAllString(SubjectValue(c, OIDSerialNumber), ""); id != "" {
		return id
	}
	return digitRun.FindString(SubjectValue(c, OIDCommonName))
}
