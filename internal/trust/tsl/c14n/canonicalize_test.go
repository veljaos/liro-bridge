package c14n

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) *Node {
	t.Helper()
	n, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return n
}

// TestExclusiveC14NSimpleReenveloping is Example 1 from the W3C Exclusive
// XML Canonicalization specification (https://www.w3.org/TR/xml-exc-c14n/),
// section "The Need for Exclusive Canonicalization": re-enveloping a
// signed element must not pull in the enclosing document's unused
// namespace declaration.
//
// Original:
//
//	<n1:elem1 xmlns:n1="http://b.example">
//	    content
//	</n1:elem1>
//
// Enveloped (n1:elem1 re-parented under a new element declaring an
// unrelated, unused prefix n0):
//
//	<n0:pdu xmlns:n0="http://a.example">
//	   <n1:elem1 xmlns:n1="http://b.example">
//	       content
//	   </n1:elem1>
//	</n0:pdu>
//
// Exclusive canonicalization of the extracted n1:elem1 subtree must be
// byte-identical to the original standalone document: n0 is never used
// inside the subtree, so it must not appear, regardless of being in
// scope at the point the subtree was extracted from.
func TestExclusiveC14NSimpleReenveloping(t *testing.T) {
	enveloped := `<n0:pdu xmlns:n0="http://a.example">
   <n1:elem1 xmlns:n1="http://b.example">
       content
   </n1:elem1>
</n0:pdu>`

	root := mustParse(t, enveloped)
	elem1 := Find(root, "http://b.example", "elem1")
	if elem1 == nil {
		t.Fatal("n1:elem1 not found")
	}

	got := string(Canonicalize(elem1, nil))
	want := `<n1:elem1 xmlns:n1="http://b.example">
       content
   </n1:elem1>`
	if got != want {
		t.Fatalf("Canonicalize() =\n%q\nwant\n%q", got, want)
	}
}

// TestExclusiveC14NComplexReenveloping is Example 2 from the same
// specification section: a subtree containing its own nested namespace
// declaration (n3, redeclared locally on n3:stuff) is moved into a
// completely different context that redefines n1 to a different URI,
// binds xml:lang/xml:space on the new ancestor, and declares an unused
// n2. None of the new context's declarations or attributes may leak into
// the canonical form of the moved subtree — that is the entire point of
// "exclusive" canonicalization (unlike plain Canonical XML, which does
// inherit ancestor context, including xml:lang/xml:space).
func TestExclusiveC14NComplexReenveloping(t *testing.T) {
	newContext := `<n2:pdu xmlns:n1="http://example.com"
        xmlns:n2="http://foo.example"
        xml:lang="fr"
        xml:space="retain">
   <n1:elem2 xmlns:n1="http://example.net"
             xml:lang="en">
       <n3:stuff xmlns:n3="ftp://example.org"></n3:stuff>
   </n1:elem2>
</n2:pdu>`

	root := mustParse(t, newContext)
	elem2 := Find(root, "http://example.net", "elem2")
	if elem2 == nil {
		t.Fatal("n1:elem2 (http://example.net) not found")
	}

	got := string(Canonicalize(elem2, nil))
	// Whitespace between attributes inside a start tag is markup, not
	// content: C14N always normalises it to a single space, regardless
	// of how the source XML happened to format it — unlike whitespace
	// between elements, which is text content and is preserved exactly.
	want := `<n1:elem2 xmlns:n1="http://example.net" xml:lang="en">
       <n3:stuff xmlns:n3="ftp://example.org"></n3:stuff>
   </n1:elem2>`
	if got != want {
		t.Fatalf("Canonicalize() =\n%q\nwant\n%q", got, want)
	}
}

// TestEnvelopedSignatureExclusionRemovesSubtree exercises the "remove
// the Signature element itself" half of the enveloped-signature
// transform (F1 §4.5), independent of the actual TSL document.
func TestEnvelopedSignatureExclusionRemovesSubtree(t *testing.T) {
	doc := `<Root xmlns="urn:example"><Data>x</Data><ds:Signature xmlns:ds="urn:dsig"><ds:SignedInfo/></ds:Signature></Root>`
	root := mustParse(t, doc)

	exclude := func(n *Node) bool {
		return n.URI == "urn:dsig" && n.Local == "Signature"
	}
	got := string(Canonicalize(root, exclude))
	want := `<Root xmlns="urn:example"><Data>x</Data></Root>`
	if got != want {
		t.Fatalf("Canonicalize() with exclusion = %q, want %q", got, want)
	}
}

