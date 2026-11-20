// Package sv signs El Salvador's DTE for the Ministerio de Hacienda: a JWS
// in compact form with RS512 over the document's JSON, as sign-old's sv.ts
// made it with jsonwebtoken: the header {"alg":"RS512","typ":"JWT"} and, as
// payload, JSON.stringify(JSON.parse(body)). An XML body (which the MH does
// not use) is signed as it is, with the header {"alg":"RS512"} and a random
// document id.
//
// Its certificate is the MH's .crt: an XML (CertificadoMH) with the
// taxpayer's NIT, the validity and the keys, the private one as PKCS #8.
package sv

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"proxima.la/sello/sign/internal/signing"
)

// New returns the country's signer.
func New() signing.Country { return country{} }

type country struct{}

func (country) Code() string { return "SV" }

// Sign signs a DTE.
func (country) Sign(m signing.Material, body []byte, contentType string, o signing.Options) (signing.Signed, error) {
	key, ok := m.Key.(*rsa.PrivateKey)
	if !ok {
		return signing.Signed{}, fmt.Errorf("%w: the key is not RSA", signing.ErrInvalidCertificate)
	}
	if key.N.BitLen() < 2048 {
		// jsonwebtoken refuses RS512 with a smaller key.
		return signing.Signed{}, fmt.Errorf("%w: RS512 needs a key of at least 2048 bits", signing.ErrInvalidCertificate)
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(body) {
		return signing.Signed{}, fmt.Errorf("%w: the body is not UTF-8", signing.ErrInvalidDocument)
	}
	var header, payload string
	var doc signing.Document
	switch contentType {
	case "application/json":
		v, err := parseJSON(string(body))
		if err != nil {
			return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err)
		}
		if v == nil {
			// sv.ts read identificacion of null and failed.
			return signing.Signed{}, fmt.Errorf("%w: the document is null", signing.ErrInvalidDocument)
		}
		var b strings.Builder
		stringify(&b, v)
		header, payload, doc = `{"alg":"RS512","typ":"JWT"}`, b.String(), metadata(v)
	case "application/xml":
		header, payload = `{"alg":"RS512"}`, string(body)
	default:
		return signing.Signed{}, signing.ErrUnsupportedContentType
	}
	var err error
	if doc.ID, err = signing.DocumentIDOr(doc.ID, o); err != nil {
		return signing.Signed{}, err
	}
	input := b64(header) + "." + b64(payload)
	digest := sha512.Sum512([]byte(input))
	sig, err := key.Sign(rand.Reader, digest[:], crypto.SHA512)
	if err != nil {
		return signing.Signed{}, err
	}
	token := input + "." + base64.RawURLEncoding.EncodeToString(sig)
	return signing.Signed{Signature: []byte(token), ContentType: "application/jose", Document: doc}, nil
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// Verify checks an RS512 JWS against the material's public key, as
// jwt.verify with algorithms ['RS512'] did, without its checks of exp and
// nbf, which a DTE does not carry.
func (country) Verify(m signing.Material, signature []byte) (signing.Verification, error) {
	pub, ok := m.PublicKey().(*rsa.PublicKey)
	if !ok {
		return signing.Verification{}, nil
	}
	parts := strings.Split(strings.TrimSpace(string(signature)), ".")
	if len(parts) != 3 {
		return signing.Verification{}, nil
	}
	header, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	payload, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	sig, err3 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return signing.Verification{}, nil
	}
	h, err := parseJSON(string(header))
	obj, ok := h.(*jsonObject)
	if err != nil || !ok {
		return signing.Verification{}, nil
	}
	if alg, _ := obj.get("alg"); alg != "RS512" {
		return signing.Verification{}, nil
	}
	digest := sha512.Sum512([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(pub, crypto.SHA512, digest[:], sig) != nil {
		return signing.Verification{}, nil
	}
	v := signing.Verification{Valid: true, Payload: payload}
	if p, err := parseJSON(string(payload)); err == nil {
		v.Document = metadata(p)
	}
	return v, nil
}

// metadata: identificacion.codigoGeneracion and identificacion.tipoDte,
// when they are non-empty strings.
func metadata(v jsonValue) signing.Document {
	var d signing.Document
	o, ok := v.(*jsonObject)
	if !ok {
		return d
	}
	id, _ := o.get("identificacion")
	ident, ok := id.(*jsonObject)
	if !ok {
		return d
	}
	if s, ok := ident.values["codigoGeneracion"].(string); ok {
		d.ID = s
	}
	if s, ok := ident.values["tipoDte"].(string); ok {
		d.Type = s
	}
	return d
}

// ParseCertificate reads the MH's .crt.
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	content := string(u.Data)
	if !strings.Contains(content, "<CertificadoMH>") || !strings.Contains(content, "<nit>") {
		return signing.Material{}, signing.CertificateInfo{}, fmt.Errorf("%w: not an MH .crt (CertificadoMH)", signing.ErrInvalidCertificate)
	}
	fail := func(what string) (signing.Material, signing.CertificateInfo, error) {
		return signing.Material{}, signing.CertificateInfo{}, fmt.Errorf("%w: %s", signing.ErrInvalidCertificate, what)
	}
	nit := tag(content, "nit")
	if nit == "" {
		return fail("missing <nit>")
	}
	notBefore, err1 := seconds(tag(content, "notBefore"))
	notAfter, err2 := seconds(tag(content, "notAfter"))
	if err1 != nil || err2 != nil {
		return fail("missing or invalid validity dates")
	}
	block := privateKeyBlock.FindStringSubmatch(content)
	if block == nil {
		return fail("missing <privateKey>")
	}
	encoded := tag(block[1], "encodied")
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(encoded), ""))
	if encoded == "" || err != nil {
		return fail("missing or invalid private key data")
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return fail("the private key is not PKCS #8: " + err.Error())
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return fail("the private key cannot sign")
	}
	info := signing.CertificateInfo{
		TaxID:     nit,
		Subject:   names(block2(content, "subject"), "commonName", "givenName", "surname", "organizationName", "organizationalName"),
		Issuer:    names(block2(content, "issuer"), "commonName", "organizationalName", "organizationName"),
		Serial:    serial(tag(content, "serial")),
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}
	if info.Subject == "" {
		// sv.ts's own reading, without the <subject> block.
		info.Subject = names(content, "givenName", "surname", "organizationName", "organizationalName")
	}
	if info.Issuer == "" {
		info.Issuer = "Ministerio de Hacienda"
	}
	return signing.Material{Key: signer}, info, nil
}

