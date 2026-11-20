// Package cr signs Costa Rica's comprobantes electrónicos (Ministerio de
// Hacienda, v4.4): an enveloped XAdES-EPES signature with exclusive
// canonicalization, RSA-SHA256 and SHA-256 digests, two references (the
// document and the SignedProperties) and Hacienda's signature policy.
//
// It reproduces ../sign-old/src/lib/server/api/signers/cr.ts byte for byte,
// including what is odd but accepted: the document reference declares an
// XPath transform, not(ancestor-or-self::ds:Signature), that the signer does
// not run (the document is digested before the signature exists, which
// gives the same bytes), redundant xmlns:ds and xmlns:xades declarations on
// SignedInfo and SignedProperties, and the X509IssuerName of
// xades.IssuerName.
package cr

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

const (
	policyURL    = "https://cdn.comprobanteselectronicos.go.cr/xml-schemas/Resoluci%C3%B3n_General_sobre_disposiciones_t%C3%A9cnicas_comprobantes_electr%C3%B3nicos_para_efectos_tributarios.pdf"
	policyDigest = "DWxin1xWOeI8OuWQXazh4VjLWAaCLAA954em7DMh0h8="
	referenceID  = "r-id-1"
)

var exclusive = c14n.Options{Exclusive: true}

// documentTypes maps a root element to its document type.
var documentTypes = map[string]string{
	"FacturaElectronica":     "01",
	"NotaDebitoElectronica":  "02",
	"NotaCreditoElectronica": "03",
	"TiqueteElectronico":     "04",
	"MensajeReceptor":        "05",
}

type country struct{}

// New returns the country's signer.
func New() signing.Country { return country{} }

func (country) Code() string { return "CR" }

// ParseCertificate reads Hacienda's .p12 with its PIN (or a PEM bundle).
// The tax id is the digits of the subject serialNumber (CPJ-3-101-123456),
// else a run of 9 to 11 digits in the CN, as sign-old did.
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := xades.ReadUpload(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := xades.Info(m.Certificate)
	info.TaxID = xades.GenericTaxID(m.Certificate)
	return m, info, nil
}

func (country) Sign(m signing.Material, body []byte, contentType string, o signing.Options) (signing.Signed, error) {
	if contentType != "application/xml" {
		return signing.Signed{}, signing.ErrUnsupportedContentType
	}
	if m.Key == nil || m.Certificate == nil {
		return signing.Signed{}, fmt.Errorf("%w: XAdES needs the certificate", signing.ErrInvalidCertificate)
	}
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err)
	}
	xmldsig.StripWhitespace(&doc.Element)
	root := doc.Root()

	rnd := xades.Random(o)
	uuid, err := xades.UUID(rnd)
	if err != nil {
		return signing.Signed{}, err
	}
	signatureID := "id-" + strings.ReplaceAll(uuid, "-", "")
	signedPropertiesID := "xades-" + signatureID

	cert, err := xades.CertificateOf(m.Certificate)
	if err != nil {
		return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidCertificate, err)
	}

	documentDigest := xades.Digest(crypto.SHA256, root, exclusive)

	props, err := xades.SignedProperties{
		Prefix:          "xades",
		Declarations:    ` xmlns:xades="` + xades.XAdESNS + `" xmlns:ds="` + xades.DSigNS + `"`,
		ID:              signedPropertiesID,
		SigningTime:     xades.UTCSeconds(xades.Now(o)),
		DigestAlgorithm: xades.SHA256,
		Certificate:     cert,
		Policy: &xades.Policy{
			Identifier:      policyURL,
			DigestAlgorithm: xades.SHA256,
			Digest:          policyDigest,
		},
		DataObjectFormat: &xades.DataObjectFormat{
			ObjectReference: "#" + referenceID,
			MimeType:        "application/octet-stream",
		},
	}.XML()
	if err != nil {
		return signing.Signed{}, err
	}
	propsDigest, err := fragmentDigest(props)
	if err != nil {
		return signing.Signed{}, err
	}

	signedInfo := signedInfoXML(signedPropertiesID, documentDigest, propsDigest)
	siDoc, err := xmldsig.Parse([]byte(signedInfo))
	if err != nil {
		return signing.Signed{}, err
	}
	value, err := xades.SignRSA(m.Key, crypto.SHA256, c14n.Canonicalize(siDoc.Root(), exclusive))
	if err != nil {
		return signing.Signed{}, err
	}

	signature := strings.Join([]string{
		`<ds:Signature Id="`, signatureID, `" xmlns:ds="`, xades.DSigNS, `">`,
		signedInfo,
		`<ds:SignatureValue Id="value-`, signatureID, `">`, value, `</ds:SignatureValue>`,
		`<ds:KeyInfo><ds:X509Data><ds:X509Certificate>`, cert.Base64, `</ds:X509Certificate></ds:X509Data></ds:KeyInfo>`,
		`<ds:Object>`,
		`<xades:QualifyingProperties xmlns:xades="`, xades.XAdESNS, `" Target="#`, signatureID, `">`,
		props,
		`</xades:QualifyingProperties>`,
		`</ds:Object>`,
		`</ds:Signature>`,
	}, "")

	// The signature goes in as the root's last child.
	mark := etree.NewElement("mark")
	root.AddChild(mark)
	before, after := xmldsig.SerializeAround(doc, mark)
	out := string(before) + signature + string(after)

	meta := metadata(body)
	if meta.ID == "" {
		id, err := xades.UUID(rnd)
		if err != nil {
			return signing.Signed{}, err
		}
		meta.ID = strings.ToUpper(id)
	}
	return signing.Signed{Signature: []byte(out), ContentType: "application/xml", Document: meta}, nil
}

