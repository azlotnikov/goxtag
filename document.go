package goxtag

import (
	"bytes"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

const (
	maxUint = ^uint(0)
	maxInt  = int(maxUint >> 1)
)

type Document struct {
	Nodes []*html.Node
}

func newDocumentWithNode(node *html.Node) *Document {
	return &Document{
		Nodes: []*html.Node{node},
	}
}

func newDocumentWithNodes(nodes []*html.Node) *Document {
	return &Document{
		Nodes: nodes,
	}
}

func (doc *Document) Length() int {
	return len(doc.Nodes)
}

func (doc *Document) Html() (ret string, e error) {
	// Since there is no .innerHtml, the HTML content must be re-created from
	// the nodes using html.Render.
	var buf bytes.Buffer

	if len(doc.Nodes) > 0 {
		for c := doc.Nodes[0].FirstChild; c != nil; c = c.NextSibling {
			e = html.Render(&buf, c)
			if e != nil {
				return
			}
		}
		ret = buf.String()
	}

	return
}

func (doc *Document) Text() string {
	var buf bytes.Buffer

	// Slightly optimized vs calling Each: no single selection object created
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			// Keep newlines and spaces, like jQuery
			buf.WriteString(n.Data)
		}
		if n.FirstChild != nil {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				f(c)
			}
		}
	}

	for _, n := range doc.Nodes {
		f(n)
	}

	return buf.String()
}

func (doc *Document) Attr(attrName string) (val string, exists bool) {
	if len(doc.Nodes) == 0 {
		return
	}
	return getAttributeValue(attrName, doc.Nodes[0])
}

func (doc *Document) Find(selector string) *Document {
	return newDocumentWithNodes(htmlquery.Find(doc.Nodes[0], selector))
}

func (doc *Document) Eq(index int) *Document {
	if index < 0 {
		index += len(doc.Nodes)
	}

	if index >= len(doc.Nodes) || index < 0 {
		return &Document{}
	}

	return doc.Slice(index, index+1)
}

func (doc *Document) Slice(start, end int) *Document {
	if start < 0 {
		start += len(doc.Nodes)
	}
	if end == maxInt {
		end = len(doc.Nodes)
	} else if end < 0 {
		end += len(doc.Nodes)
	}
	return newDocumentWithNodes(doc.Nodes[start:end])
}

func getAttributeValue(attrName string, n *html.Node) (val string, exists bool) {
	if a := getAttributePtr(attrName, n); a != nil {
		val = a.Val
		exists = true
	}
	return
}

func getAttributePtr(attrName string, n *html.Node) *html.Attribute {
	if n == nil {
		return nil
	}

	for i, a := range n.Attr {
		if a.Key == attrName {
			return &n.Attr[i]
		}
	}
	return nil
}
