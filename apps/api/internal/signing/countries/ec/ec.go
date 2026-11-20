// Package ec signs Ecuador's comprobantes electrónicos (SRI, esquema
// off-line): an enveloped XAdES-BES signature appended as the last child of
// the root comprobante (id="comprobante"), with inclusive canonicalization,
// RSA-SHA1 and SHA-1 digests as the SRI's ficha técnica requires, three
// references (SignedProperties, KeyInfo and the comprobante) and the ds and
// etsi prefixes declared on ds:Signature.
//
// It reproduces ../sign-old/src/lib/server/api/signers/ec.ts byte for byte,
// including what is odd but accepted: base64 wrapped at 76 columns (the
// certificate, the modulus and the signature value), empty elements written
// as <x></x>, the Ids with random numbers in the shapes of the SRI's own
// example (Signature620397, Certificate1562780…) and the X509IssuerName of
// xades.IssuerName.
package ec

import (
	"crypto"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xades"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

const comprobanteID = "comprobante"

var (
	inclusive = c14n.Options{}
	safeID    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
)

// codDoc maps the six root elements to their infoTributaria/codDoc.
var codDoc = map[string]string{
	"factura":              "01",
	"liquidacionCompra":    "03",
	"notaCredito":          "04",
	"notaDebito":           "05",
	"guiaRemision":         "06",
	"comprobanteRetencion": "07",
}

type country struct{}

// New returns the country's signer.
func New() signing.Country { return country{} }

func (country) Code() string { return "EC" }

// ParseCertificate reads the .p12 of an ECI accredited in Ecuador with its
// PIN (or a PEM bundle). The tax id is read as sign-old did for a country
// without its own rule: the digits of the subject serialNumber, else a run
// of 9 to 11 digits in the CN.
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := xades.ReadUpload(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := xades.Info(m.Certificate)
	info.TaxID = xades.GenericTaxID(m.Certificate)
	return m, info, nil
}

type ids struct {
	signature, signedInfo, signedPropertiesRef, signedProperties, keyInfo, comprobanteRef, signatureValue, object string
}

// newIDs draws the eight Id numbers in the order the TS signer did, each
// in 990…999989.
func newIDs(r io.Reader) (ids, error) {
	var n [8]string
	for i := range n {
		v, err := xades.RandomInt(r, 990, 999990)
		if err != nil {
			return ids{}, err
		}
		n[i] = strconv.FormatInt(v, 10)
	}
	sig := "Signature" + n[0]
	return ids{
		signature:           sig,
		signedInfo:          "Signature-SignedInfo" + n[1],
		signedPropertiesRef: "SignedPropertiesID" + n[2],
		signedProperties:    sig + "-SignedProperties" + n[3],
		keyInfo:             "Certificate" + n[4],
		comprobanteRef:      "Reference-ID-" + n[5],
		signatureValue:      "SignatureValue" + n[6],
		object:              sig + "-Object" + n[7],
	}, nil
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
	modulus, exponent, err := xades.RSAKeyValue(m.Certificate)
	if err != nil {
		return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidCertificate, err)
	}
	rnd := xades.Random(o)
	id, err := newIDs(rnd)
	if err != nil {
		return signing.Signed{}, err
	}
	cert.Base64 = xades.Wrap76(cert.Base64)
	props, err := xades.SignedProperties{
		Prefix:          "etsi",
		ID:              id.signedProperties,
		SigningTime:     xades.Offset(xades.Now(o), -5, false),
		DigestAlgorithm: xades.SHA1,
		Certificate:     cert,
		DataObjectFormat: &xades.DataObjectFormat{
			ObjectReference: "#" + id.comprobanteRef,
			Description:     "contenido comprobante",
			MimeType:        "text/xml",
		},
		EmptyElements: true,
	}.XML()
	if err != nil {
		return signing.Signed{}, err
	}

	before, after, err := prepare(body)
	if err != nil {
		return signing.Signed{}, err
	}
	keyValue := [2]string{xades.Wrap76(modulus), exponent}
	render := func(d [3]string, value string) string {
		return before + template(id, cert, keyValue, props, d, value) + after
	}

	doc, err := xmldsig.Parse([]byte(render([3]string{}, "")))
	if err != nil {
		return signing.Signed{}, err
	}
	root := doc.Root()
	sig := xmldsig.ElementByID(root, id.signature)
	propsEl := xmldsig.ElementByID(root, id.signedProperties)
	keyInfo := xmldsig.ElementByID(root, id.keyInfo)
	if sig == nil || propsEl == nil || keyInfo == nil {
		return signing.Signed{}, fmt.Errorf("%w: could not assemble the signature", signing.ErrInvalidDocument)
	}
	digests := [3]string{
		xades.Digest(crypto.SHA1, propsEl, inclusive),
		xades.Digest(crypto.SHA1, keyInfo, inclusive),
		comprobanteDigest(root, sig),
	}

	doc, err = xmldsig.Parse([]byte(render(digests, "")))
	if err != nil {
		return signing.Signed{}, err
	}
	sig = xmldsig.ElementByID(doc.Root(), id.signature)
	signedInfo := xmldsig.FirstDescendant(sig, xades.DSigNS, "SignedInfo")
	if signedInfo == nil {
		return signing.Signed{}, fmt.Errorf("%w: could not assemble the signature", signing.ErrInvalidDocument)
	}
	value, err := xades.SignRSA(m.Key, crypto.SHA1, c14n.Canonicalize(signedInfo, inclusive))
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
		Signature:   []byte(render(digests, xades.Wrap76(value))),
		ContentType: "application/xml",
		Document:    meta,
	}, nil
}

