// Package co signs Colombia's UBL 2.1 documents for the DIAN (factura
// electrónica de venta, notas, ApplicationResponse): an XAdES-EPES signature
// under DIAN's signature policy v2, with inclusive canonicalization,
// RSA-SHA256 and SHA-256 digests and three references (the document through
// an enveloped-signature transform, the KeyInfo and the SignedProperties),
// placed in ext:UBLExtension[2]/ext:ExtensionContent.
//
// It reproduces ../sign-old/src/lib/server/api/signers/co.ts byte for byte:
// the Ids of DIAN's own example (xmldsig-<uuid> and its satellites, the
// KeyInfo on a second uuid), xades and xades141 declared on
// QualifyingProperties, the "supplier" ClaimedRole, SigningTime in
// Colombian time with milliseconds and the X509IssuerName of
// xades.IssuerName.
package co

import (
	"crypto"
	"fmt"
	"strings"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xades"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

// DIAN's signature policy v2.
const (
	PolicyURL         = "https://facturaelectronica.dian.gov.co/politicadefirma/v2/politicadefirmav2.pdf"
	PolicyDescription = "Política de firma para facturas electrónicas de la República de Colombia"
	PolicyDigest      = "dMoMvtcG5aIzgYo0tIsSQeVJBDnUnfSOfBpxXrmor0Y="
	// ClaimedRole is a facturador signing its own document; a proveedor
	// tecnológico signing for a client would claim "third party".
	ClaimedRole = "supplier"
)

const ublExtNS = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"

var inclusive = c14n.Options{}

// typeCodes says where each root keeps its type code, and its default.
var typeCodes = map[string]struct {
	path     []string
	fallback string
}{
	"Invoice":             {[]string{"InvoiceTypeCode"}, "01"},
	"CreditNote":          {[]string{"CreditNoteTypeCode"}, "20"},
	"DebitNote":           {[]string{"DebitNoteTypeCode"}, "30"},
	"ApplicationResponse": {[]string{"DocumentResponse", "Response", "ResponseCode"}, ""},
}

type country struct{}

// New returns the country's signer.
func New() signing.Country { return country{} }

func (country) Code() string { return "CO" }

// ParseCertificate reads the .p12 of an ONAC-accredited ECD with its PIN
// (or a PEM bundle). The NIT, without its check digit, is read as
// extractColombiaCertInfo of sign-old does (NIT).
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := xades.ReadUpload(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := xades.Info(m.Certificate)
	info.TaxID = NIT(m.Certificate)
	if info.TaxID == "" {
		info.TaxID = xades.GenericTaxID(m.Certificate)
	}
	return m, info, nil
}

type ids struct{ signature, reference, value, keyInfo, signedProperties string }

func (country) Sign(m signing.Material, body []byte, contentType string, o signing.Options) (signing.Signed, error) {
	if contentType != "application/xml" {
		return signing.Signed{}, signing.ErrUnsupportedContentType
	}
	if m.Key == nil || m.Certificate == nil {
		return signing.Signed{}, fmt.Errorf("%w: XAdES needs the certificate", signing.ErrInvalidCertificate)
	}
	rnd := xades.Random(o)
	u1, err := xades.UUID(rnd)
	if err != nil {
		return signing.Signed{}, err
	}
	u2, err := xades.UUID(rnd)
	if err != nil {
		return signing.Signed{}, err
	}
	sigID := "xmldsig-" + u1
	id := ids{
		signature:        sigID,
		reference:        sigID + "-ref0",
		value:            sigID + "-sigvalue",
		keyInfo:          "xmldsig-" + u2 + "-keyinfo",
		signedProperties: sigID + "-signedprops",
	}
	cert, err := xades.CertificateOf(m.Certificate)
	if err != nil {
		return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidCertificate, err)
	}
	props, err := xades.SignedProperties{
		Prefix:          "xades",
		ID:              id.signedProperties,
		SigningTime:     xades.Offset(xades.Now(o), -5, true),
		DigestAlgorithm: xades.SHA256,
		Certificate:     cert,
		Policy: &xades.Policy{
			Identifier:      PolicyURL,
			Description:     PolicyDescription,
			DigestAlgorithm: xades.SHA256,
			Digest:          PolicyDigest,
		},
		ClaimedRole: ClaimedRole,
	}.XML()
	if err != nil {
		return signing.Signed{}, err
	}

	before, after, err := prepare(body)
	if err != nil {
		return signing.Signed{}, err
	}
	render := func(d [3]string, value string) string {
		return before + template(id, cert, props, d, value) + after
	}

	doc, err := xmldsig.Parse([]byte(render([3]string{}, "")))
	if err != nil {
		return signing.Signed{}, err
	}
	root := doc.Root()
	keyInfo := xmldsig.ElementByID(root, id.keyInfo)
	propsEl := xmldsig.ElementByID(root, id.signedProperties)
	if xmldsig.ElementByID(root, id.signature) == nil || keyInfo == nil || propsEl == nil {
		return signing.Signed{}, fmt.Errorf("%w: could not assemble the signature", signing.ErrInvalidDocument)
	}
	digests := [3]string{
		documentDigest(root, id.signature),
		xades.Digest(crypto.SHA256, keyInfo, inclusive),
		xades.Digest(crypto.SHA256, propsEl, inclusive),
	}

	doc, err = xmldsig.Parse([]byte(render(digests, "")))
	if err != nil {
		return signing.Signed{}, err
	}
	signedInfo := xmldsig.FirstDescendant(xmldsig.ElementByID(doc.Root(), id.signature), xades.DSigNS, "SignedInfo")
	if signedInfo == nil {
		return signing.Signed{}, fmt.Errorf("%w: could not assemble the signature", signing.ErrInvalidDocument)
	}
	value, err := xades.SignRSA(m.Key, crypto.SHA256, c14n.Canonicalize(signedInfo, inclusive))
	if err != nil {
		return signing.Signed{}, err
	}

	meta := metadata(body)
	if meta.ID == "" {
		uuid, err := xades.UUID(rnd)
		if err != nil {
			return signing.Signed{}, err
		}
		meta.ID = strings.ToUpper(uuid)
	}
	return signing.Signed{
		Signature:   []byte(render(digests, value)),
		ContentType: "application/xml",
		Document:    meta,
	}, nil
}

