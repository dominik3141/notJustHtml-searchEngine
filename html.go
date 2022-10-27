package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kagami/go-face"
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
		if qPriority == 100 {
			_, loaded := goodDomains.LoadOrStore(url.Hostname(), true)
			if !loaded {
				log.Println("goodDomains+=", url.Hostname())
			}
		}

		// GET the body using chrome
		if *useChromedp && !(strings.HasSuffix(rawUrl, ".jpg") || strings.HasSuffix(rawUrl, ".png") || strings.HasSuffix(rawUrl, ".mp4") || strings.HasSuffix(rawUrl, ".js") || strings.HasSuffix(rawUrl, ".jpeg")) {
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

		var percHashes *PerceptualHash
		var exif *ExifInfo
		var faces []face.Face
		err = nil

		saveFileToDatabase(&sha1Sum, &body)
		switch contentTypeStr {
		case "image/png":
			percHashes = calcPercptualHashes(contentTypeStr, bytes.NewReader(body), url.String())
			exif = getExif(bytes.NewReader(body), url.String())
			faces, err = indexFile(&body, false, true)
			if err != nil {
				logErrorToDb(err, ErrorFaceRecognition, url.String())
			}
		case "image/jpeg":
			percHashes = calcPercptualHashes(contentTypeStr, bytes.NewReader(body), url.String())
			exif = getExif(bytes.NewReader(body), url.String())
			faces, err = indexFile(&body, false, false)
			if err != nil {
				logErrorToDb(err, ErrorFaceRecognition, url.String())
			}
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
		}

		// insert perceptual hashes to the database
		if percHashes != nil {
			percHashes.ContentId = content.ID
			_, err = db.NewInsert().Model(percHashes).Exec(context.Background())
			handleBunSqlErr(err)
		}

		// insert face profiles to the database
		if faces != nil && len(faces) != 0 {
			for i := range faces {
				dbFace := Face{
					ContentId:  content.ID,
					Descriptor: faces[i].Descriptor,
					Rectangle:  faces[i].Rectangle,
					Shapes:     faces[i].Shapes,
				}

				_, err = db.NewInsert().Model(&dbFace).Exec(context.Background())
				check(err)
			}
		}

		// check if content type is html, otherwise the file can not be searched for links
		if len(contentTypeStr) >= 9 && contentTypeStr[:9] != "text/html" {
			continue
		}
		if len(contentTypeStr) < 9 {
			continue
		}

		if strings.HasSuffix(url.String(), ".jpeg") || strings.HasSuffix(url.String(), ".png") || strings.HasSuffix(url.String(), ".jpg") {
			continue
		}
		if debugMode {
			log.Println("Crawling ", url.String())
		}

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
