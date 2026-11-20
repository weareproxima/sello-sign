// Package xades builds the XAdES (ETSI TS 101 903 v1.3.2) signatures of the
// countries that sign with it (Costa Rica, Guatemala, Ecuador, Colombia), as
// the TypeScript signers of ../sign-old wrote them, byte for byte: their
// QualifyingProperties and SignedProperties (SigningTime, SigningCertificate
// with CertDigest and IssuerSerial, an optional SignaturePolicyIdentifier
// for EPES, SignerRole and DataObjectFormat) and the certificate fields
// they carry. The documents themselves are read, searched and written with the
// xmldsig package's DOM, the one every country shares.
//
// Each country's package assembles its ds:Signature as a string template,
// as its TypeScript signer did; this package gives them the pieces, which
// escape every value they write (xmldsig.EscapeText and EscapeAttr).
package xades

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

// Namespaces.
const (
	DSigNS     = xmldsig.NS
	XAdESNS    = "http://uri.etsi.org/01903/v1.3.2#"
	XAdES141NS = "http://uri.etsi.org/01903/v1.4.1#"
)

// Algorithm and type URIs.
const (
	SignedPropertiesType = "http://uri.etsi.org/01903#SignedProperties"
	EnvelopedSignature   = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"
	XPathFilter          = "http://www.w3.org/TR/1999/REC-xpath-19991116"
	SHA1                 = "http://www.w3.org/2000/09/xmldsig#sha1"
	SHA256               = "http://www.w3.org/2001/04/xmlenc#sha256"
	RSASHA1              = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"
	RSASHA256            = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
)

// HashOf maps a digest URI to its hash.
func HashOf(uri string) (crypto.Hash, bool) {
	switch uri {
	case SHA1, RSASHA1:
		return crypto.SHA1, true
	case SHA256, RSASHA256:
		return crypto.SHA256, true
	}
	return 0, false
}

// Policy is an EPES SignaturePolicyIdentifier.
type Policy struct {
	Identifier string
	// Description is optional.
	Description     string
	DigestAlgorithm string
	Digest          string
}

// DataObjectFormat describes the signed document.
type DataObjectFormat struct {
	ObjectReference string
	// Description is optional.
	Description string
	MimeType    string
}

// SignedProperties is xades:SignedProperties as each country writes it.
type SignedProperties struct {
	// Prefix is the XAdES prefix: "xades", or "etsi" in Ecuador.
	Prefix string
	// Declarations are written verbatim before Id (Costa Rica declares
	// xades and ds on SignedProperties itself), e.g.
	// ` xmlns:xades="…" xmlns:ds="…"`.
	Declarations string
	ID           string
	SigningTime  string
	// DigestAlgorithm is the CertDigest's.
	DigestAlgorithm string
	Certificate     Certificate
	Policy          *Policy
	// ClaimedRole is SignerRole/ClaimedRoles/ClaimedRole, when not empty.
	ClaimedRole      string
	DataObjectFormat *DataObjectFormat
	// EmptyElements writes <x></x> instead of <x/> for the empty
	// ds:DigestMethod, as Ecuador's signer did.
	EmptyElements bool
}