// prepare normalizes the document, drops every previous signature and
// splits the serialized document at ext:UBLExtension[2]/ext:ExtensionContent,
// creating what is missing of it.
func prepare(body []byte) (before, after string, err error) {
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err)
	}
	root := doc.Root()
	xmldsig.StripWhitespace(&doc.Element)
	if _, ok := typeCodes[root.Tag]; !ok {
		return "", "", fmt.Errorf("%w: <%s> is not a DIAN document (Invoice, CreditNote, DebitNote, ApplicationResponse)", signing.ErrInvalidDocument, root.Tag)
	}
	for _, s := range xmldsig.Descendants(root, xades.DSigNS, "Signature") {
		if p := s.Parent(); p != nil {
			p.RemoveChild(s)
		}
	}
	slot := signatureSlot(root)
	mark := etree.NewElement("mark")
	slot.AddChild(mark)
	b, a := xmldsig.SerializeAround(doc, mark)
	before, after = string(b), string(a)
	return before, after, nil
}

// signatureSlot is the ExtensionContent of the second UBLExtension. DIAN
// keeps the first one for sts:DianExtensions.
func signatureSlot(root *etree.Element) *etree.Element {
	extensions := xmldsig.Child(root, "UBLExtensions")
	if extensions == nil {
		extensions = etree.NewElement("ext:UBLExtensions")
		root.InsertChildAt(0, extensions)
		xmldsig.Declare(extensions, "ext", ublExtNS)
	}
	list := func() []*etree.Element {
		var out []*etree.Element
		for _, c := range extensions.ChildElements() {
			if c.Tag == "UBLExtension" {
				out = append(out, c)
			}
		}
		return out
	}
	for len(list()) < 2 {
		ext := extensions.CreateElement("ext:UBLExtension")
		xmldsig.Declare(ext, "ext", ublExtNS)
		xmldsig.Declare(ext.CreateElement("ext:ExtensionContent"), "ext", ublExtNS)
	}
	target := list()[1]
	content := xmldsig.Child(target, "ExtensionContent")
	if content == nil {
		content = target.CreateElement("ext:ExtensionContent")
		xmldsig.Declare(content, "ext", ublExtNS)
	}
	return content
}

// documentDigest is reference 1 (URI="" and the enveloped transform): the
// root without the signature, inclusive.
func documentDigest(root *etree.Element, signatureID string) string {
	clone := root.Copy()
	if s := xmldsig.ElementByID(clone, signatureID); s != nil && s.Parent() != nil {
		s.Parent().RemoveChild(s)
	}
	return xades.Digest(crypto.SHA256, clone, inclusive)
}

