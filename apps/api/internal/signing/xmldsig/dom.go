package xmldsig

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/beevik/etree"
)

// ErrMalformed wraps every reason Parse rejects a body.
var ErrMalformed = errors.New("xmldsig: malformed XML")

func malformed(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrMalformed, fmt.Sprintf(format, a...))
}

// Parse reads a document the way the TypeScript signers did, with
// @xmldom/xmldom 0.9's DOMParser in text/xml mode, so that what Serialize
// writes back is what they wrote:
//
//   - a leading byte order mark is dropped (the HTTP layer decoded the body
//     with TextDecoder) and the body must be UTF-8, whatever its declaration
//     says;
//   - line endings are normalized as xmldom's normalizeLineEndings does:
//     "\r\n", "\r\u0085", "\r", U+0085, U+2028 and U+2029 become "\n";
//   - in attribute values, literal tabs and newlines become spaces before
//     references are expanded, so "&#10;" survives;
//   - references: the five predefined entities and character references are
//     expanded; anything else that starts with "&" (an undefined entity, a
//     bare "&" as in "AT&T") is kept as literal text, as xmldom does after
//     reporting a recoverable error;
//   - CDATA sections stay CDATA (etree.NewCData), adjacent text is one node,
//     and whitespace outside the root element is kept at document level;
//   - the XML declaration is a processing instruction ("xml") and must be at
//     the very start.
//
// What xmldom rejects as fatal is rejected (mismatched or unclosed tags, a
// repeated attribute, "<" in an attribute value, an unbound prefix), and so
// is a DOCTYPE: no fiscal document carries one, and refusing it rules out
// entity expansion attacks.
func Parse(body []byte) (*etree.Document, error) {
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(body) {
		return nil, malformed("the body is not UTF-8")
	}
	p := parser{s: normalizeLineEndings(string(body)), doc: etree.NewDocument()}
	if err := p.run(); err != nil {
		return nil, err
	}
	if p.doc.Root() == nil {
		return nil, malformed("no document element")
	}
	return p.doc, nil
}

var lineEndings = strings.NewReplacer(
	"\r\n", "\n", "\r\u0085", "\n",
	"\r", "\n", "\u0085", "\n", "\u2028", "\n", "\u2029", "\n",
)

func normalizeLineEndings(s string) string { return lineEndings.Replace(s) }

type parser struct {
	s     string
	pos   int
	doc   *etree.Document
	stack []*etree.Element
	text  strings.Builder
}

func (p *parser) current() *etree.Element {
	if len(p.stack) == 0 {
		return &p.doc.Element
	}
	return p.stack[len(p.stack)-1]
}

// flush appends the pending text as one node: xmldom merges adjacent text
// when it normalizes the document.
func (p *parser) flush() {
	if p.text.Len() == 0 {
		return
	}
	data := p.text.String()
	p.text.Reset()
	if len(p.stack) == 0 && strings.TrimFunc(data, isJSSpace) != "" {
		// xmldom drops non-blank text outside the root (after an error).
		return
	}
	p.current().AddChild(etree.NewText(data))
}

func (p *parser) run() error {
	for p.pos < len(p.s) {
		lt := strings.IndexByte(p.s[p.pos:], '<')
		if lt < 0 {
			p.text.WriteString(expandReferences(p.s[p.pos:]))
			p.pos = len(p.s)
			break
		}
		if lt > 0 {
			p.text.WriteString(expandReferences(p.s[p.pos : p.pos+lt]))
			p.pos += lt
		}
		rest := p.s[p.pos:]
		var err error
		switch {
		case strings.HasPrefix(rest, "</"):
			err = p.endTag()
		case strings.HasPrefix(rest, "<?"):
			err = p.procInst()
		case strings.HasPrefix(rest, "<!--"):
			err = p.comment()
		case strings.HasPrefix(rest, "<![CDATA["):
			err = p.cdata()
		case strings.HasPrefix(rest, "<!"):
			err = malformed("DOCTYPE and other declarations are not accepted")
		default:
			err = p.startTag()
		}
		if err != nil {
			return err
		}
	}
	p.flush()
	if len(p.stack) > 0 {
		return malformed("unclosed tag <%s>", qualified(p.current().Space, p.current().Tag))
	}
	return nil
}

