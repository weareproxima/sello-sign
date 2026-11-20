// Package do signs the Dominican Republic's e-CF for the DGII: an enveloped
// XMLDSig with RSA-SHA256, inclusive C14N for the document and for
// SignedInfo, whitespace-only text dropped (PHP's preserveWhiteSpace =
// false), Reference URI="" and KeyInfo with the certificate only, the
// Signature unprefixed as the root's last child.
//
// It reproduces sign-old's do.ts byte for byte, except where that one
// signed something its own verify rejected: a root that declares the XMLDSig
// namespace with a prefix (xmldom then wrote ds:Signature) gets the
// unprefixed Signature the DGII expects.
package do

import (
	"fmt"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

// New returns the country's signer.
func New() signing.Country { return country{} }

type country struct{}

func (country) Code() string { return "DO" }

// ParseCertificate reads a PKCS #12 (or a PEM key with its certificate). The
// RNC is the subject's serialNumber digits or, failing that, the first run
// of 9 to 11 digits of its CN.
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := signing.ParseKeyAndCertificate(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := signing.Describe(m.Certificate)
	info.TaxID = signing.SerialOrCNTaxID(m.Certificate.Subject)
	return m, info, nil
}

// Sign signs an e-CF.
func (country) Sign(m signing.Material, body []byte, contentType string, o signing.Options) (signing.Signed, error) {
	if contentType != "application/xml" {
		return signing.Signed{}, signing.ErrUnsupportedContentType
	}
	if m.Certificate == nil {
		return signing.Signed{}, signing.ErrInvalidCertificate
	}
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return signing.Signed{}, invalid(err)
	}
	meta := metadata(doc.Root())
	xmldsig.StripWhitespace(&doc.Element)
	err = xmldsig.SignDocument(doc, xmldsig.Template{
		CanonicalizationMethod: c14n.Inclusive,
		SignatureMethod:        xmldsig.RSASHA256,
		References: []xmldsig.Reference{{
			URI:          "",
			Transforms:   []string{xmldsig.EnvelopedSignature},
			DigestMethod: xmldsig.SHA256,
		}},
		Certificate: m.Certificate.Raw,
	}, m.Key)
	if err != nil {
		return signing.Signed{}, err
	}
	if meta.ID, err = signing.DocumentIDOr(meta.ID, o); err != nil {
		return signing.Signed{}, err
	}
	return signing.Signed{Signature: xmldsig.Serialize(doc), ContentType: "application/xml", Document: meta}, nil
}

// Verify checks the first Signature of the document as XMLDSig prescribes.
func (country) Verify(m signing.Material, signature []byte) (signing.Verification, error) {
	doc, err := xmldsig.Parse(signature)
	if err != nil {
		return signing.Verification{}, nil
	}
	sig := xmldsig.FirstSignature(doc)
	if sig == nil || m.PublicKey() == nil || xmldsig.Verify(doc, sig, m.PublicKey()) != nil {
		return signing.Verification{}, nil
	}
	return signing.Verification{Valid: true, Document: metadata(doc.Root()), Payload: signature}, nil
}

// metadata: eNCF and TipoeCF.
func metadata(root *etree.Element) signing.Document {
	return signing.Document{ID: xmldsig.FirstText(root, "eNCF"), Type: xmldsig.FirstText(root, "TipoeCF")}
}

func invalid(err error) error { return fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err) }
