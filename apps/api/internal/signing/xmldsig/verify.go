package xmldsig

import (
	"crypto"
	"strings"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing/c14n"
)

// Verify checks sig, a ds:Signature of doc, as the XMLDSig recommendation
// does, against pub: the SignatureValue over SignedInfo canonicalized under
// its CanonicalizationMethod, then every Reference. A Reference is URI=""
// (the document) or "#id" (the element whose Id, ID or id is id); its
// transforms are enveloped-signature and the canonicalizations, inclusive
// or exclusive (with an InclusiveNamespaces PrefixList), and without one it
// is canonicalized inclusively. Comments never count: a same-document
// reference drops them.
func Verify(doc *etree.Document, sig *etree.Element, pub crypto.PublicKey) error {
	signedInfo := Child(sig, "SignedInfo")
	if signedInfo == nil {
		return invalid("no SignedInfo")
	}
	cm := Child(signedInfo, "CanonicalizationMethod")
	o, ok := c14n.OptionsFor(attr(cm, "Algorithm"))
	if !ok {
		return invalid("unsupported canonicalization %q", attr(cm, "Algorithm"))
	}
	o.InclusivePrefixes = prefixList(cm)
	value := Child(sig, "SignatureValue")
	if value == nil {
		return invalid("no SignatureValue")
	}
	sm := attr(Child(signedInfo, "SignatureMethod"), "Algorithm")
	if err := VerifyValue(pub, sm, c14n.Canonicalize(signedInfo, o), TextContent(value)); err != nil {
		return err
	}
	refs := ChildrenNamed(signedInfo, "Reference")
	if len(refs) == 0 {
		return invalid("no Reference")
	}
	for _, ref := range refs {
		if err := verifyReference(doc, sig, ref); err != nil {
			return err
		}
	}
	return nil
}

func verifyReference(doc *etree.Document, sig, ref *etree.Element) error {
	uri, _ := Attr(ref, "URI")
	var target *etree.Element
	switch {
	case uri == "":
		target = doc.Root()
	case strings.HasPrefix(uri, "#"):
		target = ElementByID(doc.Root(), uri[1:])
		if target == nil {
			return invalid("no element with Id %q", uri[1:])
		}
	default:
		return invalid("unsupported Reference URI %q", uri)
	}
	o := c14n.Options{}
	enveloped := false
	if ts := Child(ref, "Transforms"); ts != nil {
		for _, t := range ChildrenNamed(ts, "Transform") {
			alg := attr(t, "Algorithm")
			if alg == EnvelopedSignature {
				enveloped = true
				continue
			}
			to, ok := c14n.OptionsFor(alg)
			if !ok {
				return invalid("unsupported transform %q", alg)
			}
			o = to
			o.InclusivePrefixes = prefixList(t)
		}
	}
	o.Comments = false
	method := attr(Child(ref, "DigestMethod"), "Algorithm")
	var data []byte
	canon := func() {
		if uri == "" {
			data = CanonicalDocument(doc, o)
		} else {
			data = c14n.Canonicalize(target, o)
		}
	}
	if enveloped && isWithin(sig, target) {
		Detached(sig, canon)
	} else {
		canon()
	}
	got, err := Digest(method, data)
	if err != nil {
		return invalid("%v", err)
	}
	if want := stripSpace(TextContent(Child(ref, "DigestValue"))); got != want {
		return invalid("digest of %q does not match", uri)
	}
	return nil
}

// prefixList reads the InclusiveNamespaces PrefixList under a
// CanonicalizationMethod or Transform.
func prefixList(el *etree.Element) []string {
	if el == nil {
		return nil
	}
	for _, c := range el.ChildElements() {
		if c.Tag == "InclusiveNamespaces" {
			return strings.Fields(attr(c, "PrefixList"))
		}
	}
	return nil
}

func isWithin(el, ancestor *etree.Element) bool {
	for e := el; e != nil; e = e.Parent() {
		if e == ancestor {
			return true
		}
	}
	return false
}

// ElementByID finds, in document order from root (itself included), the
// element whose Id, ID or id attribute is id.
func ElementByID(root *etree.Element, id string) *etree.Element {
	if root == nil {
		return nil
	}
	for _, k := range []string{"Id", "ID", "id"} {
		if v, ok := Attr(root, k); ok && v == id {
			return root
		}
	}
	for _, c := range root.ChildElements() {
		if found := ElementByID(c, id); found != nil {
			return found
		}
	}
	return nil
}
