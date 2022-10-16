package main

import (
	"context"
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

	"github.com/bits-and-blooms/bloom/v3"
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
	Rating      float64
}

type LinkKeywordRel struct {
	ID         int64 `bun:",pk,autoincrement"`
	LinkId     int64
	Visibility int
	Text       string
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
}

type PerceptualHash struct {
	ID             int64 `bun:",pk,autoincrement"`
	ContentId      int64
	AverageHash    uint64
	DifferenceHash uint64
	PerceptionHash uint64
}

type ExifInfo struct {
	ID        int64 `bun:",pk,autoincrement"`
	ContentId int64
	Camera    string
	Timestamp int64 // as UnixMicro
	Lat       float64
	Long      float64
}

type Errors struct {
	ID             int64 `bun:",pk,autoincrement"`
	Time           time.Time
	Url            string
	HttpStatusCode int
	// ParsingError            bool
	// ResponseToBig           bool
	// ErrorReading            bool
	// ResponseSizeUneqContLen bool
	ErrorCode ErrorCode
	ErrorText string
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

const (
	createNewDb  = false
	maxFilesize  = 2e8
	maxNumOfUrls = 1e7 // an estimate of how many urls we want to index
)

var sitesIndexed int
var db *bun.DB
var rdb *redis.Client
var contentTypeToIdCache sync.Map
var knownDomains sync.Map
var goodDomains sync.Map
var knownUrlsFilter *bloom.BloomFilter

func main() {
	// create a channel to receive certain syscalls
	sigChan := make(chan os.Signal, 1)

	// parse command line arguments
	numOfRoutines := flag.Int("n", 3, "Number of crawlers to run in parallel")
	flag.Parse()

	// get database clients
	rdb = getRedisClient()
	db = getDb()

	// create a new bloom filter
	knownUrlsFilter = bloom.NewWithEstimates(maxNumOfUrls, 0.01)
	// add urls from database to filter
	initBloom(db, knownUrlsFilter)

	// create channels
	linksChan := make(chan *Link, 1e3) // extractFromPage -> saveNewLink
	newUrls := make(chan *Link, 1e3)   // saveNewLink -> handleQueue

	// load the list of flaggedWords
	flaggedWords := loadFlaggedWords()

	// add to startSites
	addStartSites(newUrls)

	// handle SIGTERM
	go handleSigTerm(sigChan)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// start goroutines
	for i := 1; i <= *numOfRoutines; i++ {
		go addToQueue(newUrls)
		go addToQueue(newUrls)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		go extractFromPage(linksChan, 90)
		go extractFromPage(linksChan, 80)
		go extractFromPage(linksChan, 70)
		go extractFromPage(linksChan, 60)
		go extractFromPage(linksChan, 50)
		go extractFromPage(linksChan, 50)
		go extractFromPage(linksChan, 50)
		go extractFromPage(linksChan, 50)
	}

	// print a status update every two seconds
	for {
		time.Sleep(5 * time.Second)

		log.Printf("Visited %v links", sitesIndexed)
	}
}

func initBloom(db *bun.DB, filter *bloom.BloomFilter) {
	sites := make([]Site, 0, 4096)
	// urls := new([]Site)
	err := db.NewSelect().Model(&Site{}).Scan(context.Background(), &sites)
	handleBunSqlErr(err)
	log.Printf("Adding %v urls to the bloom filter", len(sites))

	for i := range sites {
		filter.Add([]byte(sites[i].Url))
	}
}

func handleSigTerm(sig chan os.Signal) {
	received := <-sig
	log.Printf("Received signal %v", received)

	time.Sleep(2 * time.Second)

	log.Printf("Closing database...")
	err := db.Close()
	if err != nil {
		log.Println("Error closing database")
	}
	err = rdb.Close()
	if err != nil {
		log.Println("Error closing redis database")
	}
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
