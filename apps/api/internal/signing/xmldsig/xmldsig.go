package xmldsig

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/beevik/etree"

	"proxima.la/sello/sign/internal/signing/c14n"
)

// NS is the XMLDSig namespace.
const NS = "http://www.w3.org/2000/09/xmldsig#"

// Algorithm URIs.
const (
	EnvelopedSignature = NS + "enveloped-signature"

	SHA1   = NS + "sha1"
	SHA256 = "http://www.w3.org/2001/04/xmlenc#sha256"
	SHA512 = "http://www.w3.org/2001/04/xmlenc#sha512"

	RSASHA1   = NS + "rsa-sha1"
	RSASHA256 = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	RSASHA512 = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha512"
)

// ErrInvalid is what a signature that does not check out wraps.
var ErrInvalid = errors.New("xmldsig: invalid signature")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// DigestHash is the hash behind a DigestMethod URI.
func DigestHash(uri string) (crypto.Hash, bool) {
	switch uri {
	case SHA1:
		return crypto.SHA1, true
	case SHA256:
		return crypto.SHA256, true
	case SHA512:
		return crypto.SHA512, true
	}
	return 0, false
}

// SignatureHash is the hash behind an RSA SignatureMethod URI.
func SignatureHash(uri string) (crypto.Hash, bool) {
	switch uri {
	case RSASHA1:
		return crypto.SHA1, true
	case RSASHA256:
		return crypto.SHA256, true
	case RSASHA512:
		return crypto.SHA512, true
	}
	return 0, false
}

func sum(h crypto.Hash, data []byte) []byte {
	switch h {
	case crypto.SHA1:
		s := sha1.Sum(data)
		return s[:]
	case crypto.SHA256:
		s := sha256.Sum256(data)
		return s[:]
	case crypto.SHA512:
		s := sha512.Sum512(data)
		return s[:]
	}
	panic("xmldsig: unsupported hash")
}

// Digest is the base64 digest of data under a DigestMethod URI.
func Digest(method string, data []byte) (string, error) {
	h, ok := DigestHash(method)
	if !ok {
		return "", fmt.Errorf("xmldsig: unsupported digest method %s", method)
	}
	return base64.StdEncoding.EncodeToString(sum(h, data)), nil
}

// SignValue is the base64 PKCS #1 v1.5 signature of data under an RSA
// SignatureMethod URI. It is deterministic.
func SignValue(key crypto.Signer, method string, data []byte) (string, error) {
	h, ok := SignatureHash(method)
	if !ok {
		return "", fmt.Errorf("xmldsig: unsupported signature method %s", method)
	}
	sig, err := key.Sign(rand.Reader, sum(h, data), h)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyValue checks a base64 RSA signature value (whitespace allowed) of data.
func VerifyValue(pub crypto.PublicKey, method string, data []byte, value string) error {
	h, ok := SignatureHash(method)
	if !ok {
		return invalid("unsupported signature method %s", method)
	}
	key, ok := pub.(*rsa.PublicKey)
	if !ok {
		return invalid("the key is not RSA")
	}
	sig, err := base64.StdEncoding.DecodeString(stripSpace(value))
	if err != nil {
		return invalid("SignatureValue is not base64")
	}
	if err := rsa.VerifyPKCS1v15(key, h, sum(h, data), sig); err != nil {
		return invalid("SignatureValue does not match SignedInfo")
	}
	return nil
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if isXMLSpace(r) {
			return -1
		}
		return r
	}, s)
}

// Canonicalize canonicalizes el in its document under a
// CanonicalizationMethod or transform URI.
func Canonicalize(el *etree.Element, method string) ([]byte, error) {
	o, ok := c14n.OptionsFor(method)
	if !ok {
		return nil, fmt.Errorf("xmldsig: unsupported canonicalization %s", method)
	}
	return c14n.Canonicalize(el, o), nil
}

// CanonicalDocument canonicalizes a whole document, as a Reference with
// URI="" selects it: the root element plus the processing instructions
// outside it (never the XML declaration), and their comments only with
// o.Comments.
func CanonicalDocument(doc *etree.Document, o c14n.Options) []byte {
	var b bytes.Buffer
	root := doc.Root()
	after := false
	for _, t := range doc.Child {
		var node string
		switch n := t.(type) {
		case *etree.Element:
			b.Write(c14n.Canonicalize(n, o))
			after = n == root
			continue
		case *etree.ProcInst:
			if strings.EqualFold(n.Target, "xml") {
				continue
			}
			node = "<?" + n.Target
			if n.Inst != "" {
				node += " " + n.Inst
			}
			node += "?>"
		case *etree.Comment:
			if !o.Comments {
				continue
			}
			node = "<!--" + n.Data + "-->"
		default:
			continue
		}
		if after {
			b.WriteString("\n" + node)
		} else {
			b.WriteString(node + "\n")
		}
	}
	return b.Bytes()
}

