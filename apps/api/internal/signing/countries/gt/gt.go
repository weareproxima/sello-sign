// Package gt signs Guatemala's DTE (SAT, régimen FEL): the issuer's firma
// de emisión, an XAdES-BES signature with inclusive canonicalization,
// RSA-SHA256 and SHA-256 digests over dte:DatosEmision, placed as a child of
// dte:GTDocumento after dte:SAT and before a certifier's signature.
//
// It reproduces ../sign-old/src/lib/server/api/signers/gt.ts byte for byte:
// the fixed Ids (FirmaEmisor and its satellites), which make re-signing
// replace its own signature and leave the certifier's alone, the enveloped
// transform on the #DatosEmision reference (a no-op there), the
// SignedProperties reference without transforms and the X509IssuerName of
// xades.IssuerName.
package gt

import (
	"crypto"
	"fmt"
	"regexp"
	"strings"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xades"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

const (
	issuerSignatureID    = "FirmaEmisor"
	certifierSignatureID = "FirmaCertificador"
	signedPropertiesID   = issuerSignatureID + "-SignedProperties"
	referenceID          = issuerSignatureID + "-Reference"
	datosEmisionID       = "DatosEmision"
)

var (
	inclusive = c14n.Options{}
	safeID    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
)

type country struct{}

// New returns the country's signer.
func New() signing.Country { return country{} }

func (country) Code() string { return "GT" }

// ParseCertificate reads the SAT certificate's .p12 with its PIN (or a PEM
// bundle). The NIT comes from the subject serialNumber, then a NIT-like
// token in the CN or the OU (extractGuatemalaCertInfo of sign-old).
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := xades.ReadUpload(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := xades.Info(m.Certificate)
	info.TaxID = NIT(m.Certificate.Subject.Names)
	if info.TaxID == "" {
		info.TaxID = xades.GenericTaxID(m.Certificate)
	}
	return m, info, nil
}

func (country) Sign(m signing.Material, body []byte, contentType string, o signing.Options) (signing.Signed, error) {
	if contentType != "application/xml" {
		return signing.Signed{}, signing.ErrUnsupportedContentType
	}
	if m.Key == nil || m.Certificate == nil {
		return signing.Signed{}, fmt.Errorf("%w: XAdES needs the certificate", signing.ErrInvalidCertificate)
	}
	cert, err := xades.CertificateOf(m.Certificate)
	if err != nil {
		return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidCertificate, err)
	}
	signingTime := xades.UTCSeconds(xades.Now(o))

	before, after, datosID, err := prepare(body)
	if err != nil {
		return signing.Signed{}, err
	}
	props, err := xades.SignedProperties{
		Prefix:          "xades",
		ID:              signedPropertiesID,
		SigningTime:     signingTime,
		DigestAlgorithm: xades.SHA256,
		Certificate:     cert,
	}.XML()
	if err != nil {
		return signing.Signed{}, err
	}
	render := func(datosDigest, propsDigest, value string) string {
		return before + template(cert, props, datosID, datosDigest, propsDigest, value) + after
	}

	// The digests are taken in the document as a verifier will see it:
	// inclusive canonicalization brings in the ancestors' namespaces.
	doc, err := xmldsig.Parse([]byte(render("", "", "")))
	if err != nil {
		return signing.Signed{}, err
	}
	root := doc.Root()
	datos := xmldsig.ElementByLocalName(root, "DatosEmision")
	propsEl := xmldsig.ElementByID(root, signedPropertiesID)
	if datos == nil || propsEl == nil {
		return signing.Signed{}, fmt.Errorf("%w: could not assemble the signature", signing.ErrInvalidDocument)
	}
	datosDigest := xades.Digest(crypto.SHA256, datos, inclusive)
	propsDigest := xades.Digest(crypto.SHA256, propsEl, inclusive)

	doc, err = xmldsig.Parse([]byte(render(datosDigest, propsDigest, "")))
	if err != nil {
		return signing.Signed{}, err
	}
	sig := xmldsig.ElementByID(doc.Root(), issuerSignatureID)
	var signedInfo *etree.Element
	if sig != nil {
		signedInfo = xmldsig.FirstDescendant(sig, xades.DSigNS, "SignedInfo")
	}
	if signedInfo == nil {
		return signing.Signed{}, fmt.Errorf("%w: could not assemble the signature", signing.ErrInvalidDocument)
	}
	value, err := xades.SignRSA(m.Key, crypto.SHA256, c14n.Canonicalize(signedInfo, inclusive))
	if err != nil {
		return signing.Signed{}, err
	}

	meta := metadata(body)
	if meta.ID == "" {
		id, err := xades.UUID(xades.Random(o))
		if err != nil {
			return signing.Signed{}, err
		}
		meta.ID = strings.ToUpper(id)
	}
	return signing.Signed{
		Signature:   []byte(render(datosDigest, propsDigest, value)),
		ContentType: "application/xml",
		Document:    meta,
	}, nil
}