func (p *parser) endTag() error {
	end := strings.IndexByte(p.s[p.pos:], '>')
	if end < 0 {
		return malformed("unterminated end tag")
	}
	name := strings.TrimRightFunc(p.s[p.pos+2:p.pos+end], isXMLSpace)
	if len(p.stack) == 0 {
		return malformed("end tag </%s> without a start tag", name)
	}
	el := p.current()
	if name != qualified(el.Space, el.Tag) {
		return malformed("tag mismatch: <%s> closed by </%s>", qualified(el.Space, el.Tag), name)
	}
	p.flush()
	p.stack = p.stack[:len(p.stack)-1]
	p.pos += end + 1
	return nil
}

func (p *parser) procInst() error {
	end := strings.Index(p.s[p.pos:], "?>")
	if end < 0 {
		return malformed("unterminated processing instruction")
	}
	body := p.s[p.pos+2 : p.pos+end]
	target := body
	data := ""
	if i := strings.IndexFunc(body, isXMLSpace); i >= 0 {
		target = body[:i]
		data = strings.TrimLeftFunc(body[i:], isXMLSpace)
	}
	if !isName(target) {
		return malformed("bad processing instruction target %q", target)
	}
	if strings.EqualFold(target, "xml") && p.pos > 0 {
		return malformed("the XML declaration is only allowed at the start")
	}
	p.flush()
	p.current().AddChild(etree.NewProcInst(target, data))
	p.pos += end + 2
	return nil
}

func (p *parser) comment() error {
	end := strings.Index(p.s[p.pos+4:], "-->")
	if end < 0 {
		return malformed("unterminated comment")
	}
	p.flush()
	p.current().AddChild(etree.NewComment(p.s[p.pos+4 : p.pos+4+end]))
	p.pos += 4 + end + 3
	return nil
}

func (p *parser) cdata() error {
	if len(p.stack) == 0 {
		return malformed("CDATA outside the root element")
	}
	start := p.pos + len("<![CDATA[")
	end := strings.Index(p.s[start:], "]]>")
	if end < 0 {
		return malformed("unterminated CDATA section")
	}
	p.flush()
	p.current().AddChild(etree.NewCData(p.s[start : start+end]))
	p.pos = start + end + 3
	return nil
}

func (p *parser) startTag() error {
	if len(p.stack) == 0 && p.doc.Root() != nil {
		return malformed("content after the document element")
	}
	i := p.pos + 1
	nameEnd := i
	for nameEnd < len(p.s) && !isXMLSpace(rune(p.s[nameEnd])) && p.s[nameEnd] != '>' && p.s[nameEnd] != '/' {
		nameEnd++
	}
	name := p.s[i:nameEnd]
	if !isQName(name) {
		return malformed("bad tag name %q", name)
	}
	el := etree.NewElement(name)
	i = nameEnd
	closed := false
	for {
		start := i
		for i < len(p.s) && isXMLSpace(rune(p.s[i])) {
			i++
		}
		if i >= len(p.s) {
			return malformed("unterminated start tag <%s>", name)
		}
		if p.s[i] == '>' {
			i++
			break
		}
		if strings.HasPrefix(p.s[i:], "/>") {
			i += 2
			closed = true
			break
		}
		if i == start {
			return malformed("missing whitespace before an attribute of <%s>", name)
		}
		eq := strings.IndexByte(p.s[i:], '=')
		if eq < 0 {
			return malformed("attribute without a value in <%s>", name)
		}
		key := strings.TrimRightFunc(p.s[i:i+eq], isXMLSpace)
		if !isQName(key) {
			return malformed("bad attribute name %q", key)
		}
		i += eq + 1
		for i < len(p.s) && isXMLSpace(rune(p.s[i])) {
			i++
		}
		if i >= len(p.s) || (p.s[i] != '"' && p.s[i] != '\'') {
			return malformed("unquoted value for %s", key)
		}
		quote := p.s[i]
		end := strings.IndexByte(p.s[i+1:], quote)
		if end < 0 {
			return malformed("unterminated value for %s", key)
		}
		raw := p.s[i+1 : i+1+end]
		if strings.IndexByte(raw, '<') >= 0 {
			return malformed("'<' in the value of %s", key)
		}
		for _, a := range el.Attr {
			if qualified(a.Space, a.Key) == key {
				return malformed("attribute %s repeated", key)
			}
		}
		el.CreateAttr(key, expandReferences(attrWhitespace.Replace(raw)))
		i += 1 + end + 1
	}
	p.flush()
	p.current().AddChild(el)
	if err := checkBindings(el); err != nil {
		return err
	}
	p.pos = i
	if !closed {
		p.stack = append(p.stack, el)
	}
	return nil
}

