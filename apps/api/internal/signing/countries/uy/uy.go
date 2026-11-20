// Package uy signs Uruguay's CFE for the DGI (Formato_CFE v25-2, Zona I): an
// enveloped XMLDSig with RSA-SHA1 and inclusive C14N, Reference URI="" with
// the transforms [enveloped-signature, inclusive C14N], SHA-1, KeyInfo with
// the certificate, the Signature unprefixed as the last child of each CFE.
// A bare CFE, a CFE_Adenda (its CFE; the Adenda stays outside) or an
// envelope such as EnvioCFE (every CFE in it) is accepted, and URI="" stands
// for the CFE the Signature is in, as in DGI's own ecosystem, not for the
// whole document. Whitespace-only text is dropped and an earlier signature
// replaced.
//
// It reproduces sign-old's uy.ts byte for byte, except where that one
// signed something its own verify rejected: a CFE that declares the XMLDSig
// namespace with a prefix (xmldom then wrote ds:Signature) gets the
// unprefixed Signature.
package uy

import (
	"crypto/x509/pkix"
	"errors"
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

func (country) Code() string { return "UY" }

// Sign signs every CFE of the document.
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
	targets, err := cfes(doc.Root())
	if err != nil {
		return signing.Signed{}, fmt.Errorf("%w: %v", signing.ErrInvalidDocument, err)
	}
	for _, cfe := range targets {
		for _, old := range signatures(cfe) {
			cfe.RemoveChild(old)
		}
		digest, _ := xmldsig.Digest(xmldsig.SHA1, c14n.Canonicalize(cfe, c14n.Options{}))
		sig := xmldsig.Template{
			CanonicalizationMethod: c14n.Inclusive,
			SignatureMethod:        xmldsig.RSASHA1,
			References: []xmldsig.Reference{{
				URI:          "",
				Transforms:   []string{xmldsig.EnvelopedSignature, c14n.Inclusive},
				DigestMethod: xmldsig.SHA1,
				DigestValue:  digest,
			}},
			Certificate: m.Certificate.Raw,
		}.Build()
		cfe.AddChild(sig.Element)
		xmldsig.Declare(sig.Element, "", xmldsig.NS)
		if err := sig.Sign(m.Key); err != nil {
			return signing.Signed{}, err
		}
	}
	if meta.ID, err = signing.DocumentIDOr(meta.ID, o); err != nil {
		return signing.Signed{}, err
	}
	return signing.Signed{Signature: xmldsig.Serialize(doc), ContentType: "application/xml", Document: meta}, nil
}

// cfes are the elements to sign: the root CFE, the CFE of a CFE_Adenda, or
// every CFE of an envelope.
func cfes(root *etree.Element) ([]*etree.Element, error) {
	switch root.Tag {
	case "CFE":
		return []*etree.Element{root}, nil
	case "CFE_Adenda":
		if c := xmldsig.ChildrenNamed(root, "CFE"); len(c) > 0 {
			return c[:1], nil
		}
		return nil, errors.New("CFE_Adenda without a CFE")
	}
	if found := xmldsig.Descendants(root, "*", "CFE"); len(found) > 0 {
		return found, nil
	}
	return nil, fmt.Errorf("expected <CFE>, <CFE_Adenda> or an envelope with <CFE>, found <%s>", root.Tag)
}

func signatures(cfe *etree.Element) []*etree.Element {
	var out []*etree.Element
	for _, c := range xmldsig.ChildrenNamed(cfe, "Signature") {
		if xmldsig.NamespaceURI(c) == xmldsig.NS {
			out = append(out, c)
		}
	}
	return out
}

