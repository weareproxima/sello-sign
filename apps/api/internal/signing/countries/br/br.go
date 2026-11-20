// Package br signs Brazil's DF-e (NF-e, NFC-e, NFS-e Padrão Nacional, CT-e,
// MDF-e, NFCom, eventos…) as the MOC prescribes: an XMLDSig with RSA-SHA1
// and inclusive C14N whose Signature, unprefixed, is the sibling right
// after the signed node (the root's first inf… child with an Id) and points
// at it by Id, with the transforms [enveloped-signature, inclusive C14N],
// SHA-1, and KeyInfo with the certificate. Whitespace-only text is dropped
// and an earlier Signature of the root replaced.
//
// It reproduces sign-old's br.ts byte for byte, except for a carriage
// return in text (a "&#13;"), now written as a reference (see
// xmldsig.Serialize).
package br

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing"
	"proxima.la/sello/sign/internal/signing/c14n"
	"proxima.la/sello/sign/internal/signing/xmldsig"
)

// New returns the country's signer.
func New() signing.Country { return country{} }

type country struct{}

func (country) Code() string { return "BR" }

var safeID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// Sign signs a DF-e.
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
	root := doc.Root()
	xmldsig.StripWhitespace(&doc.Element)
	node := signedNode(root)
	if node == nil {
		return signing.Signed{}, fmt.Errorf("%w: <%s> has no <inf…> child with an Id", signing.ErrInvalidDocument, root.Tag)
	}
	id, _ := xmldsig.Attr(node, "Id")
	if !safeID.MatchString(id) {
		return signing.Signed{}, fmt.Errorf("%w: <%s Id=%q> is not an Id", signing.ErrInvalidDocument, node.Tag, id)
	}
	meta := metadata(root)
	for _, old := range rootSignatures(root) {
		root.RemoveChild(old)
	}
	digest, _ := xmldsig.Digest(xmldsig.SHA1, c14n.Canonicalize(node, c14n.Options{}))
	sig := xmldsig.Template{
		CanonicalizationMethod: c14n.Inclusive,
		SignatureMethod:        xmldsig.RSASHA1,
		References: []xmldsig.Reference{{
			URI:          "#" + id,
			Transforms:   []string{xmldsig.EnvelopedSignature, c14n.Inclusive},
			DigestMethod: xmldsig.SHA1,
			DigestValue:  digest,
		}},
		Certificate: m.Certificate.Raw,
	}.Build()
	// The template always declared the namespace, whatever the scope.
	sig.Element.CreateAttr("xmlns", xmldsig.NS)
	root.InsertChildAt(node.Index()+1, sig.Element)
	if err := sig.Sign(m.Key); err != nil {
		return signing.Signed{}, err
	}
	if meta.ID, err = signing.DocumentIDOr(meta.ID, o); err != nil {
		return signing.Signed{}, err
	}
	return signing.Signed{Signature: xmldsig.Serialize(doc), ContentType: "application/xml", Document: meta}, nil
}

// signedNode is the root's first child whose local name starts with "inf"
// and that has an Id.
func signedNode(root *etree.Element) *etree.Element {
	for _, c := range root.ChildElements() {
		if id, _ := xmldsig.Attr(c, "Id"); strings.HasPrefix(c.Tag, "inf") && id != "" {
			return c
		}
	}
	return nil
}

func rootSignatures(root *etree.Element) []*etree.Element {
	var out []*etree.Element
	for _, c := range xmldsig.ChildrenNamed(root, "Signature") {
		if xmldsig.NamespaceURI(c) == xmldsig.NS {
			out = append(out, c)
		}
	}
	return out
}

// Verify checks, as sign-old did, the root's Signature (or the first one
// anywhere, for envelopes such as procNFe): its RSA-SHA1 or RSA-SHA256
// SignatureValue over SignedInfo, then each Reference, which must point by
// Id at an inf… node whose SHA-1 or SHA-256 digest it holds.
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
	var sig *etree.Element
	if s := rootSignatures(root); len(s) > 0 {
		sig = s[0]
	} else if s := xmldsig.Descendants(root, xmldsig.NS, "Signature"); len(s) > 0 {
		sig = s[0]
	} else {
		return false
	}
	infos := xmldsig.ChildrenNamed(sig, "SignedInfo")
	value := firstText(sig, "SignatureValue")
	if len(infos) == 0 || value == "" {
		return false
	}
	signedInfo := infos[0]
	method, _ := xmldsig.Attr(byLocalName(signedInfo, "SignatureMethod"), "Algorithm")
	if method != xmldsig.RSASHA1 && method != xmldsig.RSASHA256 {
		return false
	}
	if xmldsig.VerifyValue(m.PublicKey(), method, c14n.Canonicalize(signedInfo, c14n.Options{}), value) != nil {
		return false
	}
	refs := xmldsig.Descendants(signedInfo, xmldsig.NS, "Reference")
	if len(refs) == 0 {
		return false
	}
	for _, ref := range refs {
		uri, _ := xmldsig.Attr(ref, "URI")
		want := firstText(ref, "DigestValue")
		digestMethod, _ := xmldsig.Attr(byLocalName(ref, "DigestMethod"), "Algorithm")
		if !strings.HasPrefix(uri, "#") || want == "" || (digestMethod != xmldsig.SHA1 && digestMethod != xmldsig.SHA256) {
			return false
		}
		target := xmldsig.ElementByID(root, uri[1:])
		if target == nil || !strings.HasPrefix(target.Tag, "inf") {
			return false
		}
		got, _ := xmldsig.Digest(digestMethod, c14n.Canonicalize(target, c14n.Options{}))
		if got != want {
			return false
		}
	}
	return true
}

// byLocalName is el itself or its first descendant with that local name.
func byLocalName(el *etree.Element, local string) *etree.Element {
	if el == nil {
		return nil
	}
	if el.Tag == local {
		return el
	}
	if found := xmldsig.Descendants(el, "*", local); len(found) > 0 {
		return found[0]
	}
	return nil
}

func firstText(el *etree.Element, local string) string {
	if found := byLocalName(el, local); found != nil {
		return xmldsig.JSTrim(xmldsig.TextContent(found))
	}
	return ""
}

// metadata: the signed node's Id without its letters (the chave de acesso,
// the DPS identifier, the event id), and the modelo of an NF-e/NFC-e
// (ide/mod) or else the root's name.
func metadata(root *etree.Element) signing.Document {
	node := signedNode(root)
	if node == nil {
		return signing.Document{}
	}
	id, _ := xmldsig.Attr(node, "Id")
	d := signing.Document{ID: strings.TrimLeftFunc(strings.TrimSpace(id), isLetter)}
	if node.Tag == "infNFe" {
		if ide := byLocalName(node, "ide"); ide != nil {
			d.Type = firstText(ide, "mod")
		}
	}
	if d.Type == "" {
		d.Type = root.Tag
	}
	return d
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