// Detached runs f with el taken out of its parent, and puts it back where it
// was: the enveloped-signature transform over a live document.
func Detached(el *etree.Element, f func()) {
	parent := el.Parent()
	if parent == nil {
		f()
		return
	}
	i := el.Index()
	parent.RemoveChildAt(i)
	defer parent.InsertChildAt(i, el)
	f()
}

// Reference is one ds:Reference of a template.
type Reference struct {
	URI          string
	Transforms   []string
	DigestMethod string
	DigestValue  string
}

// Template describes the ds:Signature a country writes, in the order and
// shape the TypeScript signers built it: SignedInfo (CanonicalizationMethod,
// SignatureMethod, the References), SignatureValue and, with a certificate,
// KeyInfo/X509Data/X509Certificate.
type Template struct {
	// Prefix is "" (the default namespace) or, say, "ds".
	Prefix string
	// ID is the Signature's Id attribute, if any.
	ID                     string
	CanonicalizationMethod string
	SignatureMethod        string
	References             []Reference
	// Certificate is the DER of the signer's certificate.
	Certificate []byte
}

// Signature is a built ds:Signature and the elements a signer fills.
type Signature struct {
	Element        *etree.Element
	SignedInfo     *etree.Element
	SignatureValue *etree.Element
	DigestValues   []*etree.Element
}

// Build builds the elements of t without any namespace declaration: once
// the Signature is in its document, Declare(sig.Element, t.Prefix, NS).
func (t Template) Build() *Signature {
	el := func(parent *etree.Element, local string) *etree.Element {
		e := etree.NewElement(qualified(t.Prefix, local))
		if parent != nil {
			parent.AddChild(e)
		}
		return e
	}
	s := &Signature{Element: el(nil, "Signature")}
	if t.ID != "" {
		s.Element.CreateAttr("Id", t.ID)
	}
	s.SignedInfo = el(s.Element, "SignedInfo")
	el(s.SignedInfo, "CanonicalizationMethod").CreateAttr("Algorithm", t.CanonicalizationMethod)
	el(s.SignedInfo, "SignatureMethod").CreateAttr("Algorithm", t.SignatureMethod)
	for _, r := range t.References {
		ref := el(s.SignedInfo, "Reference")
		ref.CreateAttr("URI", r.URI)
		if len(r.Transforms) > 0 {
			ts := el(ref, "Transforms")
			for _, tr := range r.Transforms {
				el(ts, "Transform").CreateAttr("Algorithm", tr)
			}
		}
		el(ref, "DigestMethod").CreateAttr("Algorithm", r.DigestMethod)
		dv := el(ref, "DigestValue")
		SetText(dv, r.DigestValue)
		s.DigestValues = append(s.DigestValues, dv)
	}
	s.SignatureValue = el(s.Element, "SignatureValue")
	if t.Certificate != nil {
		x509Data := el(el(s.Element, "KeyInfo"), "X509Data")
		SetText(el(x509Data, "X509Certificate"), base64.StdEncoding.EncodeToString(t.Certificate))
	}
	return s
}

// Sign canonicalizes SignedInfo where it sits, under its declared method,
// and fills SignatureValue.
func (s *Signature) Sign(key crypto.Signer) error {
	method := attr(Child(s.SignedInfo, "CanonicalizationMethod"), "Algorithm")
	data, err := Canonicalize(s.SignedInfo, method)
	if err != nil {
		return err
	}
	value, err := SignValue(key, attr(Child(s.SignedInfo, "SignatureMethod"), "Algorithm"), data)
	if err != nil {
		return err
	}
	SetText(s.SignatureValue, value)
	return nil
}

// SetText replaces el's children with one text node, or none for "", as
// textContent does.
func SetText(el *etree.Element, text string) {
	for len(el.Child) > 0 {
		el.RemoveChildAt(0)
	}
	if text != "" {
		el.AddChild(etree.NewText(text))
	}
}

// TextContent is el's textContent: its text and CDATA descendants, in order.
func TextContent(el *etree.Element) string {
	var b strings.Builder
	var walk func(*etree.Element)
	walk = func(e *etree.Element) {
		for _, c := range e.Child {
			switch t := c.(type) {
			case *etree.CharData:
				b.WriteString(t.Data)
			case *etree.Element:
				walk(t)
			}
		}
	}
	walk(el)
	return b.String()
}

// LocalName is el's tag without its prefix.
func LocalName(el *etree.Element) string { return el.Tag }

// Children are el's child elements, in order.
func Children(el *etree.Element) []*etree.Element { return el.ChildElements() }

