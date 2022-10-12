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
	for {
		link := <-urls

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

		if resp.ContentLength >= maxFilesize {
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
		hash := sha512.Sum512(body)
		content := Content{TimeFound: time.Now(), Url: url.String(), ContentType: http.DetectContentType(body[:512]), Hash: &hash, Size: len(body)}
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
}
