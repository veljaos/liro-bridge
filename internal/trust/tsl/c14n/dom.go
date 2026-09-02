// Package c14n implements Exclusive XML Canonicalization 1.0, without
// comments (http://www.w3.org/2001/10/xml-exc-c14n#), by hand.
//
// This exists because the Go standard library has no XML canonicalisation
// of any kind, and verifying the Trusted List's XML-DSig signature
// (SPEC §11.1, F1 §4.5) is not possible without it. It is deliberately
// small and self-contained rather than a general XML toolkit: this code
// decides what the agent trusts, so it must stay small enough to read
// end to end (F1 §4.5).
//
// internal/trust must not import anything from internal/ (SPEC §4.2 rule
// 3); this subpackage does not either, and depends only on the standard
// library.
package c14n

import (
	"encoding/xml"
	"fmt"
	"io"
)

// NodeKind distinguishes the kinds of node this package's tree can hold.
// Exclusive C14N without comments never emits comments or processing
// instructions, but they are parsed and kept in the tree so that
// skipping a subtree (the enveloped-signature transform) does not
// silently misplace surrounding text.
type NodeKind int

const (
	ElementNode NodeKind = iota
	TextNode
	CommentNode
	ProcInstNode
)

// Attr is one attribute, with its namespace resolved from the document's
// scope at the point it appears (see Node.resolve in parse.go).
type Attr struct {
	Prefix string // "" for an unprefixed attribute
	Local  string
	URI    string // "" for an unprefixed attribute (SPEC: unprefixed attributes are never in a namespace, not even the default one)
	Value  string
}

// Node is one node in the parsed document tree.
type Node struct {
	Kind NodeKind

	// Element fields.
	Prefix   string // "" for an element in the default namespace or with no namespace
	Local    string
	URI      string // resolved namespace URI, "" if none
	Attrs    []Attr
	Children []*Node

	// inScope is every prefix->URI binding in effect at this element,
	// including inherited ones, computed once during parsing. It is what
	// lets an element deep in a subtree correctly resolve a namespace it
	// uses but does not redeclare itself (F1 §4.5's exclusive-rendering
	// rule needs the real URI, wherever it was actually declared).
	inScope map[string]string

	// Text/Comment/ProcInst fields.
	Text   string
	Target string // ProcInst only
}

// xmlNSNamespace is the namespace implicitly bound to the "xml" prefix in
// every XML document, per the XML Namespaces spec. It is never declared
// with an xmlns attribute and never rendered as one.
const xmlNSNamespace = "http://www.w3.org/XML/1998/namespace"

// Parse reads an XML document with r.Token()-equivalent semantics but
// using RawToken, which reports element and attribute names as their
// literal (prefix, local) pair instead of resolving the prefix to a
// namespace URI. Exclusive C14N needs the literal prefix to reproduce it
// in the output, and needs the resolved URI to decide which prefixes are
// "the same namespace" — RawToken plus this package's own scope tracking
// gives both.
//
// It returns the document element (whitespace and any other content
// outside it is dropped, matching F1 §4.5: "whitespace outside the
// document element is dropped").
func Parse(r io.Reader) (*Node, error) {
	dec := xml.NewDecoder(r)

	type frame struct {
		node    *Node
		inScope map[string]string
	}
	var stack []frame
	var root *Node

	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("c14n: parsing XML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			parentScope := map[string]string{}
			if len(stack) > 0 {
				parentScope = stack[len(stack)-1].inScope
			}
			scope := declaredScope(parentScope, t.Attr)

			prefix, local := splitName(t.Name)
			n := &Node{
				Kind:    ElementNode,
				Prefix:  prefix,
				Local:   local,
				URI:     resolveElementURI(scope, prefix),
				inScope: scope,
			}
			for _, a := range t.Attr {
				aPrefix, aLocal := splitName(a.Name)
				if aPrefix == "xmlns" || (aPrefix == "" && aLocal == "xmlns") {
					continue // namespace declarations are not rendered as attributes
				}
				n.Attrs = append(n.Attrs, Attr{
					Prefix: aPrefix,
					Local:  aLocal,
					URI:    resolveAttrURI(scope, aPrefix),
					Value:  a.Value,
				})
			}

			if len(stack) > 0 {
				parent := stack[len(stack)-1].node
				parent.Children = append(parent.Children, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, frame{node: n, inScope: scope})

		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("c14n: unbalanced end element </%s>", t.Name.Local)
			}
			stack = stack[:len(stack)-1]

		case xml.CharData:
			if len(stack) == 0 {
				continue // whitespace outside the document element
			}
			parent := stack[len(stack)-1].node
			parent.Children = append(parent.Children, &Node{Kind: TextNode, Text: string(t)})

		case xml.Comment:
			if len(stack) == 0 {
				continue
			}
			parent := stack[len(stack)-1].node
			parent.Children = append(parent.Children, &Node{Kind: CommentNode, Text: string(t)})

		case xml.ProcInst:
			if len(stack) == 0 {
				continue // e.g. the <?xml version="1.0"?> declaration itself
			}
			parent := stack[len(stack)-1].node
			parent.Children = append(parent.Children, &Node{Kind: ProcInstNode, Target: t.Target, Text: string(t.Inst)})
		}
	}

	if root == nil {
		return nil, fmt.Errorf("c14n: document has no element")
	}
	return root, nil
}

// splitName turns RawToken's (Space, Local) pair — Space holds the raw
// prefix text, empty if none — into (prefix, local).
func splitName(n xml.Name) (prefix, local string) {
	return n.Space, n.Local
}

// declaredScope returns the namespace scope in effect on an element:
// parentScope, overridden by any xmlns/xmlns:* declarations among attrs.
func declaredScope(parentScope map[string]string, attrs []xml.Attr) map[string]string {
	scope := make(map[string]string, len(parentScope)+1)
	for k, v := range parentScope {
		scope[k] = v
	}
	for _, a := range attrs {
		prefix, local := splitName(a.Name)
		switch {
		case prefix == "" && local == "xmlns":
			scope[""] = a.Value
		case prefix == "xmlns":
			scope[local] = a.Value
		}
	}
	return scope
}

func resolveElementURI(scope map[string]string, prefix string) string {
	if prefix == "xml" {
		return xmlNSNamespace
	}
	return scope[prefix]
}

// resolveAttrURI resolves a prefixed attribute's namespace. Per the XML
// Namespaces spec (and F1 §4.5's canonicalisation rules, which build on
// it), an unprefixed attribute is never in any namespace — not even the
// default one — so this is only ever called with a non-empty prefix.
func resolveAttrURI(scope map[string]string, prefix string) string {
	if prefix == "" {
		return ""
	}
	if prefix == "xml" {
		return xmlNSNamespace
	}
	return scope[prefix]
}

// Find returns the first descendant element (or n itself) matching
// (uri, local), depth-first, or nil if none matches.
func Find(n *Node, uri, local string) *Node {
	if n.Kind == ElementNode && n.URI == uri && n.Local == local {
		return n
	}
	for _, c := range n.Children {
		if found := Find(c, uri, local); found != nil {
			return found
		}
	}
	return nil
}

// Text returns the concatenation of all direct text-node children,
// trimmed of nothing — callers that want trimming do it themselves,
// since C14N-adjacent code generally cares about exact content.
func Text(n *Node) string {
	var s string
	for _, c := range n.Children {
		if c.Kind == TextNode {
			s += c.Text
		}
	}
	return s
}
