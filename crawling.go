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
)

func saveNewLink(inChan <-chan *Link, outChan chan<- *Link, flaggedWords *[]FlaggedWord) {
	var rating float64
	var found bool

	calcLinkPriority := func(link *Link) int {
		url := link.DestUrl
		urlStr := strings.ToLower(url.String())

		// prioritize links which lead to a potentially malicious file
		if strings.HasSuffix(urlStr, ".exe") || strings.HasSuffix(urlStr, ".apk") || strings.HasSuffix(urlStr, ".jar") || strings.HasSuffix(urlStr, ".msi") || strings.HasSuffix(urlStr, ".doc") {
			return 100
			// }
		} else if strings.HasSuffix(urlStr, ".png") || strings.HasSuffix(urlStr, ".jpg") || strings.HasSuffix(urlStr, ".jpeg") {
			return 90
		}

		_, found = goodDomains.Load(link.DestUrl.Hostname())
		if found {
			return 70
		}

		// check if domain has been discovered before
		_, isKnown := knownDomains.LoadOrStore(url.Hostname(), true)
		if !isKnown {
			return 60
		}

		return 50
	}

	for {
		// get the next link from the channel
		link := <-inChan

		// create a link relation from our Link struct
		linkRel := &LinkRel{
			TimeFound:   link.TimeFound.UnixMicro(),
			Origin:      getSiteID(link.OrigUrl.String()),
			Destination: getSiteID(link.DestUrl.String()),
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
		if link.Rating > 20 && link.Priority < 100 {
			link.Priority = 80
		}

		// add link to database
		_, err := db.NewInsert().Model(linkRel).Returning("id").Exec(context.Background())
		handleBunSqlErr(err)

		// save the keywords in the database
		for i := range *link.Keywords {
			keyword := &LinkKeywordRel{
				LinkId: linkRel.ID,
				// SiteId:     getSiteID(link.DestUrl.String()),
				Visibility: (*link.Keywords)[i].Visibility,
				Text:       (*link.Keywords)[i].Text,
			}
			_, err = db.NewInsert().Model(keyword).Exec(context.Background())
			handleBunSqlErr(err)
		}

		// send link to the queue handler
		outChan <- link
	}
}

// add a link to a queue depending on the links rating and priority
func addToQueue(queueIn chan *Link) {
	var err error
	var queueName string

	for {
		// get the next url from the channel
		link := <-queueIn

		// check if the url has been indexed before, if not, add it to the filter
		if knownUrlsFilter.TestOrAdd([]byte(link.DestUrl.String())) {
			continue
		}

		if link.Priority == 0 {
			continue
		}

		// add link to queue
		queueName = fmt.Sprintf("QueuePriority%v", link.Priority)
		err = rdb.SAdd(queueName, link.DestUrl.String()).Err()
		checkRedisErr(err)

		// increase the counter for indexed sites
		sitesIndexed++
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
		out <- &Link{DestUrl: url, Priority: 60}
	}

	check(scanner.Err())
}
