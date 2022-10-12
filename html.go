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

func handleNewPage(urlToCrawlChan chan<- *Link, linksChan <-chan *Link, db *bun.DB, rdb *redis.Client) {
	log.Println("Started goroutine.")

	for {
		link := <-linksChan

		// add link to database
		lockDb.Lock()
		_, err := db.NewInsert().Model(link).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()

		// query redis to make sure the url has not been indexed before
		done, err := rdb.SIsMember("visitedLinks", link.DestUrl).Result()
		check(err)
		if done {
			continue
		}

		// send a new url to the crawlers
		urlToCrawlChan <- link

		// set link to done in redis
		err = rdb.SAdd("visitedLinks", link.DestUrl).Err()
		check(err)
	}
}

func extractFromPage(urls <-chan *Link, db *bun.DB, linksChan chan<- *Link) {
	bodyContainer := make([]byte, maxFilesize)
	var body []byte

	for {
		link := <-urls

		// parse the url that we want to index
		url, err := url.Parse(link.DestUrl)
		if err != nil {
			log.Printf("URL: %v. Url parsing error. err=%v", link.DestUrl, err)
			continue
		}

		// GET the url
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
			continue
		}

		// check if response body is to large for use to handle
		if resp.ContentLength >= maxFilesize {
			log.Printf("URL: %v. Webpage is to big.", url.String())
			dbErr := Errors{Url: url.String(), Time: time.Now(), ResponseToBig: true}
			lockDb.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleSqliteErr(err)
			lockDb.Unlock()
			continue
		}

		// read body
		n, err := resp.Body.Read(bodyContainer)
		if err != nil && err != io.EOF {
			log.Printf("URL: %v. Error reading response body. err=%v", url.String(), err)
			dbErr := Errors{Url: url.String(), Time: time.Now(), ErrorReading: true}
			lockDb.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleSqliteErr(err)
			lockDb.Unlock()
			continue
		}
		if n != int(resp.ContentLength) && resp.ContentLength != -1 {
			log.Printf("URL: %v. Length of response is different from the content length indicated in the response header. %v vs. %v", url.String(), n, resp.ContentLength)
			dbErr := Errors{Url: url.String(), Time: time.Now(), ResponseSizeUneqContLen: true}
			lockDb.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleSqliteErr(err)
			lockDb.Unlock()
		}
		err = resp.Body.Close()
		check(err)
		body = bodyContainer[:n]

		// Retrieve content information
		hash := sha512.Sum512(body)
		var contentType string
		if n >= 512 {
			contentType = http.DetectContentType(body[:512])
		} else {
			contentType = http.DetectContentType(body)
		}
		content := Content{TimeFound: time.Now(), Url: url.String(), ContentType: contentType, Hash: &hash, Size: n, HttpStatusCode: resp.StatusCode}
		lockDb.Lock()
		_, err = db.NewInsert().Model(&content).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()

		// check if content type is html, otherwise the file can not be searched for links
		if contentType[:9] != "text/html" {
			log.Printf("URL: %v. Content type is %v", link.DestUrl, contentType)
			continue
		}

		// parse html
		rootNode, err := html.Parse(bytes.NewReader(body))
		if err != nil {
			log.Printf("URL: %v. html parsing error. err=%v", url, err)
			dbErr := Errors{Url: url.String(), ParsingError: true, Time: time.Now()}
			lockDb.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleSqliteErr(err)
			lockDb.Unlock()
			continue
		}

		// get all links that can be found on this site and add send them to the channel
		getAllLinks(url, rootNode, linksChan)
	}
}
