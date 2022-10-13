package main

import (
	"net/url"
	"time"

	"github.com/icza/gox/stringsx"
	"golang.org/x/net/html"
)

type ReducedHtmlNode struct {
	Type html.NodeType
	Data string
	// Namespace string
	Attr   []html.Attribute
	Childs []*ReducedHtmlNode
}

func reduce(node *html.Node) *ReducedHtmlNode {
	return &ReducedHtmlNode{
		Type: node.Type,
		Attr: node.Attr,
		Data: node.Data,
	}
}

func getChilds(node *html.Node) []*ReducedHtmlNode {
	childs := make([]*ReducedHtmlNode, 0)

	for c := node.FirstChild; c != nil; c = c.NextSibling {
		rChild := reduce(c)
		rChild.Childs = getChilds(c)
		childs = append(childs, rChild)
	}

	return childs
}

func reduceHtmlNode(node *html.Node) *ReducedHtmlNode {
	const depth = 0

	for i := 1; i <= depth && node.Parent != nil; i++ {
		node = node.Parent
	}

	ret := reduce(node)
	ret.Childs = getChilds(node)
	return ret
}

// gets all links starting at a given html node
// all found links are send to a go channel
func getAllLinks(originUrl *url.URL, node *html.Node, links chan<- *Link) {
	extractLink := func(c *html.Node) {
		for _, a := range c.Attr {
			if a.Key == "href" || a.Key == "src" {
				linkDst, err := url.Parse(a.Val)
				if err != nil {
					// log.Println("Malformed url:", a.Val)
					break
				}

				if linkDst.Hostname() == "" {
					linkDst.Host = originUrl.Host
					linkDst.Scheme = originUrl.Scheme
					// log.Printf("Corrected url from %v to %v", a.Val, linkDst.String())
				}

				// jsonNode, err := json.MarshalIndent(reduceHtmlNode(c), "", "\t")
				reducedNode := reduceHtmlNode(c)
				// jsonNode, err := json.Marshal(reducedNode)
				// check(err)

				link := Link{
					TimeFound: time.Now(),
					OrigUrl:   originUrl,
					DestUrl:   linkDst,
					// SurroundingNode: jsonNode,
					Keywords: extractKeywords(reducedNode, 10),
				}

				links <- &link
			}
		}
	}

	for c := node; c != nil; c = c.NextSibling {
		extractLink(c)
		if c.FirstChild != nil {
			getAllLinks(originUrl, c.FirstChild, links)
		}
	}
}

func extractKeywords(rNode *ReducedHtmlNode, multiplier int) []HtmlText {
	keywords := make([]HtmlText, 0)

	switch rNode.Data {
	case "h1":
		multiplier = 1
	case "h2":
		multiplier = 2
	case "h3":
		multiplier = 3
	case "h4":
		multiplier = 4
	case "h5":
		multiplier = 5
	case "h6":
		multiplier = 6
	case "h7":
		multiplier = 7
	}

	if rNode.Type == html.TextNode && rNode.Data != "" {
		word := HtmlText{
			Text:       stringsx.Clean(rNode.Data),
			Visibility: multiplier,
		}
		keywords = append(keywords, word)
	}

	for i := range rNode.Childs {
		newKeywords := extractKeywords(rNode.Childs[i], multiplier)
		keywords = append(keywords, newKeywords...)
	}

	return keywords
}

// search the child nodes of a html link node for a text node
// an example of that would be a h3 node that serves as the link text
// func getLinkText(linkNode *html.Node) string
