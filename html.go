package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
			logErrorToDb(err, ErrorParsingUrl, rawUrl)
			continue
		}

		// GET the body using chrome
		if *useChromedp && !strings.HasSuffix(rawUrl, ".jpg") && !strings.HasSuffix(rawUrl, ".png") && !strings.HasSuffix(rawUrl, ".mp4") && !strings.HasSuffix(rawUrl, ".js") && !strings.HasSuffix(rawUrl, ".jpeg") {
			bodyStr := getPageWithChrome(url.String())
			body = []byte(*bodyStr)
		} else {
			// GET the url
			resp, err := http.Get(url.String())
			if err != nil {
				logErrorToDb(err, ErrorUrlGet, url.String())
				continue
			}
			defer resp.Body.Close()

			// check if response body is to large for use to handle
			if resp.ContentLength >= maxFilesize {
				logErrorToDb(err, ErrorResponseToBig, url.String())
				continue
			}

			// read body
			body = body[:0]
			for {
				n, err = resp.Body.Read(body[len(body):cap(body)])
				body = body[:len(body)+n]
				if err != nil {
					break
				}
			}
			n = len(body)

			// handle body read errors
			if err != nil && err != io.EOF {
				logErrorToDb(err, ErrorReadingBody, url.String())
				continue
			}
			if n == 0 {
				logErrorToDb(nil, ErrorBodyLenZero, url.String())
				continue
			}

			// check if content length indicated in the http header equals the number of bytes that we did actually read
			if n != int(resp.ContentLength) && resp.ContentLength != -1 {
				logErrorToDb(err, ErrorResponseSizeUneqContLen, url.String())
			}
		}

		// Retrieve content information
		sha512Sum := sha512.Sum512(body)
		sha1Sum := sha1.Sum(body)
		contentTypeStr := http.DetectContentType(body)

		sha1SumUrlBase64 := base64.URLEncoding.EncodeToString(sha1Sum[:])

		var percHashes *PerceptualHash
		var exif *ExifInfo
		err = nil
		switch contentTypeStr {
		case "text/html":
		case "text/javascript":
		case "image/png":
			// saveToFile(sha1SumUrlBase64+".png", &body)
			percHashes = calcPercptualHashes(contentTypeStr, bytes.NewReader(body), url.String())
			exif = getExif(bytes.NewReader(body), url.String())
		case "image/jpeg":
			// saveToFile(sha1SumUrlBase64+".jpg", &body)
			percHashes = calcPercptualHashes(contentTypeStr, bytes.NewReader(body), url.String())
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
			TimeFound:     time.Now().UnixMicro(),
			SiteID:        getSiteID(url),
			ContentTypeId: getContentTypeId(contentTypeStr),
			Size:          n,
			Sha512Sum:     &sha512Sum,
			Sha1Sum:       &sha1Sum,
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
		// if strings.HasSuffix(url.String(), ".jpeg") && strings.HasSuffix(url.String(), ".png") && strings.HasSuffix(url.String(), ".jpg") && len(contentTypeStr) >= 9 && contentTypeStr[:9] != "text/html" {
		// 	continue
		// }

		// parse html
		rootNode, err := html.Parse(bytes.NewReader(body))
		if err != nil {
			logErrorToDb(err, ErrorParsingHtml, url.String())
			continue
		}

		// get all links that can be found on this site and add send them to the channel
		getAllLinks(url, rootNode, outChan)
	}
}
