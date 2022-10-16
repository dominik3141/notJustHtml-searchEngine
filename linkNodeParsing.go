package main

import (
	"net/url"
	"time"

	"github.com/icza/gox/stringsx"
	"golang.org/x/net/html"
)

// gets all links starting at a given html node
// all found links are send to a go channel
func getAllLinks(originUrl *url.URL, node *html.Node, links chan<- *Link) {
	extractLink := func(c *html.Node) {
		for _, a := range c.Attr {
			if a.Key == "href" || a.Key == "src" {
				linkDst, err := url.Parse(a.Val)
				if err != nil {
					logErrorToDb(err, ErrorParsingUrl, a.Val)
					break
				}

				// correct urls that do not contain a hostname
				if linkDst.Hostname() == "" {
					linkDst.Host = originUrl.Host
					linkDst.Scheme = originUrl.Scheme
				}

				reducedNode := reduceHtmlNode(c)
				keywords := extractKeywords(reducedNode, 1)

				link := Link{
					TimeFound: time.Now(),
					OrigUrl:   originUrl,
					DestUrl:   linkDst,
					Keywords:  &keywords,
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

type ReducedHtmlNode struct {
	Type   html.NodeType
	Data   string
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

// search the child nodes of a html link node for a text node
// an example of that would be a h3 node that serves as the link text
func extractKeywords(rNode *ReducedHtmlNode, multiplier int) []HtmlText {
	keywords := make([]HtmlText, 0)

	switch rNode.Data {
	case "h1":
		multiplier = 10
	case "h2":
		multiplier = 9
	case "h3":
		multiplier = 8
	case "h4":
		multiplier = 7
	case "h5":
		multiplier = 6
	case "h6":
		multiplier = 5
	case "h7":
		multiplier = 4
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
