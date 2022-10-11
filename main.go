package main

import (
	"crypto/sha512"
	"log"
	"os"
	"sync"
	"time"
)

type Link struct {
	ID        int64 `bun:",pk,autoincrement"`
	TimeFound time.Time
	OrigUrl   string
	DestUrl   string
	LinkText  string
}

type Content struct {
	ID          int64 `bun:",pk,autoincrement"`
	TimeFound   time.Time
	Url         string
	ContentType string
	Size        int
	Hash        [sha512.Size]byte
}

type Errors struct {
	ID             int64 `bun:",pk,autoincrement"`
	Time           time.Time
	Url            string
	ParsingError   bool
	ResponseToBig  bool
	ErrorReading   bool
	HttpStatusCode int
}

const createNewDb = true

var lockDb sync.Mutex

func main() {
	testUrl := os.Args[1]
	log.Printf("Starting to crawl at: %v", testUrl)

	rdb := getRedisClient()
	defer rdb.Close()
	db := getDb("testDb004.sqlite")
	defer db.Close()
	linksChan := make(chan *Link, 1e4)

	linksChan <- &Link{TimeFound: time.Now(), DestUrl: testUrl}

	for i := 1; i < 10; i++ {
		go handleNewPage(linksChan, db, rdb)
	}

	for {
		time.Sleep(2 * time.Second)

		visited, err := rdb.SCard("visitedLinks").Result()
		check(err)
		log.Printf("Visited %v links", visited)
	}
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func handleSqliteErr(err error) {
	if err != nil {
		panic(err)
	}
}
