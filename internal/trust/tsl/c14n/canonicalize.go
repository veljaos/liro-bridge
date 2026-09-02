package c14n

import (
	"bytes"
	"fmt"
	"sort"
)

// Canonicalize serializes n and its descendants using Exclusive XML
// Canonicalization 1.0, without comments
// (http://www.w3.org/2001/10/xml-exc-c14n#).
//
// exclude, if non-nil, is called for every element; when it returns
// true, that element and its entire subtree are omitted from the
// output. This is how the enveloped-signature transform (SPEC/F1 §4.5:
// "remove the Signature element itself, then canonicalise") is
// implemented: Canonicalize(root, isTheSignatureElement).
//
// The namespace-rendering context always starts empty at n, regardless
// of n's real position in the original document — this is the defining
// difference from plain (non-exclusive) Canonical XML, which would pull
// in every namespace merely in scope from ancestors. Exclusive C14N
// exists precisely so that re-enveloping a signed subtree elsewhere does
// not change what it canonicalises to (F1 §4.5).
func Canonicalize(n *Node, exclude func(*Node) bool) []byte {
	var buf bytes.Buffer
	serializeElement(&buf, n, map[string]string{}, exclude)
	return buf.Bytes()
}

func qname(prefix, local string) string {
	if prefix == "" {
		return local
	}
	return prefix + ":" + local
}

type nsDecl struct{ prefix, uri string }

// serializeElement writes n's canonical form to buf. rendered is the
// namespace context already emitted by an ancestor in *this
// serialization* (not the original document) — exclusive C14N's
// minimisation is defined against that, not against the document's own
// declaration sites.
func serializeElement(buf *bytes.Buffer, n *Node, rendered map[string]string, exclude func(*Node) bool) {
	newRendered := make(map[string]string, len(rendered)+2)
	for k, v := range rendered {
		newRendered[k] = v
	}

	var toRender []nsDecl
	// A prefix absent from newRendered reads as "" (Go's zero value for
	// string), which is deliberately the same value used for "no
	// namespace" / "no default namespace" — so an element or attribute
	// that needs no namespace never triggers a render just because the
	// map has no entry yet.
	needRender := func(prefix, uri string) {
		if newRendered[prefix] != uri {
			toRender = append(toRender, nsDecl{prefix, uri})
			newRendered[prefix] = uri
		}
	}

	// The element's own name: a prefixed element always needs its
	// namespace visible; an unprefixed element needs the default
	// namespace rendered only if it actually has one, or to cancel one
	// inherited from an ancestor's rendered context with xmlns="".
	if n.Prefix != "" {
		needRender(n.Prefix, n.URI)
	} else {
		needRender("", n.URI)
	}

	// Attributes: only prefixed ones carry a namespace. An unprefixed
	// attribute is never in any namespace — not even the default one —
	// so it can never trigger a namespace declaration (F1 §4.5's
	// escaping/rendering rules build on this XML Namespaces rule). The
	// implicit "xml" prefix is never declared, only ever used.
	for _, a := range n.Attrs {
		if a.Prefix == "" || a.Prefix == "xml" {
			continue
		}
		needRender(a.Prefix, a.URI)
	}

	sort.Slice(toRender, func(i, j int) bool {
		if (toRender[i].prefix == "") != (toRender[j].prefix == "") {
			return toRender[i].prefix == "" // default namespace sorts first
		}
		return toRender[i].prefix < toRender[j].prefix
	})

	attrs := make([]Attr, len(n.Attrs))
	copy(attrs, n.Attrs)
	sort.SliceStable(attrs, func(i, j int) bool {
		if attrs[i].URI != attrs[j].URI {
			return attrs[i].URI < attrs[j].URI // no-namespace ("") sorts first
		}
		return attrs[i].Local < attrs[j].Local
	})

	buf.WriteByte('<')
	buf.WriteString(qname(n.Prefix, n.Local))
	for _, d := range toRender {
		buf.WriteByte(' ')
		if d.prefix == "" {
			buf.WriteString(`xmlns="`)
		} else {
			buf.WriteString("xmlns:")
			buf.WriteString(d.prefix)
			buf.WriteString(`="`)
		}
		buf.WriteString(escapeAttrValue(d.uri))
		buf.WriteByte('"')
	}
	for _, a := range attrs {
		buf.WriteByte(' ')
		buf.WriteString(qname(a.Prefix, a.Local))
		buf.WriteString(`="`)
		buf.WriteString(escapeAttrValue(a.Value))
		buf.WriteByte('"')
	}
	buf.WriteByte('>')

	for _, c := range n.Children {
		serializeNode(buf, c, newRendered, exclude)
	}

	// Empty elements are always written as start tag plus end tag, never
	// self-closing (F1 §4.5) — the loop above simply writes nothing
	// between the two when there are no children.
	buf.WriteString("</")
	buf.WriteString(qname(n.Prefix, n.Local))
	buf.WriteByte('>')
}

func serializeNode(buf *bytes.Buffer, n *Node, rendered map[string]string, exclude func(*Node) bool) {
	switch n.Kind {
	case ElementNode:
		if exclude != nil && exclude(n) {
			return
		}
		serializeElement(buf, n, rendered, exclude)
	case TextNode:
		buf.WriteString(escapeText(n.Text))
	case CommentNode:
		// "without comments": comments never appear in the output.
	case ProcInstNode:
		buf.WriteString("<?")
		buf.WriteString(n.Target)
		if n.Text != "" {
			buf.WriteByte(' ')
			buf.WriteString(n.Text)
		}
		buf.WriteString("?>")
	default:
		panic(fmt.Sprintf("c14n: unknown node kind %d", n.Kind))
	}
}

// escapeAttrValue applies the F1 §4.5 attribute-value escaping rules.
func escapeAttrValue(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '"':
			b.WriteString("&quot;")
		case '\t':
			b.WriteString("&#x9;")
		case '\n':
			b.WriteString("&#xA;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeText applies the F1 §4.5 text-node escaping rules.
func escapeText(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\r':
			b.WriteString("&#xD;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