var privateKeyBlock = regexp.MustCompile(`<privateKey>([\s\S]*?)</privateKey>`)

// block2 is what is inside the first <name>…</name> of xml, or "".
func block2(xml, name string) string {
	m := regexp.MustCompile(`<` + name + `>([\s\S]*?)</` + name + `>`).FindStringSubmatch(xml)
	if m == nil {
		return ""
	}
	return m[1]
}

// names joins the distinct values of the tags of xml, in that order.
func names(xml string, tags ...string) string {
	var out []string
	for _, t := range tags {
		if v := tag(xml, t); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return strings.Join(out, ", ")
}

// serial writes the .crt's decimal serial in upper-case hexadecimal, as
// signing.Describe does for an X.509 certificate.
func serial(s string) string {
	if n, ok := new(big.Int).SetString(s, 10); ok {
		return strings.ToUpper(n.Text(16))
	}
	return s
}

// tag is the trimmed text of the first <name>…</name> without markup
// inside, as sv.ts's extractTag read it.
func tag(xml, name string) string {
	m := regexp.MustCompile(`<` + regexp.QuoteMeta(name) + `>([^<]*)</` + regexp.QuoteMeta(name) + `>`).FindStringSubmatch(xml)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func seconds(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(int64(f * 1000)).UTC(), nil
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