// prepare normalizes the comprobante, gives its root id="comprobante" when
// it has none, drops any previous signature and splits the serialized
// document where the new one goes: the root's last child.
func prepare(body []byte) (before, after string, err error) {
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err)
	}
	root := doc.Root()
	xmldsig.StripWhitespace(&doc.Element)
	if _, ok := codDoc[root.Tag]; !ok {
		return "", "", fmt.Errorf("%w: <%s> is not an SRI comprobante", signing.ErrInvalidDocument, root.Tag)
	}
	id, _ := xmldsig.Attr(root, "id")
	if id == "" {
		id, _ = xmldsig.Attr(root, "Id")
	}
	if id == "" {
		root.CreateAttr("id", comprobanteID)
		id = comprobanteID
	}
	if id != comprobanteID || !safeID.MatchString(id) {
		return "", "", fmt.Errorf("%w: <%s> must carry id=%q, found id=%q", signing.ErrInvalidDocument, root.Tag, comprobanteID, id)
	}
	for _, s := range xmldsig.ChildrenNS(root, xades.DSigNS, "Signature") {
		root.RemoveChild(s)
	}
	mark := etree.NewElement("mark")
	root.AddChild(mark)
	b, a := xmldsig.SerializeAround(doc, mark)
	before, after = string(b), string(a)
	return before, after, nil
}

// comprobanteDigest is reference 3: the enveloped-signature transform and
// inclusive canonicalization, the root without the signature.
func comprobanteDigest(root, sig *etree.Element) string {
	parent := sig.Parent()
	if parent == nil {
		return xades.Digest(crypto.SHA1, root, inclusive)
	}
	at := sig.Index()
	parent.RemoveChildAt(at)
	defer parent.InsertChildAt(at, sig)
	return xades.Digest(crypto.SHA1, root, inclusive)
}

