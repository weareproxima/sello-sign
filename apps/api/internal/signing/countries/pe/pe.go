// Package pe signs Peru's CPE (UBL 2.1) for SUNAT: an enveloped XMLDSig
// with RSA-SHA256 over SHA-256 and inclusive C14N, Reference URI="" with the
// enveloped-signature transform, KeyInfo with the certificate only, as a
// ds:Signature inside ext:UBLExtensions/ext:UBLExtension/ext:ExtensionContent
// (the last empty one, or a new one), with the Id that cac:Signature points
// at. Whitespace-only text is dropped and an earlier signature replaced.
//
// It reproduces sign-old's pe.ts byte for byte, except where that one
// signed something its own verify rejected: when the root does not declare
// the ext prefix, xmldom declared it on the new UBLExtensions only when
// serializing, after SignedInfo had been canonicalized without it.
package pe

import (
	"fmt"
	"regexp"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

// New returns the country's signer.
func New() signing.Country { return country{} }

type country struct{}

func (country) Code() string { return "PE" }

// ExtNS is UBL 2.1's CommonExtensionComponents.
const ExtNS = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"

// DefaultSignatureID is the ds:Signature Id when the document names none.
const DefaultSignatureID = "SignatureSP"

var safeID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// typeByRoot is catálogo 01 for the roots without a type-code element.
var typeByRoot = map[string]string{
	"CreditNote":       "07",
	"DebitNote":        "08",
	"DespatchAdvice":   "09",
	"Retention":        "20",
	"Perception":       "40",
	"SummaryDocuments": "RC",
	"VoidedDocuments":  "RA",
}

// Sign signs a CPE.
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
	root := doc.Root()
	xmldsig.StripWhitespace(&doc.Element)
	for _, old := range extensionSignatures(root) {
		old.Parent().RemoveChild(old)
	}
	id := signatureID(root)
	content := extensionContent(root)
	digest, err := xmldsig.Digest(xmldsig.SHA256, xmldsig.CanonicalDocument(doc, c14n.Options{}))
	if err != nil {
		return signing.Signed{}, err
	}
	sig := xmldsig.Template{
		Prefix:                 "ds",
		ID:                     id,
		CanonicalizationMethod: c14n.Inclusive,
		SignatureMethod:        xmldsig.RSASHA256,
		References: []xmldsig.Reference{{
			URI:          "",
			Transforms:   []string{xmldsig.EnvelopedSignature},
			DigestMethod: xmldsig.SHA256,
			DigestValue:  digest,
		}},
		Certificate: m.Certificate.Raw,
	}.Build()
	content.AddChild(sig.Element)
	xmldsig.Declare(sig.Element, "ds", xmldsig.NS)
	if err := sig.Sign(m.Key); err != nil {
		return signing.Signed{}, err
	}
	if meta.ID, err = signing.DocumentIDOr(meta.ID, o); err != nil {
		return signing.Signed{}, err
	}
	return signing.Signed{Signature: xmldsig.Serialize(doc), ContentType: "application/xml", Document: meta}, nil
}

// signatureID: the #fragment of cac:Signature's cbc:URI, else its cbc:ID,
// else SignatureSP, skipping what cannot be an Id.
func signatureID(root *etree.Element) string {
	var block *etree.Element
	for _, c := range xmldsig.ChildrenNamed(root, "Signature") {
		if xmldsig.NamespaceURI(c) != xmldsig.NS {
			block = c
			break
		}
	}
	if block == nil {
		return DefaultSignatureID
	}
	if uris := xmldsig.Descendants(block, "*", "URI"); len(uris) > 0 {
		uri := xmldsig.JSTrim(xmldsig.TextContent(uris[0]))
		if len(uri) > 1 && uri[0] == '#' && safeID.MatchString(uri[1:]) {
			return uri[1:]
		}
	}
	if id := childText(block, "ID"); safeID.MatchString(id) {
		return id
	}
	return DefaultSignatureID
}

