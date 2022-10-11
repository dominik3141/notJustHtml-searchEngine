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
	log.Println("Started goroutine.")
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

		go extractFromPage(link.OrigUrl, link, db, linksChan)

		// set link to done in redis
		err = rdb.SAdd("visitedLinks", link.DestUrl).Err()
		check(err)
	}
}

func extractFromPage(originUrl string, link Link, db *bun.DB, linksChan chan<- Link) {
	url, err := url.Parse(link.DestUrl)
	if err != nil {
		log.Printf("URL: %v. Url parsing error. err=%v", link.DestUrl, err)
		return
	}

	resp, err := http.Get(url.String())
	if err != nil {
		log.Printf("URL: %v. html get error. err=%v", url.String(), err)
		dbErr := GetErr{Url: url.String(), Time: time.Now()}
		if resp != nil {
			dbErr.HttpStatusCode = resp.StatusCode
		}
		_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
		handleSqliteErr(err)
		return
	}

	rootNode, err := html.Parse(resp.Body)
	if err != nil {
		log.Printf("URL: %v. html parsing error. err=%v", url, err)
		dbErr := GetErr{Url: url.String(), ParsingError: true, Time: time.Now()}
		_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
		handleSqliteErr(err)
		return
	}

	getAllLinks(url, rootNode, linksChan)
	err = resp.Body.Close()
	check(err)
}

// gets all links starting at a given html node
// all found links are send to a go channel
func getAllLinks(originUrl *url.URL, node *html.Node, links chan<- Link) {
	extractLink := func(c *html.Node) {
		for _, a := range c.Attr {
			if a.Key == "href" {
				linkDst, err := url.Parse(a.Val)
				if err != nil {
					log.Println("Malformed url:", a.Val)
					break
				}

				if linkDst.Hostname() == "" {
					linkDst.Host = originUrl.Host
					linkDst.Scheme = originUrl.Scheme
					// log.Printf("Corrected url from %v to %v", a.Val, linkDst.String())
				}

				link := Link{
					OrigUrl: originUrl.String(),
					DestUrl: linkDst.String(),
					// LinkText: c.,
					TimeFound: time.Now(),
				}

				links <- link
				break
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
