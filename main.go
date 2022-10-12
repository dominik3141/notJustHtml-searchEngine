package main

import (
	"crypto/sha512"
	"flag"
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
	SurroundingNode []byte
}

type Content struct {
	ID          int64 `bun:",pk,autoincrement"`
	TimeFound   time.Time
	Url         string
	ContentType string
	Size        int
	Hash        *[sha512.Size]byte
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
const maxFilesize = 1e8

var lockDb sync.Mutex

func main() {
	// create a channel to receive certain syscalls
	sigChan := make(chan os.Signal, 1)

	// parse command line arguments
	dbPath := flag.String("dbPath", "testDbxxx.sqlite", "Path to the database")
	startUrl := flag.String("url", "", "Url to start crawling at")
	numOfRoutines := flag.Int("n", 3, "Number of crawlers to run in parralel")
	flag.Parse()

	// get database clients
	rdb := getRedisClient()
	defer rdb.Close()
	db := getDb(*dbPath)
	defer db.Close()

	// create channels
	linksChan := make(chan *Link, 1e2)
	urlToCrawlChan := make(chan *Link, 1e2)

	// send startUrl to channel
	linksChan <- &Link{TimeFound: time.Now(), DestUrl: *startUrl}

	// handle SIGTERM
	go handleSigTerm(sigChan, db)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// start goroutines
	for i := 1; i <= *numOfRoutines; i++ {
		go handleNewPage(urlToCrawlChan, linksChan, db, rdb)
		go extractFromPage(urlToCrawlChan, db, linksChan)
	}

	// print a staus update every two seconds
	log.Printf("Starting to crawl at: %v", *startUrl)
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
