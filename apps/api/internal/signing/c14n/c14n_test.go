package c14n

import (
	"testing"

	"github.com/beevik/etree"
)

func parse(t *testing.T, s string) *etree.Document {
	doc := etree.NewDocument()
	if err := doc.ReadFromString(s); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCanonicalize(t *testing.T) {
	const in = `<r xmlns="urn:d" xmlns:a="urn:a" xmlns:u="urn:unused" xml:lang="es"><a:x b="2" a:c="1" a="&lt;&quot;&#9;"><y>t &amp; &gt;<!--c--></y></a:x></r>`
	doc := parse(t, in)
	x := doc.Root().ChildElements()[0]
	for name, c := range map[string]struct {
		o    Options
		want string
	}{
		"inclusive": {Options{},
			`<a:x xmlns="urn:d" xmlns:a="urn:a" xmlns:u="urn:unused" a="&lt;&quot;&#x9;" b="2" xml:lang="es" a:c="1"><y>t &amp; &gt;</y></a:x>`},
		"inclusive with comments": {Options{Comments: true},
			`<a:x xmlns="urn:d" xmlns:a="urn:a" xmlns:u="urn:unused" a="&lt;&quot;&#x9;" b="2" xml:lang="es" a:c="1"><y>t &amp; &gt;<!--c--></y></a:x>`},
		"exclusive": {Options{Exclusive: true},
			`<a:x xmlns:a="urn:a" a="&lt;&quot;&#x9;" b="2" a:c="1"><y xmlns="urn:d">t &amp; &gt;</y></a:x>`},
		"exclusive with a prefix list": {Options{Exclusive: true, InclusivePrefixes: []string{"u", "#default"}},
			`<a:x xmlns="urn:d" xmlns:a="urn:a" xmlns:u="urn:unused" a="&lt;&quot;&#x9;" b="2" a:c="1"><y>t &amp; &gt;</y></a:x>`},
	} {
		if got := string(Canonicalize(x, c.o)); got != c.want {
			t.Errorf("%s\n got %s\nwant %s", name, got, c.want)
		}
	}
}