// extensionContent finds the last empty ext:ExtensionContent, creating
// ext:UBLExtensions (first child of the root) or a new
// ext:UBLExtension/ext:ExtensionContent as needed, with the prefix the root
// declares for ExtNS, or "ext".
func extensionContent(root *etree.Element) *etree.Element {
	prefix := "ext"
	for _, a := range root.Attr {
		if a.Space == "xmlns" && a.Value == ExtNS {
			prefix = a.Key
			break
		}
	}
	create := func(parent *etree.Element, local string, at int) *etree.Element {
		el := etree.NewElement(prefix + ":" + local)
		parent.InsertChildAt(at, el)
		xmldsig.Declare(el, prefix, ExtNS)
		return el
	}
	extensions := firstNamed(root, "UBLExtensions")
	if extensions == nil {
		extensions = create(root, "UBLExtensions", 0)
	}
	var empty *etree.Element
	for _, ext := range xmldsig.ChildrenNamed(extensions, "UBLExtension") {
		for _, c := range xmldsig.ChildrenNamed(ext, "ExtensionContent") {
			if len(c.ChildElements()) == 0 && xmldsig.JSTrim(xmldsig.TextContent(c)) == "" {
				empty = c
			}
		}
	}
	if empty != nil {
		return empty
	}
	ext := create(extensions, "UBLExtension", len(extensions.Child))
	return create(ext, "ExtensionContent", 0)
}

// extensionSignatures are the ds:Signature children of every
// ext:ExtensionContent.
func extensionSignatures(root *etree.Element) []*etree.Element {
	var out []*etree.Element
	for _, content := range xmldsig.Descendants(root, "*", "ExtensionContent") {
		for _, c := range xmldsig.ChildrenNamed(content, "Signature") {
			if xmldsig.NamespaceURI(c) == xmldsig.NS {
				out = append(out, c)
			}
		}
	}
	return out
}

// Verify checks the last signature inside an ext:ExtensionContent, as
// sign-old did: whitespace-only text dropped first, the SignatureValue
// (RSA-SHA256, -SHA512 or -SHA1) over SignedInfo, then the Reference, which
// must be URI="" with the enveloped transform (SHA-256, -512 or -1).
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
	xmldsig.StripWhitespace(&doc.Element)
	sigs := extensionSignatures(root)
	if len(sigs) == 0 {
		return false
	}
	sig := sigs[len(sigs)-1]
	signedInfo := firstNamed(sig, "SignedInfo")
	value := childText(sig, "SignatureValue")
	if signedInfo == nil || value == "" {
		return false
	}
	method, _ := xmldsig.Attr(firstNamed(signedInfo, "SignatureMethod"), "Algorithm")
	if xmldsig.VerifyValue(m.PublicKey(), method, c14n.Canonicalize(signedInfo, c14n.Options{}), value) != nil {
		return false
	}
	ref := firstNamed(signedInfo, "Reference")
	if ref == nil {
		return false
	}
	if uri, _ := xmldsig.Attr(ref, "URI"); uri != "" {
		return false
	}
	enveloped := false
	for _, ts := range xmldsig.ChildrenNamed(ref, "Transforms") {
		for _, t := range xmldsig.ChildrenNamed(ts, "Transform") {
			if alg, _ := xmldsig.Attr(t, "Algorithm"); alg == xmldsig.EnvelopedSignature {
				enveloped = true
			}
		}
	}
	digestMethod, _ := xmldsig.Attr(firstNamed(ref, "DigestMethod"), "Algorithm")
	want := childText(ref, "DigestValue")
	if !enveloped || want == "" {
		return false
	}
	sig.Parent().RemoveChild(sig)
	got, err := xmldsig.Digest(digestMethod, xmldsig.CanonicalDocument(doc, c14n.Options{}))
	return err == nil && got == want
}

// ParseCertificate reads a PKCS #12 (the CDT, or one from an INDECOPI
// accredited entity) or a PEM key with its certificate. The RUC is looked
// for in every OU, then the CN, then organizationIdentifier (NTRPE-<RUC>),
// preferring one whose check digit is right; never in serialNumber, which
// holds the DNI of the person.
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := signing.ParseKeyAndCertificate(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := signing.Describe(m.Certificate)
	info.TaxID = RUC(m.Certificate.Subject)
	return m, info, nil
}

// metadata: the root's cbc:ID, and InvoiceTypeCode, DespatchAdviceTypeCode
// or the code of the root.
func metadata(root *etree.Element) signing.Document {
	d := signing.Document{ID: childText(root, "ID")}
	for _, local := range []string{"InvoiceTypeCode", "DespatchAdviceTypeCode"} {
		if d.Type = childText(root, local); d.Type != "" {
			return d
		}
	}
	d.Type = typeByRoot[root.Tag]
	return d
}

func firstNamed(el *etree.Element, local string) *etree.Element {
	if el == nil {
		return nil
	}
	if c := xmldsig.ChildrenNamed(el, local); len(c) > 0 {
		return c[0]
	}
	return nil
}

func childText(el *etree.Element, local string) string {
	if c := firstNamed(el, local); c != nil {
		return xmldsig.JSTrim(xmldsig.TextContent(c))
	}
	return ""
}
