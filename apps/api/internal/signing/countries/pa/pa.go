// Package pa signs Panama's rFE for the DGI (SFEP), per the "Ficha Técnica
// de la Factura Electrónica para PAC" v1.00 §3.6: an enveloped XMLDSig with
// RSA-SHA256 and exclusive C14N, a single Reference URI="" with the
// transforms [enveloped-signature, exc-c14n], KeyInfo with the certificate
// only, the Signature unprefixed. The digest covers the rFE without gNoFirm
// (which carries the QR derived from the signature) and without
// whitespace-only text; the Signature goes after the business groups and
// before gNoFirm, which stays last.
//
// Because gNoFirm is left out by the ficha and not by a transform, a
// document that already has it does not verify with a stock XMLDSig
// verifier; Verify applies the DGI's rule. It reproduces sign-old's pa.ts
// byte for byte.
package pa

import (
	"fmt"
	"strings"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

// New returns the country's signer.
func New() signing.Country { return country{} }

type country struct{}

func (country) Code() string { return "PA" }

// UnsignedGroup is group G10, "Campos Fuera de la Firma Electrónica".
const UnsignedGroup = "gNoFirm"

// OIDRUC is the subject attribute with the RUC (Registro Público de Panamá).
var OIDRUC = []int{2, 16, 591, 1, 1, 2, 2}

// ParseCertificate reads a PKCS #12 or a PEM key with its certificate. The
// RUC is the subject attribute 2.16.591.1.1.2.2 or, failing that, what
// sign-old read for every country (serialNumber digits, or 9 to 11 digits of
// the CN).
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := signing.ParseKeyAndCertificate(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := signing.Describe(m.Certificate)
	info.TaxID = strings.TrimSpace(signing.SubjectValue(m.Certificate.Subject, OIDRUC))
	if info.TaxID == "" {
		info.TaxID = signing.SerialOrCNTaxID(m.Certificate.Subject)
	}
	return m, info, nil
}

// Sign signs an rFE.
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
	root := doc.Root()
	unsigned := unsignedGroup(root)
	if unsigned != nil {
		root.RemoveChild(unsigned)
	}
	digest, err := xmldsig.Digest(xmldsig.SHA256, xmldsig.CanonicalDocument(doc, c14n.Options{Exclusive: true}))
	if err != nil {
		return signing.Signed{}, err
	}
	sig := xmldsig.Template{
		CanonicalizationMethod: c14n.Exclusive,
		SignatureMethod:        xmldsig.RSASHA256,
		References: []xmldsig.Reference{{
			URI:          "",
			Transforms:   []string{xmldsig.EnvelopedSignature, c14n.Exclusive},
			DigestMethod: xmldsig.SHA256,
			DigestValue:  digest,
		}},
		Certificate: m.Certificate.Raw,
	}.Build()
	root.AddChild(sig.Element)
	xmldsig.Declare(sig.Element, "", xmldsig.NS)
	if unsigned != nil {
		root.AddChild(unsigned)
	}
	if err := sig.Sign(m.Key); err != nil {
		return signing.Signed{}, err
	}
	if meta.ID, err = signing.DocumentIDOr(meta.ID, o); err != nil {
		return signing.Signed{}, err
	}
	return signing.Signed{Signature: xmldsig.Serialize(doc), ContentType: "application/xml", Document: meta}, nil
}

// Verify applies the DGI's rule, as sign-old did: the RSA-SHA256 signature
// of the exclusive C14N of SignedInfo, exactly one Reference with URI="",
// and its digest over the exclusive C14N of the rFE without the Signature,
// gNoFirm or whitespace-only text.
func (country) Verify(m signing.Material, signature []byte) (signing.Verification, error) {
	if m.PublicKey() == nil || !verify(m, signature) {
		return signing.Verification{}, nil
	}
	doc, _ := xmldsig.Parse(signature)
	return signing.Verification{Valid: true, Document: metadata(doc.Root()), Payload: signature}, nil
}

func verify(m signing.Material, signature []byte) bool {
	doc, err := xmldsig.Parse(signature)
	if err != nil {
		return false
	}
	root := doc.Root()
	sigs := xmldsig.Descendants(root, xmldsig.NS, "Signature")
	if len(sigs) == 0 {
		return false
	}
	sig := sigs[0]
	signedInfo := first(xmldsig.Descendants(sig, xmldsig.NS, "SignedInfo"))
	value := first(xmldsig.Descendants(sig, xmldsig.NS, "SignatureValue"))
	if signedInfo == nil || value == nil || xmldsig.JSTrim(xmldsig.TextContent(value)) == "" {
		return false
	}
	data := c14n.Canonicalize(signedInfo, c14n.Options{Exclusive: true})
	if xmldsig.VerifyValue(m.PublicKey(), xmldsig.RSASHA256, data, xmldsig.TextContent(value)) != nil {
		return false
	}
	refs := xmldsig.Descendants(sig, xmldsig.NS, "Reference")
	if len(refs) != 1 {
		return false
	}
	if uri, _ := xmldsig.Attr(refs[0], "URI"); uri != "" {
		return false
	}
	dv := first(xmldsig.Descendants(refs[0], xmldsig.NS, "DigestValue"))
	if dv == nil {
		return false
	}
	want := xmldsig.JSTrim(xmldsig.TextContent(dv))
	if want == "" {
		return false
	}
	sig.Parent().RemoveChild(sig)
	if unsigned := unsignedGroup(root); unsigned != nil {
		root.RemoveChild(unsigned)
	}
	xmldsig.StripWhitespace(root)
	got, _ := xmldsig.Digest(xmldsig.SHA256, xmldsig.CanonicalDocument(doc, c14n.Options{Exclusive: true}))
	return got == want
}

func first(els []*etree.Element) *etree.Element {
	if len(els) == 0 {
		return nil
	}
	return els[0]
}

func unsignedGroup(root *etree.Element) *etree.Element {
	return first(xmldsig.ChildrenNamed(root, UnsignedGroup))
}

// metadata: dId (the CUFE preceded by "FE") and iDoc (B06).
func metadata(root *etree.Element) signing.Document {
	text := func(local string) string {
		if el := first(xmldsig.Descendants(root, "*", local)); el != nil {
			return xmldsig.JSTrim(xmldsig.TextContent(el))
		}
		return ""
	}
	return signing.Document{ID: text("dId"), Type: text("iDoc")}
}
