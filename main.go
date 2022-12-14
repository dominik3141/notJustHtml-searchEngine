package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/go-redis/redis"
	"github.com/uptrace/bun"
)

const (
	createNewDb  = false
	maxFilesize  = 2e7
	maxNumOfUrls = 1e7 // an estimate of how many urls we want to index
)

var (
	sitesIndexed         int
	db                   *bun.DB
	rdb                  *redis.Client
	contentTypeToIdCache sync.Map
	// knownDomains         sync.Map
	// goodDomains          sync.Map
	knownUrlsFilter *bloom.BloomFilter
	debugMode       bool
	useChromedp     *bool
)

// var domainIdCache = make(map[string]int64)

func main() {
	// create a channel to receive certain syscalls
	sigChan := make(chan os.Signal, 1)

	// parse command line arguments
	numOfRoutines := flag.Int("n", 3, "Number of crawlers to run in parallel")
	debugModeP := flag.Bool("debug", false, "Run program in debug mode")
	useChromedp = flag.Bool("chrome", false, "Use chrome to browse html")
	flag.Parse()
	debugMode = *debugModeP

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
	addStartSites()

	// handle SIGTERM
	go handleSigTerm(sigChan)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// start goroutines
	for i := 1; i <= *numOfRoutines; i++ {
		// go addToQueue(newUrls)
		go addToQueue(newUrls)
		// go saveNewLink(linksChan, newUrls, flaggedWords)
		go saveNewLink(linksChan, newUrls, flaggedWords)
		// go saveNewLink(linksChan, newUrls, flaggedWords)
		go extractFromPage(linksChan, 90)
		// go extractFromPage(linksChan, 90)
		// go extractFromPage(linksChan, 90)
		// go extractFromPage(linksChan, 90)
		// go extractFromPage(linksChan, 70)
		// go extractFromPage(linksChan, 70)
		// go extractFromPage(linksChan, 70)
		// go extractFromPage(linksChan, 60)
		// go extractFromPage(linksChan, 60)
		// go extractFromPage(linksChan, 50)
		// go extractFromPage(linksChan, 50)
	}

	// print a status update every two seconds
	for {
		time.Sleep(5 * time.Second)

		log.Printf("Found %v links", sitesIndexed)
	}
}

func initBloom(db *bun.DB, filter *bloom.BloomFilter) {
	sites := make([]Site, 0, 4096)
	err := db.NewSelect().Model(&Site{}).Scan(context.Background(), &sites)
	handleBunSqlErr(err)
	log.Printf("Adding %v urls to the bloom filter", len(sites))

	for i := range sites {
		filter.Add([]byte(sites[i].Url))
	}

	log.Printf("Done adding urls to the bloom filter")
}

func handleSigTerm(sig chan os.Signal) {
	received := <-sig
	log.Printf("Received signal %v", received)

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