// TestAttributeAndTextEscaping proves the F1 §4.5 escaping table.
//
// A literal carriage return in source XML is normalised to '\n' by any
// conforming XML processor (including encoding/xml) before an
// application ever sees it — only a character reference (&#13;) survives
// as a real '\r' through parsing. The vectors below use &#13; for that
// reason: it is the only way to actually exercise the '\r' branch of
// escapeAttrValue/escapeText against real parser output rather than
// against an untested assumption about what the parser hands back.
func TestAttributeAndTextEscaping(t *testing.T) {
	doc := "<r a=\"&amp;&lt;&quot;\t\n&#13; end\"><![CDATA[a & b < c > d]]>&#13;</r>"
	root := mustParse(t, doc)
	got := string(Canonicalize(root, nil))
	want := "<r a=\"&amp;&lt;&quot;&#x9;&#xA;&#xD; end\">a &amp; b &lt; c &gt; d&#xD;</r>"
	if got != want {
		t.Fatalf("Canonicalize() = %q, want %q", got, want)
	}
}

func TestEmptyElementIsNeverSelfClosing(t *testing.T) {
	root := mustParse(t, `<r><empty/></r>`)
	got := string(Canonicalize(root, nil))
	want := `<r><empty></empty></r>`
	if got != want {
		t.Fatalf("Canonicalize() = %q, want %q", got, want)
	}
}

func TestCommentsAreDroppedWithoutCommentsVariant(t *testing.T) {
	root := mustParse(t, `<r><!-- a comment -->text</r>`)
	got := string(Canonicalize(root, nil))
	want := `<r>text</r>`
	if got != want {
		t.Fatalf("Canonicalize() = %q, want %q", got, want)
	}
}

// TestAttributesSortedByNamespaceThenLocalName covers the ordering rule
// (namespace URI primary key, local name secondary, unprefixed
// attributes — no namespace — sorting first).
func TestAttributesSortedByNamespaceThenLocalName(t *testing.T) {
	doc := `<r xmlns:b="urn:b" xmlns:a="urn:a" b:z="1" a:y="2" plain="3" a:x="4"></r>`
	root := mustParse(t, doc)
	got := string(Canonicalize(root, nil))
	want := `<r xmlns:a="urn:a" xmlns:b="urn:b" plain="3" a:x="4" a:y="2" b:z="1"></r>`
	if got != want {
		t.Fatalf("Canonicalize() = %q, want %q", got, want)
	}
}

// TestUnusedInheritedNamespaceIsNotRendered directly targets the
// "visibly utilised" rule central to exclusivity (F1 §4.5): a namespace
// merely in lexical scope, but never referenced by the subtree's element
// names or attribute names, must never appear in the output.
func TestUnusedInheritedNamespaceIsNotRendered(t *testing.T) {
	doc := `<a:root xmlns:a="urn:a" xmlns:unused="urn:unused"><a:child>text</a:child></a:root>`
	root := mustParse(t, doc)
	got := string(Canonicalize(root, nil))
	if strings.Contains(got, "unused") {
		t.Fatalf("Canonicalize() = %q, must not mention the unused namespace", got)
	}
	want := `<a:root xmlns:a="urn:a"><a:child>text</a:child></a:root>`
	if got != want {
		t.Fatalf("Canonicalize() = %q, want %q", got, want)
	}
}

// TestPrefixReboundDeeperInTreeRendersAgain: exclusive C14N tracks
// prefix+URI pairs, not just prefixes — the same prefix bound to a
// different URI further down must be re-declared.
func TestPrefixReboundDeeperInTreeRendersAgain(t *testing.T) {
	doc := `<a:r xmlns:a="urn:one"><a:child xmlns:a="urn:two">x</a:child></a:r>`
	root := mustParse(t, doc)
	got := string(Canonicalize(root, nil))
	want := `<a:r xmlns:a="urn:one"><a:child xmlns:a="urn:two">x</a:child></a:r>`
	if got != want {
		t.Fatalf("Canonicalize() = %q, want %q", got, want)
	}
}