var attrWhitespace = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// checkBindings rejects prefixes nothing declares, as xmldom does.
func checkBindings(el *etree.Element) error {
	for _, a := range el.Attr {
		if a.Space == "xmlns" && a.Value == "" {
			return malformed("xmlns:%s cannot be empty", a.Key)
		}
	}
	if el.Space != "" && lookupPrefix(el, el.Space) == "" {
		return malformed("unbound prefix %s", el.Space)
	}
	for _, a := range el.Attr {
		if a.Space != "" && a.Space != "xmlns" && a.Space != "xml" && lookupPrefix(el, a.Space) == "" {
			return malformed("unbound prefix %s", a.Space)
		}
	}
	return nil
}

// expandReferences does what xmldom's entityReplacer does over /&#?\w+;?/:
// a well-formed reference to a predefined entity or a character becomes its
// text, anything else stays as written.
func expandReferences(s string) string {
	if strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '&')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		s = s[i:]
		// The candidate: "&", an optional "#", word characters, an optional ";".
		j := 1
		if j < len(s) && s[j] == '#' {
			j++
		}
		k := j
		for k < len(s) && isWordByte(s[k]) {
			k++
		}
		if k == j {
			b.WriteByte('&')
			s = s[1:]
			continue
		}
		if k < len(s) && s[k] == ';' {
			if r, ok := reference(s[1:k]); ok {
				b.WriteString(r)
				s = s[k+1:]
				continue
			}
			k++
		}
		b.WriteString(s[:k])
		s = s[k:]
	}
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

var predefined = map[string]string{"amp": "&", "lt": "<", "gt": ">", "quot": `"`, "apos": "'"}

// reference resolves name, the text between "&" and ";".
func reference(name string) (string, bool) {
	if r, ok := predefined[name]; ok {
		return r, true
	}
	if !strings.HasPrefix(name, "#") {
		return "", false
	}
	digits, base := name[1:], 10
	if strings.HasPrefix(digits, "x") {
		digits, base = digits[1:], 16
	}
	if digits == "" {
		return "", false
	}
	for _, c := range digits {
		if !(c >= '0' && c <= '9' || base == 16 && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')) {
			return "", false
		}
	}
	n, err := strconv.ParseUint(digits, base, 32)
	if err != nil || n > utf8.MaxRune {
		return "", false
	}
	r := rune(n)
	if r >= 0xD800 && r <= 0xDFFF {
		// JavaScript keeps a lone surrogate; encoded as UTF-8 it is U+FFFD.
		r = utf8.RuneError
	}
	return string(r), true
}

func isXMLSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

func isQName(s string) bool {
	if s == "" || strings.Count(s, ":") > 1 {
		return false
	}
	for _, part := range strings.Split(s, ":") {
		if !isName(part) {
			return false
		}
	}
	return true
}

func isName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r == ':' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= 0xC0:
		case i > 0 && (r == '-' || r == '.' || r >= '0' && r <= '9' || r == 0xB7):
		default:
			return false
		}
	}
	return true
}

// StripWhitespace removes, at every level under n (an element, or
// &doc.Element for the whole document), the text nodes that are only
// whitespace as JavaScript's String.prototype.trim sees it, as the
// TypeScript signers did before signing. CDATA sections stay.
func StripWhitespace(n *etree.Element) {
	for i := len(n.Child) - 1; i >= 0; i-- {
		switch t := n.Child[i].(type) {
		case *etree.CharData:
			if !t.IsCData() && strings.TrimFunc(t.Data, isJSSpace) == "" {
				n.RemoveChildAt(i)
			}
		case *etree.Element:
			StripWhitespace(t)
		}
	}
}

// JSTrim trims what JavaScript's trim() trims: its WhiteSpace and
// LineTerminator characters, which are not quite unicode.IsSpace (no U+0085,
// but U+FEFF).
func JSTrim(s string) string { return strings.TrimFunc(s, isJSSpace) }

func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680', '\u2028', '\u2029',
		'\u202f', '\u205f', '\u3000', '\ufeff':
		return true
	}
	return r >= '\u2000' && r <= '\u200a'
}

