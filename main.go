package main

import (
	"crypto/sha512"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/uptrace/bun"
)

type Link struct {
	ID              int64 `bun:",pk,autoincrement"`
	TimeFound       time.Time
	OrigUrl         string
	DestUrl         string
	LinkText        string
	SurroundingNode []byte
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
	sigChan := make(chan os.Signal, 1)

	testUrl := os.Args[1]
	log.Printf("Starting to crawl at: %v", testUrl)

	rdb := getRedisClient()
	defer rdb.Close()
	db := getDb("testDb006.sqlite")
	defer db.Close()
	linksChan := make(chan *Link, 1e4)

	linksChan <- &Link{TimeFound: time.Now(), DestUrl: testUrl}

	// handle SIGTERM
	go handleSigTerm(sigChan, db)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	for i := 1; i < 4; i++ {
		go handleNewPage(linksChan, db, rdb)
	}

	for {
		time.Sleep(2 * time.Second)

		visited, err := rdb.SCard("visitedLinks").Result()
		check(err)
		log.Printf("Visited %v links", visited)
	}
}

func handleSigTerm(sig chan os.Signal, db *bun.DB) {
	received := <-sig
	log.Printf("Received signal %v", received)

	log.Printf("Locking database in order to close it")
	lockDb.Lock()
	log.Printf("Database locked")
	db.Close()
	log.Printf("Database closed")

	os.Exit(0)
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
