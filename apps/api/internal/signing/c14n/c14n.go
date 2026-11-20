// Package c14n canonicalizes an element of an etree document with
// Canonical XML 1.0 (https://www.w3.org/TR/xml-c14n), inclusive or
// exclusive (https://www.w3.org/TR/xml-exc-c14n), with or without comments,
// as it sits in its document: the namespaces its ancestors declare are in
// scope.
//
// It started as the spike of 2026-11-17, which verified with it the 13
// sample signatures of the TypeScript signers (docs/rebuild.md). Node sets
// other than a whole subtree are not supported: a caller drops a subtree,
// such as an enveloped Signature, from the document before calling, and
// puts it back after.
package c14n

import (
	"bytes"
	"sort"
	"strings"

	"github.com/beevik/etree"
)

const xmlNS = "http://www.w3.org/XML/1998/namespace"

// Options picks the canonicalization.
type Options struct {
	// Exclusive renders only the namespaces an element visibly uses.
	Exclusive bool
	// Comments keeps the comments (the "#WithComments" variants).
	Comments bool
	// InclusivePrefixes is the InclusiveNamespaces PrefixList of exclusive
	// canonicalization: prefixes rendered as inclusive canonicalization
	// would, "#default" standing for the default namespace.
	InclusivePrefixes []string
}

// Algorithm URIs, as Signature elements name them.
const (
	Inclusive             = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315"
	InclusiveWithComments = "http://www.w3.org/TR/2001/REC-xml-c14n-20010315#WithComments"
	Exclusive             = "http://www.w3.org/2001/10/xml-exc-c14n#"
	ExclusiveWithComments = "http://www.w3.org/2001/10/xml-exc-c14n#WithComments"
)

// OptionsFor maps an algorithm URI to its options.
func OptionsFor(uri string) (Options, bool) {
	switch uri {
	case Inclusive:
		return Options{}, true
	case InclusiveWithComments:
		return Options{Comments: true}, true
	case Exclusive:
		return Options{Exclusive: true}, true
	case ExclusiveWithComments:
		return Options{Exclusive: true, Comments: true}, true
	}
	return Options{}, false
}

// Canonicalize writes apex and its subtree in canonical form.
func Canonicalize(apex *etree.Element, o Options) []byte {
	var b bytes.Buffer
	var inherited []etree.Attr
	if !o.Exclusive {
		// Canonical XML 1.0 gives the apex the xml:* attributes of its
		// ancestors, the nearest one's first.
		seen := map[string]bool{}
		for _, a := range apex.Attr {
			if a.Space == "xml" {
				seen[a.Key] = true
			}
		}
		for p := apex.Parent(); p != nil; p = p.Parent() {
			for _, a := range p.Attr {
				if a.Space == "xml" && !seen[a.Key] {
					seen[a.Key] = true
					inherited = append(inherited, a)
				}
			}
		}
	}
	writeElement(&b, apex, scopeOf(apex.Parent()), map[string]string{}, o, inherited)
	return b.Bytes()
}

func declarations(el *etree.Element) map[string]string {
	out := map[string]string{}
	for _, a := range el.Attr {
		switch {
		case a.Space == "xmlns":
			out[a.Key] = a.Value
		case a.Space == "" && a.Key == "xmlns":
			out[""] = a.Value
		}
	}
	return out
}

func scopeOf(el *etree.Element) map[string]string {
	if el == nil {
		return map[string]string{}
	}
	scope := scopeOf(el.Parent())
	for p, u := range declarations(el) {
		scope[p] = u
	}
	return scope
}

func writeElement(b *bytes.Buffer, el *etree.Element, parentScope, rendered map[string]string, o Options, extra []etree.Attr) {
	scope := map[string]string{}
	for p, u := range parentScope {
		scope[p] = u
	}
	for p, u := range declarations(el) {
		scope[p] = u
	}
	emit := map[string]string{}
	consider := func(p string) {
		if p == "xml" {
			return
		}
		if u := scope[p]; rendered[p] != u {
			emit[p] = u
		}
	}
	if o.Exclusive {
		for _, p := range o.InclusivePrefixes {
			if p == "#default" {
				p = ""
			}
			if _, ok := scope[p]; ok || p == "" {
				consider(p)
			}
		}
		consider(el.Space)
		for _, a := range el.Attr {
			if a.Space != "" && a.Space != "xmlns" {
				consider(a.Space)
			}
		}
	} else {
		for p := range scope {
			consider(p)
		}
		if _, ok := scope[""]; !ok {
			consider("")
		}
	}
	now := map[string]string{}
	for p, u := range rendered {
		now[p] = u
	}
	prefixes := make([]string, 0, len(emit))
	for p, u := range emit {
		now[p] = u
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	name := el.Tag
	if el.Space != "" {
		name = el.Space + ":" + el.Tag
	}
	b.WriteString("<" + name)
	for _, p := range prefixes {
		if p == "" {
			b.WriteString(` xmlns="` + escapeAttr(emit[p]) + `"`)
		} else {
			b.WriteString(" xmlns:" + p + `="` + escapeAttr(emit[p]) + `"`)
		}
	}
	type attr struct{ ns, local, name, value string }
	var attrs []attr
	for _, a := range append(append([]etree.Attr{}, el.Attr...), extra...) {
		if a.Space == "xmlns" || (a.Space == "" && a.Key == "xmlns") {
			continue
		}
		x := attr{local: a.Key, name: a.Key, value: a.Value}
		if a.Space != "" {
			x.name = a.Space + ":" + a.Key
			x.ns = scope[a.Space]
			if a.Space == "xml" {
				x.ns = xmlNS
			}
		}
		attrs = append(attrs, x)
	}
	sort.Slice(attrs, func(i, j int) bool {
		if attrs[i].ns != attrs[j].ns {
			return attrs[i].ns < attrs[j].ns
		}
		return attrs[i].local < attrs[j].local
	})
	for _, a := range attrs {
		b.WriteString(" " + a.name + `="` + escapeAttr(a.value) + `"`)
	}
	b.WriteString(">")
	for _, c := range el.Child {
		switch t := c.(type) {
		case *etree.Element:
			writeElement(b, t, scope, now, o, nil)
		case *etree.CharData:
			b.WriteString(escapeText(t.Data))
		case *etree.Comment:
			if o.Comments {
				b.WriteString("<!--" + t.Data + "-->")
			}
		case *etree.ProcInst:
			b.WriteString("<?" + t.Target)
			if t.Inst != "" {
				b.WriteString(" " + t.Inst)
			}
			b.WriteString("?>")
		}
	}
	b.WriteString("</" + name + ">")
}

var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\r", "&#xD;")
var attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;", "\t", "&#x9;", "\n", "&#xA;", "\r", "&#xD;")

func escapeText(s string) string { return textEscaper.Replace(s) }
func escapeAttr(s string) string { return attrEscaper.Replace(s) }