func template(id ids, cert xades.Certificate, keyValue [2]string, props string, d [3]string, value string) string {
	method := func(alg string) string { return `<ds:DigestMethod Algorithm="` + alg + `"></ds:DigestMethod>` }
	return strings.Join([]string{
		`<ds:Signature xmlns:ds="`, xades.DSigNS, `" xmlns:etsi="`, xades.XAdESNS, `" Id="`, id.signature, `">`,
		`<ds:SignedInfo Id="`, id.signedInfo, `">`,
		`<ds:CanonicalizationMethod Algorithm="`, c14n.Inclusive, `"></ds:CanonicalizationMethod>`,
		`<ds:SignatureMethod Algorithm="`, xades.RSASHA1, `"></ds:SignatureMethod>`,
		`<ds:Reference Id="`, id.signedPropertiesRef, `" Type="`, xades.SignedPropertiesType, `" URI="#`, id.signedProperties, `">`,
		method(xades.SHA1), `<ds:DigestValue>`, d[0], `</ds:DigestValue>`,
		`</ds:Reference>`,
		`<ds:Reference URI="#`, id.keyInfo, `">`,
		method(xades.SHA1), `<ds:DigestValue>`, d[1], `</ds:DigestValue>`,
		`</ds:Reference>`,
		`<ds:Reference Id="`, id.comprobanteRef, `" URI="#`, comprobanteID, `">`,
		`<ds:Transforms>`,
		`<ds:Transform Algorithm="`, xades.EnvelopedSignature, `"></ds:Transform>`,
		`</ds:Transforms>`,
		method(xades.SHA1), `<ds:DigestValue>`, d[2], `</ds:DigestValue>`,
		`</ds:Reference>`,
		`</ds:SignedInfo>`,
		`<ds:SignatureValue Id="`, id.signatureValue, `">`, value, `</ds:SignatureValue>`,
		`<ds:KeyInfo Id="`, id.keyInfo, `">`,
		`<ds:X509Data>`,
		`<ds:X509Certificate>`, cert.Base64, `</ds:X509Certificate>`,
		`</ds:X509Data>`,
		`<ds:KeyValue>`,
		`<ds:RSAKeyValue>`,
		`<ds:Modulus>`, keyValue[0], `</ds:Modulus>`,
		`<ds:Exponent>`, keyValue[1], `</ds:Exponent>`,
		`</ds:RSAKeyValue>`,
		`</ds:KeyValue>`,
		`</ds:KeyInfo>`,
		`<ds:Object Id="`, id.object, `">`,
		`<etsi:QualifyingProperties Target="#`, id.signature, `">`,
		props,
		`</etsi:QualifyingProperties>`,
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
	sigs := xmldsig.ChildrenNS(root, xades.DSigNS, "Signature")
	if len(sigs) == 0 {
		return false
	}
	sig := sigs[0]
	signedInfo := xmldsig.FirstDescendant(sig, xades.DSigNS, "SignedInfo")
	value := xmldsig.TrimmedText(xmldsig.ElementByLocalName(sig, "SignatureValue"))
	if signedInfo == nil || value == "" {
		return false
	}
	if !xades.VerifyRSA(m.Certificate.PublicKey, crypto.SHA1, c14n.Canonicalize(signedInfo, inclusive), value) {
		return false
	}
	refs := xmldsig.Descendants(signedInfo, xades.DSigNS, "Reference")
	if len(refs) != 3 {
		return false
	}
	var sawComprobante, sawProps, sawKeyInfo bool
	for _, ref := range refs {
		uri, _ := xmldsig.Attr(ref, "URI")
		expected := xmldsig.TrimmedText(xmldsig.ElementByLocalName(ref, "DigestValue"))
		if !strings.HasPrefix(uri, "#") || expected == "" {
			return false
		}
		target := xmldsig.ElementByID(root, uri[1:])
		if target == nil {
			return false
		}
		var actual string
		if target == root {
			actual = comprobanteDigest(root, sig)
			sawComprobante = true
		} else {
			actual = xades.Digest(crypto.SHA1, target, inclusive)
		}
		if actual != expected {
			return false
		}
		if target.Tag == "KeyInfo" {
			sawKeyInfo = true
		}
		if typ, _ := xmldsig.Attr(ref, "Type"); typ == xades.SignedPropertiesType && target.Tag == "SignedProperties" {
			sawProps = true
		}
	}
	return sawComprobante && sawProps && sawKeyInfo
}

// metadata: the id is infoTributaria/claveAcceso; the type,
// infoTributaria/codDoc, else the one the root's name implies.
func metadata(body []byte) signing.Document {
	doc, err := xmldsig.Parse(body)
	if err != nil {
		return signing.Document{}
	}
	root := doc.Root()
	var d signing.Document
	if info := xmldsig.ElementByLocalName(root, "infoTributaria"); info != nil {
		d.ID = xmldsig.TrimmedText(xmldsig.ElementByLocalName(info, "claveAcceso"))
		d.Type = xmldsig.TrimmedText(xmldsig.ElementByLocalName(info, "codDoc"))
	}
	if d.Type == "" {
		d.Type = codDoc[root.Tag]
	}
	return d
}