// Serialize writes doc as xmldom's XMLSerializer.serializeToString does for
// a parsed document: names and attributes as they are, in their order,
// double-quoted, `<x/>` for an element without children; except that a
// carriage return in text (only a "&#13;" can put one there) is written
// "&#13;", where xmldom wrote it raw and broke the signature. It adds no
// namespace declarations: an element built by a signer declares its own (see
// Declare).
func Serialize(doc *etree.Document) []byte {
	var b bytes.Buffer
	s := serializer{w: &b}
	for _, c := range doc.Child {
		s.write(c)
	}
	return b.Bytes()
}

// SerializeAround is Serialize with mark left out: what comes before it and
// what comes after, for a signer that splices a template in its place.
func SerializeAround(doc *etree.Document, mark *etree.Element) (before, after []byte) {
	var b, a bytes.Buffer
	s := serializer{w: &b, mark: mark, after: &a}
	for _, c := range doc.Child {
		s.write(c)
	}
	return b.Bytes(), a.Bytes()
}

type serializer struct {
	w     *bytes.Buffer
	mark  *etree.Element
	after *bytes.Buffer
}

func (s *serializer) write(t etree.Token) {
	switch n := t.(type) {
	case *etree.Element:
		if s.mark != nil && n == s.mark {
			s.w = s.after
			return
		}
		name := qualified(n.Space, n.Tag)
		s.w.WriteString("<" + name)
		for _, a := range n.Attr {
			s.w.WriteString(" " + qualified(a.Space, a.Key) + `="` + attrEscaper.Replace(a.Value) + `"`)
		}
		if len(n.Child) == 0 {
			s.w.WriteString("/>")
			return
		}
		s.w.WriteString(">")
		for _, c := range n.Child {
			s.write(c)
		}
		s.w.WriteString("</" + name + ">")
	case *etree.CharData:
		if n.IsCData() {
			s.w.WriteString("<![CDATA[" + strings.ReplaceAll(n.Data, "]]>", "]]]]><![CDATA[>") + "]]>")
		} else {
			s.w.WriteString(textEscaper.Replace(n.Data))
		}
	case *etree.Comment:
		s.w.WriteString("<!--" + n.Data + "-->")
	case *etree.ProcInst:
		s.w.WriteString("<?" + n.Target + " " + n.Inst + "?>")
	}
}

func qualified(space, local string) string {
	if space == "" {
		return local
	}
	return space + ":" + local
}

var (
	// xmldom wrote a carriage return in text as it is, which every parser
	// reads back as a line feed: the digest, taken over the carriage
	// return, no longer matched. It is written as a reference instead.
	textEscaper = strings.NewReplacer("<", "&lt;", "&", "&amp;", ">", "&gt;", "\r", "&#13;")
	attrEscaper = strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;", `"`, "&quot;", "\t", "&#9;", "\n", "&#10;", "\r", "&#13;")
)

// EscapeText escapes s as Serialize escapes a text node.
func EscapeText(s string) string { return textEscaper.Replace(s) }

// EscapeAttr escapes s as xmldom escapes a double-quoted attribute value.
func EscapeAttr(s string) string { return attrEscaper.Replace(s) }

// lookupPrefix resolves prefix in el's scope ("" when unbound).
func lookupPrefix(el *etree.Element, prefix string) string {
	if prefix == "xml" {
		return "http://www.w3.org/XML/1998/namespace"
	}
	for e := el; e != nil; e = e.Parent() {
		for _, a := range e.Attr {
			if prefix == "" && a.Space == "" && a.Key == "xmlns" {
				return a.Value
			}
			if prefix != "" && a.Space == "xmlns" && a.Key == prefix {
				return a.Value
			}
		}
	}
	return ""
}

// NamespaceURI is the namespace of el, resolved through its ancestors.
func NamespaceURI(el *etree.Element) string { return lookupPrefix(el, el.Space) }

// Declare declares prefix (or the default namespace, for "") as uri on el,
// appending the attribute after el's own, as xmldom's serializer does for an
// element created with createElementNS, unless el's scope already binds it
// to uri.
func Declare(el *etree.Element, prefix, uri string) {
	if lookupPrefix(el, prefix) == uri {
		return
	}
	if prefix == "" {
		el.CreateAttr("xmlns", uri)
	} else {
		el.CreateAttr("xmlns:"+prefix, uri)
	}
}
