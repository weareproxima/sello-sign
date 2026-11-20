// Package signing is what every country's signer has in common: what a
// certificate yields, how a document is signed and verified, and the
// registry the API looks a country up in. It names no country: each one
// lives in its own package under countries/.
package signing

import (
	"crypto"
	"crypto/x509"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotImplemented is what a country without its signer yet answers.
	ErrNotImplemented = errors.New("signing: country not implemented")
	// ErrUnsupportedContentType: the country doesn't sign that media type.
	ErrUnsupportedContentType = errors.New("signing: unsupported content type")
	// ErrInvalidDocument: the body isn't a document the country can sign.
	ErrInvalidDocument = errors.New("signing: invalid document")
	// ErrInvalidCertificate: the upload isn't a certificate the country reads.
	ErrInvalidCertificate = errors.New("signing: invalid certificate")
)

// Upload is a certificate file as the client sends it: a PEM, a DER, El
// Salvador's .crt from the MH (an XML with the keys), or a PKCS#12 with
// its PIN.
type Upload struct {
	Data     []byte
	Password string
}

// Material is what a certificate yields for signing. The API keeps it
// encrypted (EncodeMaterial) and decodes it in memory to sign.
type Material struct {
	// Key signs. Every country uses RSA today.
	Key crypto.Signer
	// Certificate is the signer's X.509 certificate; nil when the format
	// brings only the keys (El Salvador's .crt).
	Certificate *x509.Certificate
	// Chain holds the intermediates the file brought, if any.
	Chain []*x509.Certificate
}

// CertificateInfo is what the API stores and the panel shows of an upload.
type CertificateInfo struct {
	// TaxID is the taxpayer id the certificate names, when the country puts
	// one there; the API checks it against the organization's.
	TaxID     string
	Subject   string
	Issuer    string
	Serial    string
	NotBefore time.Time
	// NotAfter is zero when the format has no expiry.
	NotAfter time.Time
}

// Document identifies what was signed: codigoGeneracion and tipoDte in El
// Salvador, eNCF and TipoeCF in the Dominican Republic, Clave in Costa
// Rica…
type Document struct {
	ID   string
	Type string
}

// Signed is a signature: the JWS (El Salvador) or the signed XML.
type Signed struct {
	Signature []byte
	// ContentType is "application/jose" or "application/xml".
	ContentType string
	Document    Document
}

// Verification is what checking a signature found.
type Verification struct {
	Valid    bool
	Document Document
	// Payload is the signed document: the JSON of a JWS, the XML otherwise.
	Payload []byte
}

// Options fixes what a signature would otherwise take from the world, so
// tests can reproduce a signer's output byte for byte.
type Options struct {
	// Now is the signing time; zero means time.Now().
	Now time.Time
	// Rand feeds the random Ids; nil means crypto/rand.
	Rand io.Reader
}

// Country signs and verifies one country's documents.
type Country interface {
	// Code is the ISO 3166-1 alpha-2 code, in upper case.
	Code() string
	// ParseCertificate reads an upload in the formats the country's
	// authority hands out.
	ParseCertificate(Upload) (Material, CertificateInfo, error)
	// Sign signs a document. contentType is the request's media type
	// ("application/json" or "application/xml", without parameters).
	Sign(m Material, body []byte, contentType string, o Options) (Signed, error)
	// Verify checks a signature against the organization's material. A
	// signature that does not check out, or cannot be read, is Valid: false
	// with a nil error.
	Verify(m Material, signature []byte) (Verification, error)
}

// NotImplemented is the placeholder a country's package returns until its
// signer is written.
func NotImplemented(code string) Country { return notImplemented(code) }

type notImplemented string

func (c notImplemented) Code() string { return string(c) }

func (notImplemented) ParseCertificate(Upload) (Material, CertificateInfo, error) {
	return Material{}, CertificateInfo{}, ErrNotImplemented
}

func (notImplemented) Sign(Material, []byte, string, Options) (Signed, error) {
	return Signed{}, ErrNotImplemented
}

func (notImplemented) Verify(Material, []byte) (Verification, error) {
	return Verification{}, ErrNotImplemented
}
