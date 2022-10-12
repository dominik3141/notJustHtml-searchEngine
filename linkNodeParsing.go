package main

import (
	"encoding/json"
	"net/url"
	"time"

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
				jsonNode, err := json.Marshal(reduceHtmlNode(c))
				check(err)

				link := Link{
					TimeFound:       time.Now(),
					OrigUrl:         originUrl,
					DestUrl:         linkDst,
					SurroundingNode: jsonNode,
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

// search the child nodes of a html link node for a text node
// an example of that would be a h3 node that serves as the link text
// func getLinkText(linkNode *html.Node) string