// Verify checks every CFE, as sign-old did: each with exactly one Signature,
// its RSA-SHA1 SignatureValue over SignedInfo, and the SHA-1 digest of the
// CFE without it. A partly signed envelope is not valid.
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
	targets, err := cfes(doc.Root())
	if err != nil {
		return false
	}
	for _, cfe := range targets {
		sigs := signatures(cfe)
		if len(sigs) != 1 {
			return false
		}
		sig := sigs[0]
		signedInfos := xmldsig.Descendants(sig, xmldsig.NS, "SignedInfo")
		value := firstText(sig, "SignatureValue")
		if len(signedInfos) == 0 || value == "" {
			return false
		}
		data := c14n.Canonicalize(signedInfos[0], c14n.Options{})
		if xmldsig.VerifyValue(m.PublicKey(), xmldsig.RSASHA1, data, value) != nil {
			return false
		}
		want := firstText(signedInfos[0], "DigestValue")
		if want == "" {
			return false
		}
		var got string
		xmldsig.Detached(sig, func() {
			got, _ = xmldsig.Digest(xmldsig.SHA1, c14n.Canonicalize(cfe, c14n.Options{}))
		})
		if got != want {
			return false
		}
	}
	return true
}

// ParseCertificate reads a PKCS #12 or a PEM key with its certificate. The
// RUT (12 digits) is a 12-digit token of the subject's serialNumber, or all
// its digits if they are 12, or a 12-digit token of the CN, then of the OU.
func (country) ParseCertificate(u signing.Upload) (signing.Material, signing.CertificateInfo, error) {
	m, err := signing.ParseKeyAndCertificate(u)
	if err != nil {
		return signing.Material{}, signing.CertificateInfo{}, err
	}
	info := signing.Describe(m.Certificate)
	info.TaxID = RUT(m.Certificate.Subject.Names)
	return m, info, nil
}

var rutToken = regexp.MustCompile(`[0-9]{12}`)

// rutIn is the first run of exactly twelve digits not glued to letters or
// digits.
func rutIn(s string) string {
	for i := 0; i < len(s); {
		loc := rutToken.FindStringIndex(s[i:])
		if loc == nil {
			return ""
		}
		start, end := i+loc[0], i+loc[1]
		if (start == 0 || !isAlnum(s[start-1])) && (end == len(s) || !isAlnum(s[end])) {
			return s[start:end]
		}
		i = start + 1
	}
	return ""
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// metadata: Serie-Nro (or Nro) and TipoCFE of the first CFE's IdDoc.
func metadata(root *etree.Element) signing.Document {
	targets, err := cfes(root)
	if err != nil {
		return signing.Document{}
	}
	idDoc := targets[0]
	if found := xmldsig.Descendants(idDoc, "*", "IdDoc"); len(found) > 0 {
		idDoc = found[0]
	}
	d := signing.Document{Type: firstText(idDoc, "TipoCFE")}
	serie, nro := firstText(idDoc, "Serie"), firstText(idDoc, "Nro")
	if serie != "" && nro != "" {
		d.ID = serie + "-" + nro
	} else {
		d.ID = nro
	}
	return d
}

// firstText looks by local name in any namespace first, then by qualified
// name, as uy.ts did.
func firstText(el *etree.Element, local string) string {
	if found := xmldsig.Descendants(el, "*", local); len(found) > 0 {
		if t := xmldsig.TextContent(found[0]); t != "" {
			return xmldsig.JSTrim(t)
		}
	}
	if found := xmldsig.DescendantsByName(el, local); len(found) > 0 {
		if t := xmldsig.TextContent(found[0]); t != "" {
			return xmldsig.JSTrim(t)
		}
	}
	return ""
}

// RUT reads the taxpayer's RUT from subject attributes as sign-old's
// extractUruguayCertInfo did.
func RUT(names []pkix.AttributeTypeAndValue) string {
	subject := pkix.Name{Names: names}
	serial := signing.SubjectValue(subject, signing.OIDSerialNumber)
	if r := rutIn(serial); r != "" {
		return r
	}
	if d := signing.Digits(serial); len(d) == 12 {
		return d
	}
	if r := rutIn(signing.SubjectValue(subject, signing.OIDCommonName)); r != "" {
		return r
	}
	return rutIn(signing.SubjectValue(subject, signing.OIDOrganizationalUnit))
}