func signedInfoXML(signedPropertiesID, documentDigest, propsDigest string) string {
	return strings.Join([]string{
		`<ds:SignedInfo xmlns:ds="`, xades.DSigNS, `">`,
		`<ds:CanonicalizationMethod Algorithm="`, c14n.Exclusive, `"/>`,
		`<ds:SignatureMethod Algorithm="`, xades.RSASHA256, `"/>`,
		`<ds:Reference Id="`, referenceID, `" Type="" URI="">`,
		`<ds:Transforms>`,
		`<ds:Transform Algorithm="`, xades.XPathFilter, `">`,
		`<ds:XPath>not(ancestor-or-self::ds:Signature)</ds:XPath>`,
		`</ds:Transform>`,
		`<ds:Transform Algorithm="`, c14n.Exclusive, `"/>`,
		`</ds:Transforms>`,
		`<ds:DigestMethod Algorithm="`, xades.SHA256, `"/>`,
		`<ds:DigestValue>`, documentDigest, `</ds:DigestValue>`,
		`</ds:Reference>`,
		`<ds:Reference Type="`, xades.SignedPropertiesType, `" URI="#`, signedPropertiesID, `">`,
		`<ds:Transforms>`,
		`<ds:Transform Algorithm="`, c14n.Exclusive, `"/>`,
		`</ds:Transforms>`,
		`<ds:DigestMethod Algorithm="`, xades.SHA256, `"/>`,
		`<ds:DigestValue>`, propsDigest, `</ds:DigestValue>`,
		`</ds:Reference>`,
		`</ds:SignedInfo>`,
	}, "")
}

// fragmentDigest digests SignedProperties as its own document, as the TS
// signer did: under exclusive canonicalization that is the same as in its
// final place, because it declares what it uses.
func fragmentDigest(xml string) (string, error) {
	d, err := xmldsig.Parse([]byte(xml))
	if err != nil {
		return "", err
	}
	return xades.Digest(crypto.SHA256, d.Root(), exclusive), nil
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
	sig := xmldsig.FirstDescendant(root, xades.DSigNS, "Signature")
	if sig == nil {
		return false
	}
	signedInfo := xmldsig.FirstDescendant(sig, xades.DSigNS, "SignedInfo")
	value := xmldsig.TrimmedText(xmldsig.ElementByLocalName(sig, "SignatureValue"))
	if signedInfo == nil || value == "" {
		return false
	}
	if !xades.VerifyRSA(m.Certificate.PublicKey, crypto.SHA256, c14n.Canonicalize(signedInfo, exclusive), value) {
		return false
	}

	var docDigest, propsDigest string
	for _, ref := range xmldsig.Descendants(sig, xades.DSigNS, "Reference") {
		digest := xmldsig.TrimmedText(xmldsig.ElementByLocalName(ref, "DigestValue"))
		if digest == "" {
			continue
		}
		uri, hasURI := xmldsig.Attr(ref, "URI")
		typ, _ := xmldsig.Attr(ref, "Type")
		if hasURI && uri == "" {
			docDigest = digest
		} else if typ == xades.SignedPropertiesType {
			propsDigest = digest
		}
	}
	if docDigest == "" || propsDigest == "" {
		return false
	}

	// The document without its signature, which has to be the root's child.
	unsigned := root.Copy()
	clone := xmldsig.FirstDescendant(unsigned, xades.DSigNS, "Signature")
	if clone == nil || clone.Parent() != unsigned {
		return false
	}
	unsigned.RemoveChild(clone)
	xmldsig.StripWhitespace(unsigned)
	if xades.Digest(crypto.SHA256, unsigned, exclusive) != docDigest {
		return false
	}

	props := xmldsig.FirstDescendant(sig, xades.XAdESNS, "SignedProperties")
	return props != nil && xades.Digest(crypto.SHA256, props, exclusive) == propsDigest
}

// metadata reads the Clave and the type from the root's name.
func metadata(body []byte) signing.Document {
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return signing.Document{}
	}
	root := doc.Root()
	var clave string
	for _, c := range root.ChildElements() {
		if e := xmldsig.ElementByLocalName(c, "Clave"); e != nil {
			clave = xmldsig.TrimmedText(e)
			break
		}
	}
	return signing.Document{ID: clave, Type: documentTypes[root.Tag]}
}
