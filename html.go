package main

import (
	"bytes"
	"context"
	"crypto/sha512"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/go-redis/redis"
	"github.com/uptrace/bun"
	"golang.org/x/net/html"
)

func handleNewPage(linksChan chan *Link, db *bun.DB, rdb *redis.Client) {
	log.Println("Started goroutine.")
	for {
		link := <-linksChan

		// add link to database
		lockDb.Lock()
		_, err := db.NewInsert().Model(&link).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()

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

func extractFromPage(originUrl string, link *Link, db *bun.DB, linksChan chan<- *Link) {
	url, err := url.Parse(link.DestUrl)
	if err != nil {
		log.Printf("URL: %v. Url parsing error. err=%v", link.DestUrl, err)
		return
	}

	resp, err := http.Get(url.String())
	if err != nil {
		log.Printf("URL: %v. html get error. err=%v", url.String(), err)
		dbErr := Errors{Url: url.String(), Time: time.Now()}
		if resp != nil {
			dbErr.HttpStatusCode = resp.StatusCode
		}
		lockDb.Lock()
		_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()
		return
	}

	if resp.ContentLength >= 1e8 {
		log.Printf("URL: %v. Webpage is to big.", url.String())
		dbErr := Errors{Url: url.String(), Time: time.Now(), ResponseToBig: true}
		lockDb.Lock()
		_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()
		return
	}

	// read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("URL: %v. Error reading response body", url.String())
		dbErr := Errors{Url: url.String(), Time: time.Now(), ErrorReading: true}
		lockDb.Lock()
		_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()
		return
	}
	err = resp.Body.Close()
	check(err)

	// Retrieve content information
	content := Content{Url: url.String(), ContentType: http.DetectContentType(body[:512]), Hash: sha512.Sum512(body), Size: len(body)}
	lockDb.Lock()
	_, err = db.NewInsert().Model(&content).Exec(context.Background())
	handleSqliteErr(err)
	lockDb.Unlock()

	rootNode, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		log.Printf("URL: %v. html parsing error. err=%v", url, err)
		dbErr := Errors{Url: url.String(), ParsingError: true, Time: time.Now()}
		lockDb.Lock()
		_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()
		return
	}

	getAllLinks(url, rootNode, linksChan)
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

				link := Link{
					OrigUrl:   originUrl.String(),
					DestUrl:   linkDst.String(),
					LinkText:  c.Data,
					TimeFound: time.Now(),
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
