package main

import (
	"context"
	"log"
	"time"

	"github.com/go-redis/redis"
)

type Errors struct {
	ID        int64 `bun:",pk,autoincrement"`
	Time      time.Time
	Url       string
	ErrorCode ErrorCode
	ErrorText string
	// HttpStatusCode int
	// ParsingError            bool
	// ResponseToBig           bool
	// ErrorReading            bool
	// ResponseSizeUneqContLen bool
}

type ErrorCode int

const (
	ErrorParsingHtml ErrorCode = iota
	ErrorResponseToBig
	ErrorReadingBody
	ErrorResponseSizeUneqContLen
	ErrorReadExif
	ErrorPerceptualHash
	ErrorParsingUrl
	ErrorUrlGet
	ErrorBodyLenZero
)

func logErrorToDb(err error, errCode ErrorCode, url string) {
	if debugMode {
		log.Printf("URL: %v.\terrCode:%v\terr=%v", url, errCode, err)
	}
	dbErr := Errors{
		Url:       url,
		ErrorCode: ErrorParsingHtml,
		Time:      time.Now(),
		ErrorText: err.Error(),
	}

	_, err = db.NewInsert().Model(&dbErr).Exec(context.Background())
	handleBunSqlErr(err)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func handleBunSqlErr(err error) {
	if err != nil {
		panic(err)
	}
}

func checkRedisErr(err error) {
	if err != redis.Nil && err != nil {
		panic(err)
	}
}
