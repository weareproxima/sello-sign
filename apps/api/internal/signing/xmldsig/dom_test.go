package xmldsig

import (
	"errors"
	"testing"
)

// Expected outputs are what new XMLSerializer().serializeToString(new
// DOMParser().parseFromString(input, 'text/xml')) gives with
// @xmldom/xmldom 0.9 (with the carriage return fix noted).
func TestParseSerialize(t *testing.T) {
	for in, want := range map[string]string{
		"\xef\xbb\xbf<?xml version=\"1.0\" ?><a/>":             `<?xml version="1.0" ?><a/>`,
		`<a>AT&T &nbsp; &#xZZ; &amp;amp; &#x41;&#66;</a>`:      `<a>AT&amp;T &amp;nbsp; &amp;#xZZ; &amp;amp; AB</a>`,
		"<a b=\"x\ty\nz\" c='&#9;&#10;&#13;' d='\"' e=\">\"/>": `<a b="x y z" c="&#9;&#10;&#13;" d="&quot;" e="&gt;"/>`,
		"<a>l1\r\nl2\rl3\u2028l4\u0085l5</a>":                  "<a>l1\nl2\nl3\nl4\nl5</a>",
		"<a>&#13;</a>":                                         "<a>&#13;</a>",
		"<a><![CDATA[<x> & ]]]]><b></b></a>":                   "<a><![CDATA[<x> & ]]]]><b/></a>",
		"<a><!-- c --><?pi  data ?></a>":                       "<a><!-- c --><?pi data ?></a>",
		"<p:a xmlns:p=\"u\" p:b=\"1\" xml:lang=\"es\"></p:a>":  `<p:a xmlns:p="u" p:b="1" xml:lang="es"/>`,
		"\n<a>\n  <b> x </b>\n</a>\n":                          "\n<a>\n  <b> x </b>\n</a>\n",
		"<a>&#55357;&#128512;</a>":                             "<a>\ufffd😀</a>",
	} {
		doc, err := Parse([]byte(in))
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got := string(Serialize(doc)); got != want {
			t.Errorf("%q\n got %q\nwant %q", in, got, want)
		}
	}
	for _, bad := range []string{
		"", " <?xml version=\"1.0\"?><a/>", "<!DOCTYPE a><a/>", "<a></b>", "<a>", "<a b='1' b='2'/>",
		"<a b='<'/>", "<p:a/>", "<a p:b='1'/>", "<a/><b/>", "<a xmlns:p=''/>", "\xff<a/>", "text", "<a b=1/>",
	} {
		if _, err := Parse([]byte(bad)); !errors.Is(err, ErrMalformed) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestStripWhitespace(t *testing.T) {
	doc, err := Parse([]byte("<?xml version=\"1.0\"?>\n<a>\n\t<b>\u00a0\ufeff</b> <c> x </c><![CDATA[ ]]>\u3000</a>\n"))
	if err != nil {
		t.Fatal(err)
	}
	StripWhitespace(&doc.Element)
	if got, want := string(Serialize(doc)), `<?xml version="1.0"?><a><b/><c> x </c><![CDATA[ ]]></a>`; got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}

	// What the XAdES signers do with a body: the declared encoding is
	// ignored, line endings and attribute whitespace are normalized, and
	// what is left of the text is as it was.
	doc, err = Parse([]byte("\uFEFF<?xml version='1.0' encoding=\"ISO-8859-1\"?>\r\n<a x=\"l1\nl2\" y=\"c&#10;r\">\r\n  <b>t &amp; &lt; &gt; \"q\"</b><c></c><![CDATA[ <raw> ]]><!--k--><?pi?> </a>\n"))
	if err != nil {
		t.Fatal(err)
	}
	StripWhitespace(&doc.Element)
	if got, want := string(Serialize(doc)), "<?xml version='1.0' encoding=\"ISO-8859-1\"?><a x=\"l1 l2\" y=\"c&#10;r\"><b>t &amp; &lt; &gt; \"q\"</b><c/><![CDATA[ <raw> ]]><!--k--><?pi ?></a>"; got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestSerializeAround(t *testing.T) {
	doc, err := Parse([]byte("<a><b>AT&T&#13;</b><c/></a>"))
	if err != nil {
		t.Fatal(err)
	}
	mark := doc.Root().ChildElements()[1]
	before, after := SerializeAround(doc, mark)
	if string(before) != "<a><b>AT&amp;T&#13;</b>" || string(after) != "</a>" {
		t.Errorf("got %q and %q", before, after)
	}
}

func TestLookups(t *testing.T) {
	doc, err := Parse([]byte(`<r xmlns:ds="` + NS + `" Id="root"><ds:x><y> one </y></ds:x><x/><ds:x/></r>`))
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Root()
	if ElementByLocalName(root, "r") != root || ElementByLocalName(root, "y") == nil || ElementByLocalName(nil, "y") != nil {
		t.Error("ElementByLocalName")
	}
	if got := TrimmedText(ElementByLocalName(root, "y")); got != "one" || TrimmedText(nil) != "" {
		t.Errorf("TrimmedText = %q", got)
	}
	if len(ChildrenNS(root, NS, "x")) != 2 || len(ChildrenNS(root, "", "x")) != 1 {
		t.Error("ChildrenNS")
	}
	if FirstDescendant(root, NS, "x") != root.ChildElements()[0] || FirstDescendant(root, NS, "y") != nil || FirstDescendant(nil, NS, "x") != nil {
		t.Error("FirstDescendant")
	}
	if Child(root, "x") != root.ChildElements()[0] || Child(nil, "x") != nil {
		t.Error("Child")
	}
}

func TestDeclare(t *testing.T) {
	doc, _ := Parse([]byte(`<a xmlns:ds="` + NS + `" xmlns:x="other"><b/></a>`))
	b := doc.Root().ChildElements()[0]
	Declare(b, "ds", NS)
	Declare(b, "x", "mine")
	Declare(b, "", NS)
	if got, want := string(Serialize(doc)), `<a xmlns:ds="`+NS+`" xmlns:x="other"><b xmlns:x="mine" xmlns="`+NS+`"/></a>`; got != want {
		t.Errorf("got %s", got)
	}
}
