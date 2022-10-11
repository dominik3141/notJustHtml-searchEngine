package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/go-redis/redis"
	"github.com/uptrace/bun"
	"golang.org/x/net/html"
)

func handleNewPage(linksChan chan Link, db *bun.DB, rdb *redis.Client) {
	var dbErr GetErr

	for {
		link := <-linksChan

		// add link to database
		_, err := db.NewInsert().Model(&link).Exec(context.Background())
		handleSqliteErr(err)

		// query redis
		done, err := rdb.SIsMember("visitedLinks", link.DestUrl).Result()
		check(err)
		if done {
			continue
		}

		url, err := url.Parse(link.DestUrl)
		if err != nil {
			log.Printf("URL: %v. Url parsing error. err=%v", link.DestUrl, err)
		}

		resp, err := http.Get(url.String())
		if err != nil {
			log.Printf("URL: %v. html get error. err=%v", url.String(), err)
			dbErr = GetErr{Url: url.String(), HttpStatusCode: resp.StatusCode, Time: time.Now()}
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleSqliteErr(err)
		}

		rawHtml := resp.Body
		rootNode, err := html.Parse(rawHtml)
		if err != nil {
			log.Printf("URL: %v. html parsing error. err=%v", url, err)
			dbErr = GetErr{Url: url.String(), ParsingError: true, Time: time.Now()}
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleSqliteErr(err)
		}

		getAllLinks(url.Hostname(), rootNode, linksChan)

		// set link to done in redis
		err = rdb.SAdd("visitedLinks", link.DestUrl).Err()
		check(err)
	}
}

// gets all links starting at a given html node
// all found links are send to a go channel
func getAllLinks(originUrl string, node *html.Node, links chan<- Link) {
	extractLink := func(c *html.Node) {
		if c.Type == html.ElementNode && c.Data == "a" {
			for _, a := range c.Attr {
				if a.Key == "href" {
					links <- Link{
						OrigUrl: originUrl,
						DestUrl: originUrl + a.Val,
						// LinkText: c.,
						TimeFound: time.Now(),
					}
					break
				}
			}
		}
	}

	for c := node.FirstChild; c != nil; c = c.NextSibling {
		extractLink(c)
		if c.FirstChild != nil {
			getAllLinks(originUrl, c.FirstChild, links)
		}
	}
}

// search the child nodes of a html link node for a text node
// an example of that would be a h3 node that serves as the link text
// func getLinkText(linkNode *html.Node) string
