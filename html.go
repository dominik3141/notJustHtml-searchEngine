package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/html"
)

func extractFromPage(outChan chan<- *Link, qPriority int) {
	body := make([]byte, maxFilesize)
	var n int
	var rawUrl string
	var err error

	for {
		for {
			rawUrl, err = rdb.SPop(fmt.Sprintf("QueuePriority%v", qPriority)).Result()
			checkRedisErr(err)
			if rawUrl != "" {
				break
			}
			time.Sleep(2 * time.Second)
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
			// log.Printf("URL: %v. html get error. err=%v", url.String(), err)
			dbErr := Errors{Url: url.String(), Time: time.Now()}
			if resp != nil {
				dbErr.HttpStatusCode = resp.StatusCode
			}
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			continue
		}
		defer resp.Body.Close()

		// check if response body is to large for use to handle
		if resp.ContentLength >= maxFilesize {
			log.Printf("URL: %v. Webpage is to big.", url.String())
			dbErr := Errors{Url: url.String(), Time: time.Now(), ResponseToBig: true}
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
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
		n = len(body)
		if (err != nil && err != io.EOF) || n == 0 {
			log.Printf("URL: %v. Error reading response body. err=%v", url.String(), err)
			dbErr := Errors{Url: url.String(), Time: time.Now(), ErrorReading: true}
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			continue
		}
		// check if content length indicated in the http header equals the number of bytes that we did actually read
		if n != int(resp.ContentLength) && resp.ContentLength != -1 {
			dbErr := Errors{Url: url.String(), Time: time.Now(), ResponseSizeUneqContLen: true}
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
		}

		// Retrieve content information
		sha512Sum := sha512.Sum512(body)
		sha1Sum := sha1.Sum(body)
		var contentTypeStr string
		if n >= 512 {
			contentTypeStr = http.DetectContentType(body[:512])
		} else {
			contentTypeStr = http.DetectContentType(body)
		}

		sha1SumUrlBase64 := base64.URLEncoding.EncodeToString(sha1Sum[:])

		var percHashes *PerceptualHash
		var exif *ExifInfo
		err = nil
		switch contentTypeStr {
		case "text/html":
		case "text/javascript":
		case "image/png":
			// saveToFile(sha1SumUrlBase64+".png", &body)
			percHashes, _ = calcPercptualHashes(contentTypeStr, bytes.NewReader(body))
			exif = getExif(bytes.NewReader(body), url.String())
		case "image/jpeg":
			// saveToFile(sha1SumUrlBase64+".jpg", &body)
			percHashes, _ = calcPercptualHashes(contentTypeStr, bytes.NewReader(body))
			exif = getExif(bytes.NewReader(body), url.String())
		case "application/x-gzip":
			saveToFile(sha1SumUrlBase64+".gz", &body)
		case "text/plain":
			saveToFile(sha1SumUrlBase64+".txt", &body)
		case "application/java-archive":
			saveToFile(sha1SumUrlBase64+".jar", &body)
		case "text/csv":
			saveToFile(sha1SumUrlBase64+".csv", &body)
		case "application/json":
			saveToFile(sha1SumUrlBase64+".json", &body)
		case "application/zip":
			saveToFile(sha1SumUrlBase64+".zip", &body)
		case "application/pdf":
			saveToFile(sha1SumUrlBase64+".pdf", &body)
		case "video/mp4":
			saveToFile(sha1SumUrlBase64+".mp4", &body)
		case "application/oxtet-stream":
			saveToFile(sha1SumUrlBase64+".bin", &body)
		case "application/vnd.android.package-archive":
			saveToFile(sha1SumUrlBase64+".apk", &body)
		case " application/x-msdownload":
			saveToFile(sha1SumUrlBase64+".exe", &body)
		case " application/word":
			saveToFile(sha1SumUrlBase64+".doc", &body)
		case " application/excel":
			saveToFile(sha1SumUrlBase64+".xls", &body)
		}

		content := Content{
			TimeFound:      time.Now().UnixMicro(),
			SiteID:         getSiteID(url.String()),
			ContentTypeId:  getContentTypeId(contentTypeStr),
			HttpStatusCode: resp.StatusCode,
			Size:           n,
			Sha512Sum:      &sha512Sum,
			Sha1Sum:        &sha1Sum,
		}
		_, err = db.NewInsert().Model(&content).Returning("id").Exec(context.Background())
		handleBunSqlErr(err)

		// insert exif information to database
		if exif != nil {
			exif.ContentId = content.ID
			_, err = db.NewInsert().Model(exif).Exec(context.Background())
			handleBunSqlErr(err)
			if exif.Lat != 0 {
				goodDomains.LoadOrStore(url.Hostname(), true)
			}
		}

		// insert perceptual hashes to the database
		if percHashes != nil {
			percHashes.ContentId = content.ID
			_, err = db.NewInsert().Model(percHashes).Exec(context.Background())
			handleBunSqlErr(err)
		}

		// check if content type is html, otherwise the file can not be searched for links
		if len(contentTypeStr) >= 9 && contentTypeStr[:9] != "text/html" {
			continue
		}

		// parse html
		rootNode, err := html.Parse(bytes.NewReader(body))
		if err != nil {
			log.Printf("URL: %v. html parsing error. err=%v", url, err)
			dbErr := Errors{Url: url.String(), ErrorCode: ErrorParsingHtml, Time: time.Now()}
			_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
			handleBunSqlErr(err)
			continue
		}

		// get all links that can be found on this site and add send them to the channel
		getAllLinks(url, rootNode, outChan)
	}
}