// ChildrenNamed are el's child elements with that local name, any namespace.
func ChildrenNamed(el *etree.Element, local string) []*etree.Element {
	var out []*etree.Element
	for _, c := range el.ChildElements() {
		if c.Tag == local {
			out = append(out, c)
		}
	}
	return out
}

// Descendants are el's descendant elements (not el) in document order whose
// local name is local and, unless ns is "*", whose namespace is ns:
// getElementsByTagNameNS.
func Descendants(el *etree.Element, ns, local string) []*etree.Element {
	var out []*etree.Element
	var walk func(*etree.Element)
	walk = func(e *etree.Element) {
		for _, c := range e.ChildElements() {
			if c.Tag == local && (ns == "*" || NamespaceURI(c) == ns) {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(el)
	return out
}

// DescendantsByName are el's descendants whose qualified name is name:
// getElementsByTagName.
func DescendantsByName(el *etree.Element, name string) []*etree.Element {
	var out []*etree.Element
	var walk func(*etree.Element)
	walk = func(e *etree.Element) {
		for _, c := range e.ChildElements() {
			if qualified(c.Space, c.Tag) == name {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(el)
	return out
}

// Child is el's first child element with that local name, any namespace;
// nil when el is nil or has none.
func Child(el *etree.Element, local string) *etree.Element {
	if el == nil {
		return nil
	}
	for _, c := range el.ChildElements() {
		if c.Tag == local {
			return c
		}
	}
	return nil
}

// ChildrenNS are el's child elements with that namespace and local name.
func ChildrenNS(el *etree.Element, ns, local string) []*etree.Element {
	var out []*etree.Element
	for _, c := range el.ChildElements() {
		if c.Tag == local && NamespaceURI(c) == ns {
			out = append(out, c)
		}
	}
	return out
}

// FirstDescendant is the first of Descendants(el, ns, local), or nil (also
// for a nil el).
func FirstDescendant(el *etree.Element, ns, local string) *etree.Element {
	if el == nil {
		return nil
	}
	if found := Descendants(el, ns, local); len(found) > 0 {
		return found[0]
	}
	return nil
}

// ElementByLocalName finds, in document order from root (itself included),
// the first element with that local name, any namespace.
func ElementByLocalName(root *etree.Element, local string) *etree.Element {
	if root == nil {
		return nil
	}
	if root.Tag == local {
		return root
	}
	for _, c := range root.ChildElements() {
		if found := ElementByLocalName(c, local); found != nil {
			return found
		}
	}
	return nil
}

// TrimmedText is el's textContent.trim(), "" for a nil el.
func TrimmedText(el *etree.Element) string {
	if el == nil {
		return ""
	}
	return JSTrim(TextContent(el))
}

func attr(el *etree.Element, key string) string {
	if el == nil {
		return ""
	}
	for _, a := range el.Attr {
		if a.Space == "" && a.Key == key {
			return a.Value
		}
	}
	return ""
}

// Attr is the value of el's unprefixed attribute key, and whether it has it.
func Attr(el *etree.Element, key string) (string, bool) {
	for _, a := range el.Attr {
		if a.Space == "" && a.Key == key {
			return a.Value, true
		}
	}
	return "", false
}

// SignDocument signs a whole document the way DO and BO do: the single
// Reference of t (URI="", enveloped) gets the digest of the document as it
// is, the Signature becomes the root's last child, declares its namespace,
// and SignedInfo is signed where it sits. Only the first Reference is
// filled.
func SignDocument(doc *etree.Document, t Template, key crypto.Signer) error {
	digest, err := Digest(t.References[0].DigestMethod, CanonicalDocument(doc, c14n.Options{}))
	if err != nil {
		return err
	}
	t.References[0].DigestValue = digest
	sig := t.Build()
	doc.Root().AddChild(sig.Element)
	Declare(sig.Element, t.Prefix, NS)
	return sig.Sign(key)
}

// FirstText is the trimmed text of the first descendant of root named local,
// as the TypeScript signers' getElementText found it: first by qualified
// name (getElementsByTagName), then by local name in any namespace, each
// only if its text is not empty. "" when there is none.
func FirstText(root *etree.Element, local string) string {
	if found := DescendantsByName(root, local); len(found) > 0 {
		if text := TextContent(found[0]); text != "" {
			return JSTrim(text)
		}
	}
	if found := Descendants(root, "*", local); len(found) > 0 {
		if text := TextContent(found[0]); text != "" {
			return JSTrim(text)
		}
	}
	return ""
}

// FirstSignature is the first ds:Signature of doc in document order.
func FirstSignature(doc *etree.Document) *etree.Element {
	if root := doc.Root(); root != nil {
		if root.Tag == "Signature" && NamespaceURI(root) == NS {
			return root
		}
		if found := Descendants(root, NS, "Signature"); len(found) > 0 {
			return found[0]
		}
	}
	return nil
}
