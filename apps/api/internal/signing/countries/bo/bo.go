// Package bo signs Bolivia's invoices for the SIN (Facturación en Línea,
// modalidad Electrónica en Línea): an enveloped XMLDSig with RSA-SHA256, as
// the SIN's Java sample with Apache Santuario makes it. Reference URI="" with
// the transforms [enveloped-signature, C14N WithComments], SignedInfo in
// plain inclusive C14N, KeyInfo with the certificate only, the Signature
// unprefixed as the root's last child; whitespace-only text is dropped and
// an earlier Signature replaced.
//
// It reproduces sign-old's bo.ts byte for byte, except where that one
// signed something a conforming verifier (its own included) rejects:
//   - SignedInfo was canonicalized without the namespaces it inherits, so a
//     root that declares xmlns:xsi broke the signature;
//   - comments counted in the digest, but a same-document reference
//     (URI="") drops them before any transform, so a document with comments
//     broke it too.
package bo

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

func (country) Code() string { return "BO" }

// ParseCertificate reads a PKCS #12 (ADSIB or Digicert Bolivia) or a PEM key
// with its certificate. The NIT is the subject's serialNumber ("Número de
// NIT …") digits, as the SIN's CSR specification lays it out.
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := signing.ParseKeyAndCertificate(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := signing.Describe(m.Certificate)
	info.TaxID = signing.SerialOrCNTaxID(m.Certificate.Subject)
	return m, info, nil
}

// Sign signs an invoice of any documento sector.
func (country) Sign(m signing.Material, body []byte, contentType string, o signing.Options) (signing.Signed, error) {
	if contentType != "application/xml" {
		return signing.Signed{}, signing.ErrUnsupportedContentType
	}
	if m.Certificate == nil {
		return signing.Signed{}, signing.ErrInvalidCertificate
	}
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err)
	}
	meta := metadata(doc.Root())
	xmldsig.StripWhitespace(&doc.Element)
	for _, old := range xmldsig.Descendants(doc.Root(), xmldsig.NS, "Signature") {
		if p := old.Parent(); p != nil {
			p.RemoveChild(old)
		}
	}
	err = xmldsig.SignDocument(doc, xmldsig.Template{
		CanonicalizationMethod: c14n.Inclusive,
		SignatureMethod:        xmldsig.RSASHA256,
		References: []xmldsig.Reference{{
			URI:          "",
			Transforms:   []string{xmldsig.EnvelopedSignature, c14n.InclusiveWithComments},
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

// metadata: the cuf, and codigoDocumentoSector or else the root's name.
func metadata(root *etree.Element) signing.Document {
	d := signing.Document{ID: xmldsig.FirstText(root, "cuf"), Type: xmldsig.FirstText(root, "codigoDocumentoSector")}
	if d.Type == "" {
		d.Type = root.Tag
	}
	return d
}
