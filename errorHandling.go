package main

import (
	"time"

	"github.com/go-redis/redis"
)

type Errors struct {
	ID             int64 `bun:",pk,autoincrement"`
	Time           time.Time
	Url            string
	HttpStatusCode int
	ErrorCode      ErrorCode
	ErrorText      string
	// ParsingError            bool
	// ResponseToBig           bool
	// ErrorReading            bool
	// ResponseSizeUneqContLen bool
}

type ErrorCode int

const (
	ErrorParsingHtml ErrorCode = iota
	ErrorResponseToBig
	ErrorReading
	ErrorResponseSizeUneqContLen
	ErrorReadExif
	ErrorPerceptualHash
)

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
