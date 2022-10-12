package main

import (
	"crypto/sha512"
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-redis/redis"
	"github.com/uptrace/bun"
)

type Link struct {
	TimeFound       time.Time
	OrigUrl         *url.URL
	DestUrl         *url.URL
	SurroundingNode []byte
}

type LinkRel struct {
	ID          int64 `bun:",pk,autoincrement"`
	TimeFound   int64
	Origin      int64
	Destination int64
}

// type LinkText struct {
// 	ID              int64 `bun:",pk,autoincrement"`
// 	LinkID          int64
// 	SurroundingNode []byte
// }

type Site struct {
	ID  int64 `bun:",pk,autoincrement"`
	Url string
}

type Content struct {
	ID             int64 `bun:",pk,autoincrement"`
	TimeFound      int64
	SiteID         int64
	ContentType    string
	HttpStatusCode int
	Size           int
	Hash           *[sha512.Size]byte
}

type Errors struct {
	ID                      int64 `bun:",pk,autoincrement"`
	Time                    time.Time
	Url                     string
	ParsingError            bool
	ResponseToBig           bool
	ErrorReading            bool
	ResponseSizeUneqContLen bool
	HttpStatusCode          int
}

const (
	createNewDb  = true
	maxFilesize  = 2e7
	maxNumOfUrls = 1e7 // an estimate of how many urls we want to index
)

var lockDb sync.Mutex
var sitesIndexed int
var db *bun.DB
var rdb *redis.Client

func main() {
	// create a channel to receive certain syscalls
	sigChan := make(chan os.Signal, 1)

	// parse command line arguments
	dbPath := flag.String("dbPath", "testDbxxx.sqlite", "Path to the database")
	rawStartUrl := flag.String("url", "", "Url to start crawling at")
	numOfRoutines := flag.Int("n", 3, "Number of crawlers to run in parralel")
	flag.Parse()

	// get database clients
	rdb = getRedisClient()
	defer rdb.Close()
	db = getDb(*dbPath)
	defer db.Close()

	// create channels
	linksChan := make(chan *Link, 1e2)  // extractFromPage -> saveNewLink
	newUrls := make(chan *url.URL, 1e2) // saveNewLink -> handleQueue

	// start queueWorker
	go addToQueue(newUrls)

	// send startUrl to channel
	startUrl, err := url.Parse(*rawStartUrl)
	check(err)
	newUrls <- startUrl

	// handle SIGTERM
	go handleSigTerm(sigChan)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go extractFromPage(linksChan, "highPrioQueue")
	// start goroutines
	for i := 1; i <= *numOfRoutines; i++ {
		go saveNewLink(linksChan, newUrls)
		go extractFromPage(linksChan, "normalPrioQueue")
	}

	// print a staus update every two seconds
	log.Printf("Starting to crawl at: %v", *rawStartUrl)
	for {
		time.Sleep(2 * time.Second)

		log.Printf("Visited %v links", sitesIndexed)
	}
}

func handleSigTerm(sig chan os.Signal) {
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

func checkRedisErr(err error) {
	if err != redis.Nil && err != nil {
		panic(err)
	}
}
