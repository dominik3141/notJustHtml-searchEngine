package main

import (
	"crypto/sha1"
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

type HtmlText struct {
	Visibility int
	Text       string
}

type Link struct {
	TimeFound time.Time
	OrigUrl   *url.URL
	DestUrl   *url.URL
	Keywords  *[]HtmlText
	Rating    float64
	Priority  int
}

type LinkRel struct {
	ID          int64 `bun:",pk,autoincrement"`
	TimeFound   int64
	Origin      int64
	Destination int64
	Keywords    []HtmlText
	Rating      float64
}

type Site struct {
	ID  int64 `bun:",pk,autoincrement"`
	Url string
}

type Content struct {
	ID             int64 `bun:",pk,autoincrement"`
	TimeFound      int64
	SiteID         int64
	ContentTypeId  int64
	HttpStatusCode int
	Size           int
	Sha512Sum      *[sha512.Size]byte
	Sha1Sum        *[sha1.Size]byte
	AverageHash    uint64 // a perceptual hash
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
	maxFilesize  = 2e8
	maxNumOfUrls = 1e7 // an estimate of how many urls we want to index
)

var dbMutex sync.Mutex
var sitesIndexed int
var db *bun.DB
var rdb *redis.Client
var contentTypeToIdCache sync.Map
var knownDomains sync.Map

func main() {
	// create a channel to receive certain syscalls
	sigChan := make(chan os.Signal, 1)

	// parse command line arguments
	dbPath := flag.String("dbPath", "testDbxxx.sqlite", "Path to the database")
	numOfRoutines := flag.Int("n", 3, "Number of crawlers to run in parallel")
	flag.Parse()

	// get database clients
	rdb = getRedisClient()
	defer rdb.Close()
	db = getDb(*dbPath)
	defer db.Close()

	// create channels
	linksChan := make(chan *Link, 1e3) // extractFromPage -> saveNewLink
	newUrls := make(chan *Link, 1e3)   // saveNewLink -> handleQueue

	// load the list of flaggedWords
	flaggedWords := loadFlaggedWords()

	// start queueWorker
	go addToQueue(newUrls)

	// add to startSites
	addStartSites(newUrls)

	// handle SIGTERM
	go handleSigTerm(sigChan)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// start goroutines
	for i := 1; i <= *numOfRoutines; i++ {
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go extractFromPage(linksChan, "QueuePriority20")
		go extractFromPage(linksChan, "QueuePriority30")
		go extractFromPage(linksChan, "QueuePriority70")
		go extractFromPage(linksChan, "QueuePriority70")
		go extractFromPage(linksChan, "QueuePriority100")
		go extractFromPage(linksChan, "QueuePriority100")
	}

	// print a status update every two seconds
	for {
		time.Sleep(2 * time.Second)

		log.Printf("Visited %v links", sitesIndexed)
	}
}

func handleSigTerm(sig chan os.Signal) {
	received := <-sig
	log.Printf("Received signal %v", received)

	log.Printf("Locking database in order to close it")
	dbMutex.Lock()
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