// XML renders the properties.
func (p SignedProperties) XML() (string, error) {
	h, ok := HashOf(p.DigestAlgorithm)
	if !ok {
		return "", errors.New("xades: unknown digest algorithm")
	}
	x := p.Prefix
	var b strings.Builder
	w := func(s ...string) {
		for _, v := range s {
			b.WriteString(v)
		}
	}
	digestMethod := func(alg string) {
		if p.EmptyElements {
			w(`<ds:DigestMethod Algorithm="`, xmldsig.EscapeAttr(alg), `"></ds:DigestMethod>`)
		} else {
			w(`<ds:DigestMethod Algorithm="`, xmldsig.EscapeAttr(alg), `"/>`)
		}
	}
	w(`<`, x, `:SignedProperties`, p.Declarations, ` Id="`, xmldsig.EscapeAttr(p.ID), `">`)
	w(`<`, x, `:SignedSignatureProperties>`)
	w(`<`, x, `:SigningTime>`, xmldsig.EscapeText(p.SigningTime), `</`, x, `:SigningTime>`)
	w(`<`, x, `:SigningCertificate>`, `<`, x, `:Cert>`, `<`, x, `:CertDigest>`)
	digestMethod(p.DigestAlgorithm)
	w(`<ds:DigestValue>`, p.Certificate.CertDigest(h), `</ds:DigestValue>`)
	w(`</`, x, `:CertDigest>`, `<`, x, `:IssuerSerial>`)
	w(`<ds:X509IssuerName>`, xmldsig.EscapeText(p.Certificate.IssuerName), `</ds:X509IssuerName>`)
	w(`<ds:X509SerialNumber>`, p.Certificate.Serial, `</ds:X509SerialNumber>`)
	w(`</`, x, `:IssuerSerial>`, `</`, x, `:Cert>`, `</`, x, `:SigningCertificate>`)
	if pol := p.Policy; pol != nil {
		w(`<`, x, `:SignaturePolicyIdentifier>`, `<`, x, `:SignaturePolicyId>`, `<`, x, `:SigPolicyId>`)
		w(`<`, x, `:Identifier>`, xmldsig.EscapeText(pol.Identifier), `</`, x, `:Identifier>`)
		if pol.Description != "" {
			w(`<`, x, `:Description>`, xmldsig.EscapeText(pol.Description), `</`, x, `:Description>`)
		}
		w(`</`, x, `:SigPolicyId>`, `<`, x, `:SigPolicyHash>`)
		digestMethod(pol.DigestAlgorithm)
		w(`<ds:DigestValue>`, xmldsig.EscapeText(pol.Digest), `</ds:DigestValue>`)
		w(`</`, x, `:SigPolicyHash>`, `</`, x, `:SignaturePolicyId>`, `</`, x, `:SignaturePolicyIdentifier>`)
	}
	if p.ClaimedRole != "" {
		w(`<`, x, `:SignerRole>`, `<`, x, `:ClaimedRoles>`)
		w(`<`, x, `:ClaimedRole>`, xmldsig.EscapeText(p.ClaimedRole), `</`, x, `:ClaimedRole>`)
		w(`</`, x, `:ClaimedRoles>`, `</`, x, `:SignerRole>`)
	}
	w(`</`, x, `:SignedSignatureProperties>`)
	if f := p.DataObjectFormat; f != nil {
		w(`<`, x, `:SignedDataObjectProperties>`)
		w(`<`, x, `:DataObjectFormat ObjectReference="`, xmldsig.EscapeAttr(f.ObjectReference), `">`)
		if f.Description != "" {
			w(`<`, x, `:Description>`, xmldsig.EscapeText(f.Description), `</`, x, `:Description>`)
		}
		w(`<`, x, `:MimeType>`, xmldsig.EscapeText(f.MimeType), `</`, x, `:MimeType>`)
		w(`</`, x, `:DataObjectFormat>`, `</`, x, `:SignedDataObjectProperties>`)
	}
	w(`</`, x, `:SignedProperties>`)
	return b.String(), nil
}

// ── Time and randomness ────────────────────────────────────────────────────

// Now is the signing time of o.
func Now(o signing.Options) time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// UTCSeconds formats t as JavaScript's toISOString() without milliseconds:
// 2026-11-20T03:19:00Z.
func UTCSeconds(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// Offset formats t in a fixed offset east of UTC, with or without
// milliseconds: 2026-09-02T09:23:12-05:00, 2015-09-04T20:06:27.100-05:00.
func Offset(t time.Time, hours int, millis bool) string {
	z := time.FixedZone("", hours*3600)
	layout := "2006-01-02T15:04:05-07:00"
	if millis {
		layout = "2006-01-02T15:04:05.000-07:00"
	}
	return t.In(z).Format(layout)
}

// Random reads o.Rand, or crypto/rand when it's nil.
func Random(o signing.Options) io.Reader {
	if o.Rand == nil {
		return rand.Reader
	}
	return o.Rand
}

// UUID is crypto.randomUUID(): a version 4 UUID from 16 random bytes, in
// lower-case hex.
func UUID(r io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// RandomInt is Node's crypto.randomInt(min, max): 48-bit draws with
// rejection, so the same bytes give the same numbers.
func RandomInt(r io.Reader, min, max int64) (int64, error) {
	const randMax = 1<<48 - 1
	span := max - min
	if span <= 0 || span > randMax {
		return 0, errors.New("xades: bad range")
	}
	limit := randMax - randMax%span
	for {
		var b [8]byte
		if _, err := io.ReadFull(r, b[2:]); err != nil {
			return 0, err
		}
		x := int64(binary.BigEndian.Uint64(b[:]))
		if x < limit {
			return min + x%span, nil
		}
	}
}

// ── Canonicalization, digests and RSA ──────────────────────────────────────

// Digest is the base64 digest of el's canonical form, in its document.
func Digest(h crypto.Hash, el *etree.Element, o c14n.Options) string {
	return HashBase64(h, c14n.Canonicalize(el, o))
}

// HashBase64 is the base64 digest of data.
func HashBase64(h crypto.Hash, data []byte) string {
	d := h.New()
	d.Write(data)
	return base64.StdEncoding.EncodeToString(d.Sum(nil))
}

// SignRSA signs data with PKCS#1 v1.5, in base64.
func SignRSA(key crypto.Signer, h crypto.Hash, data []byte) (string, error) {
	if _, ok := key.Public().(*rsa.PublicKey); !ok {
		return "", errors.New("xades: the key is not RSA")
	}
	d := h.New()
	d.Write(data)
	sig, err := key.Sign(rand.Reader, d.Sum(nil), h)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyRSA checks a base64 PKCS#1 v1.5 signature. Line breaks and spaces
// in value are ignored.
func VerifyRSA(pub crypto.PublicKey, h crypto.Hash, data []byte, value string) bool {
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
	if err != nil {
		return false
	}
	d := h.New()
	d.Write(data)
	return rsa.VerifyPKCS1v15(rsaPub, h, d.Sum(nil), sig) == nil
}
