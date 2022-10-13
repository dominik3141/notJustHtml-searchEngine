package main

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/html"
)

func saveNewLink(inChan <-chan *Link, outChan chan<- *url.URL) {
	for {
		link := <-inChan

		linkRel := &LinkRel{
			TimeFound:   link.TimeFound.UnixMicro(),
			Origin:      getSiteID(link.OrigUrl.String()),
			Destination: getSiteID(link.DestUrl.String()),
			Keywords:    link.Keywords,
		}

		// add link to database
		lockDb.Lock()
		_, err := db.NewInsert().Model(linkRel).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()

		outChan <- link.DestUrl
	}
}

func extractFromPage(outChan chan<- *Link, queueName string) {
	bodyContainer := make([]byte, maxFilesize)
	var body []byte

	for {
		rawUrl, err := rdb.SPop(queueName).Result()
		checkRedisErr(err)
		if rawUrl == "" {
			time.Sleep(time.Second)
			continue
		}

		// parse the url that we want to index
		url, err := url.Parse(rawUrl)
		if err != nil {
			log.Printf("URL: %v. Url parsing error. err=%v", rawUrl, err)
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
		var contentTypeStr string
		if n >= 512 {
			contentTypeStr = http.DetectContentType(body[:512])
		} else {
			contentTypeStr = http.DetectContentType(body)
		}

		hashBase64 := base64.StdEncoding.EncodeToString(hash[:])
		switch contentTypeStr {
		case "text/html":
		case "text/javascript":
		case "image/png":
			saveToFile(hashBase64+".png", &body)
		case "image/jpeg":
			saveToFile(hashBase64+".jpg", &body)
		case "application/x-gzip":
			saveToFile(hashBase64+".gz", &body)
		case "text/plain":
			saveToFile(hashBase64+".txt", &body)
		case "application/java-archive":
			saveToFile(hashBase64+".jar", &body)
		case "text/csv":
			saveToFile(hashBase64+".csv", &body)
		case "application/json":
			saveToFile(hashBase64+".json", &body)
		case "application/zip":
			saveToFile(hashBase64+".zip", &body)
		case "application/pdf":
			saveToFile(hashBase64+".pdf", &body)
		case "video/mp4":
			saveToFile(hashBase64+".mp4", &body)
		case "application/oxtet-stream":
			saveToFile(hashBase64+".bin", &body)
		case "application/vnd.android.package-archive":
			saveToFile(hashBase64+".apk", &body)
		case " application/x-msdownload":
			saveToFile(hashBase64+".exe", &body)
		case " application/word":
			saveToFile(hashBase64+".doc", &body)
		case " application/excel":
			saveToFile(hashBase64+".xl", &body)
		}
		content := Content{TimeFound: time.Now().UnixMicro(), SiteID: getSiteID(url.String()), ContentTypeId: getContentTypeId(contentTypeStr), Hash: &hash, Size: n, HttpStatusCode: resp.StatusCode}
		lockDb.Lock()
		_, err = db.NewInsert().Model(&content).Exec(context.Background())
		handleSqliteErr(err)
		lockDb.Unlock()

		// check if content type is html, otherwise the file can not be searched for links
		if len(contentTypeStr) >= 8 && contentTypeStr[:9] != "text/html" {
			// log.Printf("URL: %v. Content type is %v", url.String(), contentType)
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
		getAllLinks(url, rootNode, outChan)
	}
}