// prepare normalizes the document, drops a previous FirmaEmisor and splits
// the serialized document where the signature goes: the first child of
// GTDocumento after SAT, before a certifier's signature.
func prepare(body []byte) (before, after, datosID string, err error) {
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return "", "", "", fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err)
	}
	root := doc.Root()
	xmldsig.StripWhitespace(&doc.Element)
	if root.Tag != "GTDocumento" {
		return "", "", "", fmt.Errorf("%w: expected <dte:GTDocumento>, found <%s>", signing.ErrInvalidDocument, root.Tag)
	}
	datos := xmldsig.ElementByLocalName(root, "DatosEmision")
	if datos == nil {
		return "", "", "", fmt.Errorf("%w: no <dte:DatosEmision> to sign", signing.ErrInvalidDocument)
	}
	datosID, _ = xmldsig.Attr(datos, "ID")
	if datosID == "" {
		datos.CreateAttr("ID", datosEmisionID)
		datosID = datosEmisionID
	}
	if !safeID.MatchString(datosID) {
		return "", "", "", fmt.Errorf("%w: <dte:DatosEmision ID=%q> is not an ID", signing.ErrInvalidDocument, datosID)
	}
	for _, s := range xmldsig.ChildrenNS(root, xades.DSigNS, "Signature") {
		if id, _ := xmldsig.Attr(s, "Id"); id == issuerSignatureID {
			root.RemoveChild(s)
		}
	}
	mark := etree.NewElement("mark")
	if rest := xmldsig.ChildrenNS(root, xades.DSigNS, "Signature"); len(rest) > 0 {
		root.InsertChildAt(rest[0].Index(), mark)
	} else {
		root.AddChild(mark)
	}
	b, a := xmldsig.SerializeAround(doc, mark)
	before, after = string(b), string(a)
	return before, after, datosID, nil
}

func template(cert xades.Certificate, props, datosID, datosDigest, propsDigest, value string) string {
	return strings.Join([]string{
		`<ds:Signature xmlns:ds="`, xades.DSigNS, `" Id="`, issuerSignatureID, `">`,
		`<ds:SignedInfo>`,
		`<ds:CanonicalizationMethod Algorithm="`, c14n.Inclusive, `"/>`,
		`<ds:SignatureMethod Algorithm="`, xades.RSASHA256, `"/>`,
		`<ds:Reference Id="`, referenceID, `" URI="#`, datosID, `">`,
		`<ds:Transforms>`,
		`<ds:Transform Algorithm="`, xades.EnvelopedSignature, `"/>`,
		`</ds:Transforms>`,
		`<ds:DigestMethod Algorithm="`, xades.SHA256, `"/>`,
		`<ds:DigestValue>`, datosDigest, `</ds:DigestValue>`,
		`</ds:Reference>`,
		`<ds:Reference Type="`, xades.SignedPropertiesType, `" URI="#`, signedPropertiesID, `">`,
		`<ds:DigestMethod Algorithm="`, xades.SHA256, `"/>`,
		`<ds:DigestValue>`, propsDigest, `</ds:DigestValue>`,
		`</ds:Reference>`,
		`</ds:SignedInfo>`,
		`<ds:SignatureValue Id="`, issuerSignatureID, `-SignatureValue">`, value, `</ds:SignatureValue>`,
		`<ds:KeyInfo><ds:X509Data><ds:X509Certificate>`, cert.Base64, `</ds:X509Certificate></ds:X509Data></ds:KeyInfo>`,
		`<ds:Object>`,
		`<xades:QualifyingProperties xmlns:xades="`, xades.XAdESNS, `" Target="#`, issuerSignatureID, `">`,
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
	sig := issuerSignature(root)
	if sig == nil {
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
	if len(refs) < 2 {
		return false
	}
	var sawDatos, sawProps bool
	for _, ref := range refs {
		uri, _ := xmldsig.Attr(ref, "URI")
		expected := xmldsig.TrimmedText(xmldsig.ElementByLocalName(ref, "DigestValue"))
		if !strings.HasPrefix(uri, "#") || expected == "" {
			return false
		}
		target := xmldsig.ElementByID(root, uri[1:])
		if target == nil || xades.Digest(crypto.SHA256, target, inclusive) != expected {
			return false
		}
		if target.Tag == "DatosEmision" {
			sawDatos = true
		}
		if typ, _ := xmldsig.Attr(ref, "Type"); typ == xades.SignedPropertiesType && target.Tag == "SignedProperties" {
			sawProps = true
		}
	}
	return sawDatos && sawProps
}

// issuerSignature is the FirmaEmisor, else the first signature (not the
// certifier's) with a reference to DatosEmision.
func issuerSignature(root *etree.Element) *etree.Element {
	var fallback *etree.Element
	for _, s := range xmldsig.Descendants(root, xades.DSigNS, "Signature") {
		id, _ := xmldsig.Attr(s, "Id")
		if id == issuerSignatureID {
			return s
		}
		if id == certifierSignatureID || fallback != nil {
			continue
		}
		for _, ref := range xmldsig.Descendants(s, xades.DSigNS, "Reference") {
			uri, _ := xmldsig.Attr(ref, "URI")
			if !strings.HasPrefix(uri, "#") {
				continue
			}
			if t := xmldsig.ElementByID(root, uri[1:]); t != nil && t.Tag == "DatosEmision" {
				fallback = s
				break
			}
		}
	}
	return fallback
}

// metadata: the type is DatosGenerales/@Tipo; the id, the certifier's
// NumeroAutorizacion once certified, else DatosGenerales/@NumeroAcceso.
func metadata(body []byte) signing.Document {
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return signing.Document{}
	}
	root := doc.Root()
	var d signing.Document
	generales := xmldsig.ElementByLocalName(root, "DatosGenerales")
	if c := xmldsig.ElementByLocalName(root, "Certificacion"); c != nil {
		d.ID = xmldsig.TrimmedText(xmldsig.ElementByLocalName(c, "NumeroAutorizacion"))
	}
	if generales != nil {
		if d.ID == "" {
			v, _ := xmldsig.Attr(generales, "NumeroAcceso")
			d.ID = xmldsig.JSTrim(v)
		}
		v, _ := xmldsig.Attr(generales, "Tipo")
		d.Type = xmldsig.JSTrim(v)
	}
	return d
}