func template(id ids, cert xades.Certificate, props string, d [3]string, value string) string {
	return strings.Join([]string{
		`<ds:Signature xmlns:ds="`, xades.DSigNS, `" Id="`, id.signature, `">`,
		`<ds:SignedInfo>`,
		`<ds:CanonicalizationMethod Algorithm="`, c14n.Inclusive, `"/>`,
		`<ds:SignatureMethod Algorithm="`, xades.RSASHA256, `"/>`,
		`<ds:Reference Id="`, id.reference, `" URI="">`,
		`<ds:Transforms>`,
		`<ds:Transform Algorithm="`, xades.EnvelopedSignature, `"/>`,
		`</ds:Transforms>`,
		`<ds:DigestMethod Algorithm="`, xades.SHA256, `"/>`,
		`<ds:DigestValue>`, d[0], `</ds:DigestValue>`,
		`</ds:Reference>`,
		`<ds:Reference URI="#`, id.keyInfo, `">`,
		`<ds:DigestMethod Algorithm="`, xades.SHA256, `"/>`,
		`<ds:DigestValue>`, d[1], `</ds:DigestValue>`,
		`</ds:Reference>`,
		`<ds:Reference Type="`, xades.SignedPropertiesType, `" URI="#`, id.signedProperties, `">`,
		`<ds:DigestMethod Algorithm="`, xades.SHA256, `"/>`,
		`<ds:DigestValue>`, d[2], `</ds:DigestValue>`,
		`</ds:Reference>`,
		`</ds:SignedInfo>`,
		`<ds:SignatureValue Id="`, id.value, `">`, value, `</ds:SignatureValue>`,
		`<ds:KeyInfo Id="`, id.keyInfo, `">`,
		`<ds:X509Data>`,
		`<ds:X509Certificate>`, cert.Base64, `</ds:X509Certificate>`,
		`</ds:X509Data>`,
		`</ds:KeyInfo>`,
		`<ds:Object>`,
		`<xades:QualifyingProperties xmlns:xades="`, xades.XAdESNS, `" xmlns:xades141="`, xades.XAdES141NS, `" Target="#`, id.signature, `">`,
		props,
		`</xades:QualifyingProperties>`,
		`</ds:Object>`,
		`</ds:Signature>`,
	}, "")
}

func (country) Verify(m signing.Material, signature []byte) (signing.Verification, error) {
	if m.Certificate == nil || !verify(m, signature) {
		return signing.Verification{}, nil
	}
	return signing.Verification{Valid: true, Document: metadata(signature), Payload: signature}, nil
}

func verify(m signing.Material, signed []byte) bool {
	doc, err := xmldsig.Parse(signed)
	if err != nil {
		return false
	}
	root := doc.Root()
	sigs := xmldsig.Descendants(root, xades.DSigNS, "Signature")
	if len(sigs) != 1 {
		return false
	}
	sig := sigs[0]
	sigID, _ := xmldsig.Attr(sig, "Id")
	if sigID == "" {
		return false
	}
	signedInfo := xmldsig.FirstDescendant(sig, xades.DSigNS, "SignedInfo")
	value := xmldsig.TrimmedText(xmldsig.ElementByLocalName(sig, "SignatureValue"))
	if signedInfo == nil || value == "" {
		return false
	}
	if !xades.VerifyRSA(m.Certificate.PublicKey, crypto.SHA256, c14n.Canonicalize(signedInfo, inclusive), value) {
		return false
	}
	refs := xmldsig.Descendants(signedInfo, xades.DSigNS, "Reference")
	if len(refs) != 3 {
		return false
	}
	var sawDocument, sawKeyInfo, sawProps bool
	for _, ref := range refs {
		uri, ok := xmldsig.Attr(ref, "URI")
		expected := xmldsig.TrimmedText(xmldsig.ElementByLocalName(ref, "DigestValue"))
		if !ok || expected == "" {
			return false
		}
		if uri == "" {
			if documentDigest(root, sigID) != expected {
				return false
			}
			sawDocument = true
			continue
		}
		if !strings.HasPrefix(uri, "#") {
			return false
		}
		target := xmldsig.ElementByID(root, uri[1:])
		if target == nil || xades.Digest(crypto.SHA256, target, inclusive) != expected {
			return false
		}
		if target.Tag == "KeyInfo" {
			sawKeyInfo = true
		}
		if typ, _ := xmldsig.Attr(ref, "Type"); typ == xades.SignedPropertiesType && target.Tag == "SignedProperties" {
			sawProps = true
		}
	}
	return sawDocument && sawKeyInfo && sawProps
}

// metadata: the id is the root's cbc:UUID (CUFE/CUDE), else its cbc:ID; the
// type, the root's own type code, both read as direct children.
func metadata(body []byte) signing.Document {
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return signing.Document{}
	}
	root := doc.Root()
	d := signing.Document{ID: xmldsig.TrimmedText(xmldsig.Child(root, "UUID"))}
	if d.ID == "" {
		d.ID = xmldsig.TrimmedText(xmldsig.Child(root, "ID"))
	}
	if tc, ok := typeCodes[root.Tag]; ok {
		el := root
		for _, name := range tc.path {
			if el = xmldsig.Child(el, name); el == nil {
				break
			}
		}
		d.Type = xmldsig.TrimmedText(el)
		if d.Type == "" {
			d.Type = tc.fallback
		}
	}
	return d
}
