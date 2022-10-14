package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/bits-and-blooms/bloom/v3"
)

func saveNewLink(inChan <-chan *Link, outChan chan<- *Link, flaggedWords *[]FlaggedWord) {
	var rating float64
	calcLinkPriority := func(link *Link) int {
		url := link.DestUrl
		urlStr := strings.ToLower(url.String())

		// prioritize links which lead to a potentially malicious file
		if strings.HasSuffix(urlStr, ".exe") || strings.HasSuffix(urlStr, ".apk") || strings.HasSuffix(urlStr, ".jar") || strings.HasSuffix(urlStr, ".msi") || strings.HasSuffix(urlStr, ".doc") {
			return 100
		}

		// check if domain has been discovered before
		_, isKnown := knownDomains.LoadOrStore(url.Hostname(), true)
		if !isKnown {
			return 30
		}

		return 20
	}

	for {
		// get the next link from the channel
		link := <-inChan

		// create a link relation from our Link struct
		linkRel := &LinkRel{
			TimeFound:   link.TimeFound.UnixMicro(),
			Origin:      getSiteID(link.OrigUrl.String()),
			Destination: getSiteID(link.DestUrl.String()),
			Keywords:    *link.Keywords,
		}

		// Use the keywords associated with the link to calculate an importance rating
		if link.Keywords != nil {
			rating = calcLinkRating(flaggedWords, link.Keywords)
			link.Rating = rating
			linkRel.Rating = rating
		}

		// calculate link priority for the queue handler
		link.Priority = calcLinkPriority(link)

		// overwrite link priority if link rating is high enough
		if link.Rating > 1 && link.Priority < 100 {
			link.Priority = 70
		}

		// add link to database
		dbMutex.Lock()
		_, err := db.NewInsert().Model(linkRel).Exec(context.Background())
		handleBunSqlErr(err)
		dbMutex.Unlock()

		// send link to the queue handler
		outChan <- link
	}
}

// add a link to a queue depending on the links rating and priority
func addToQueue(queueIn chan *Link) {
	filter := bloom.NewWithEstimates(maxNumOfUrls, 0.01)
	var err error
	var queueName string

	for {
		// get the next url from the channel
		link := <-queueIn

		// check if the url has been indexed before
		if filter.Test([]byte(link.DestUrl.String())) {
			continue
		}

		// add link to queue
		queueName = fmt.Sprintf("QueuePriority%v", link.Priority)
		err = rdb.SAdd(queueName, link.DestUrl.String()).Err()
		checkRedisErr(err)

		// increase the counter for indexed sites
		sitesIndexed++

		// add url to the bloom filter of known urls
		filter.Add([]byte(link.DestUrl.String()))
	}
}

type FlaggedWord struct {
	Priority int // higher means more important
	Word     string
}

// calculate a rating for a given link based on how well its keywords match our flagged words
func calcLinkRating(flaggedWords *[]FlaggedWord, keywords *[]HtmlText) float64 {
	const flagPriorityVsKeywordVisibility = 2

	var rating float64

	for i := range *keywords {
		for j := range *flaggedWords {
			if strings.Contains((*keywords)[i].Text, (*flaggedWords)[j].Word) {
				rating += float64((*keywords)[i].Visibility) * float64(flagPriorityVsKeywordVisibility*(*flaggedWords)[j].Priority)
			}
		}
	}

	return rating
}

///////////////////////////////
// Load certain config files
///////////////////////////////

func loadFlaggedWords() *[]FlaggedWord {
	const filename = "config/flaggedWords.csv"

	flaggedWords := make([]FlaggedWord, 0)

	f, err := os.Open(filename)
	check(err)
	defer f.Close()

	reader := csv.NewReader(f)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			return &flaggedWords
		}
		check(err)
		prio, err := strconv.Atoi(record[1])
		check(err)
		flaggedWords = append(flaggedWords, FlaggedWord{Priority: prio, Word: strings.ToLower(record[0])})

	}
}

func addStartSites(out chan *Link) {
	const filename = "config/links.txt"

	f, err := os.Open(filename)
	check(err)
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		url, err := url.Parse(scanner.Text())
		check(err)
		out <- &Link{DestUrl: url}
	}

	check(scanner.Err())
}
