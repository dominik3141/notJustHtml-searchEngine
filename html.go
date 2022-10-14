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

func saveNewLink(inChan <-chan *Link, outChan chan<- *Link) {
	for {
		link := <-inChan

		linkRel := &LinkRel{
			TimeFound:   link.TimeFound.UnixMicro(),
			Origin:      getSiteID(link.OrigUrl.String()),
			Destination: getSiteID(link.DestUrl.String()),
			Keywords:    *link.Keywords,
		}

		// add link to database
		dbMutex.Lock()
		_, err := db.NewInsert().Model(linkRel).Exec(context.Background())
		handleBunSqlErr(err)
		dbMutex.Unlock()

		outChan <- link
	}
}

func extractFromPage(outChan chan<- *Link, queueName string) {
	body := make([]byte, maxFilesize)
	var n int

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
			dbMutex.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			dbMutex.Unlock()
			continue
		}

		// check if response body is to large for use to handle
		if resp.ContentLength >= maxFilesize {
			log.Printf("URL: %v. Webpage is to big.", url.String())
			dbErr := Errors{Url: url.String(), Time: time.Now(), ResponseToBig: true}
			dbMutex.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			dbMutex.Unlock()
			continue
		}

		// read body
		body = body[:0]
		for {
			n, err = resp.Body.Read(body[len(body):cap(body)])
			body = body[:len(body)+n]
			if err != nil {
				if err == io.EOF {
					break
				}
			}
		}
		if err != nil && err != io.EOF {
			log.Printf("URL: %v. Error reading response body. err=%v", url.String(), err)
			dbErr := Errors{Url: url.String(), Time: time.Now(), ErrorReading: true}
			dbMutex.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			dbMutex.Unlock()
			continue
		}
		n = len(body)
		if n != int(resp.ContentLength) && resp.ContentLength != -1 {
			// log.Printf("URL: %v. Length of response is different from the content length indicated in the response header. %v vs. %v", url.String(), n, resp.ContentLength)
			dbErr := Errors{Url: url.String(), Time: time.Now(), ResponseSizeUneqContLen: true}
			dbMutex.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			dbMutex.Unlock()
		}
		err = resp.Body.Close()
		check(err)

		// Retrieve content information
		sha512Sum := sha512.Sum512(body)
		var contentTypeStr string
		if n >= 512 {
			contentTypeStr = http.DetectContentType(body[:512])
		} else {
			contentTypeStr = http.DetectContentType(body)
		}

		sha512SumUrlBase64 := base64.URLEncoding.EncodeToString(sha512Sum[:])
		sha512SumUrlBase64 = sha512SumUrlBase64[:20]
		switch contentTypeStr {
		case "text/html":
		case "text/javascript":
		case "image/png":
			saveToFile(sha512SumUrlBase64+".png", &body)
		case "image/jpeg":
			saveToFile(sha512SumUrlBase64+".jpg", &body)
		case "application/x-gzip":
			saveToFile(sha512SumUrlBase64+".gz", &body)
		case "text/plain":
			saveToFile(sha512SumUrlBase64+".txt", &body)
		case "application/java-archive":
			saveToFile(sha512SumUrlBase64+".jar", &body)
		case "text/csv":
			saveToFile(sha512SumUrlBase64+".csv", &body)
		case "application/json":
			saveToFile(sha512SumUrlBase64+".json", &body)
		case "application/zip":
			saveToFile(sha512SumUrlBase64+".zip", &body)
		case "application/pdf":
			saveToFile(sha512SumUrlBase64+".pdf", &body)
		case "video/mp4":
			saveToFile(sha512SumUrlBase64+".mp4", &body)
		case "application/oxtet-stream":
			saveToFile(sha512SumUrlBase64+".bin", &body)
		case "application/vnd.android.package-archive":
			saveToFile(sha512SumUrlBase64+".apk", &body)
		case " application/x-msdownload":
			saveToFile(sha512SumUrlBase64+".exe", &body)
		case " application/word":
			saveToFile(sha512SumUrlBase64+".doc", &body)
		case " application/excel":
			saveToFile(sha512SumUrlBase64+".xl", &body)
		}
		content := Content{TimeFound: time.Now().UnixMicro(), SiteID: getSiteID(url.String()), ContentTypeId: getContentTypeId(contentTypeStr), Sha512Sum: &sha512Sum, Size: n, HttpStatusCode: resp.StatusCode}
		dbMutex.Lock()
		_, err = db.NewInsert().Model(&content).Exec(context.Background())
		handleBunSqlErr(err)
		dbMutex.Unlock()

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
			dbMutex.Lock()
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			dbMutex.Unlock()
			continue
		}

		// get all links that can be found on this site and add send them to the channel
		getAllLinks(url, rootNode, outChan)
	}
}
