package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

func saveNewLink(inChan <-chan *Link, outChan chan<- *Link, flaggedWords *[]FlaggedWord) {
	var rating float64

	calcLinkPriority := func(link *Link) int {
		url := link.DestUrl
		urlStr := strings.ToLower(url.String())

		if strings.HasSuffix(urlStr, ".png") || strings.HasSuffix(urlStr, ".jpg") || strings.HasSuffix(urlStr, ".jpeg") {
			return 90
		}

		return 0
	}

	for {
		// get the next link from the channel
		link := <-inChan

		// create a link relation from our Link struct
		linkRel := &LinkRel{
			TimeFound:   link.TimeFound.UnixMicro(),
			Origin:      getSiteID(link.OrigUrl),
			Destination: getSiteID(link.DestUrl),
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
		if link.Rating > 20 && link.Priority < 90 {
			link.Priority = 80
		}

		// add link to database
		_, err := db.NewInsert().Model(linkRel).Returning("id").Exec(context.Background())
		handleBunSqlErr(err)

		// save the keywords in the database
		for i := range *link.Keywords {
			keyword := &LinkKeywordRel{
				LinkId:     linkRel.ID,
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

func addStartSites() {
	const filename = "config/links.txt"

	f, err := os.Open(filename)
	check(err)
	defer f.Close()

	scanner := bufio.NewScanner(f)

	counter := 0
	for scanner.Scan() {
		counter++
		if debugMode {
			log.Println("Adding start side with url:", scanner.Text())
			log.Println("Urls added so far:", counter)
		}
		// add link to queue
		err = rdb.SAdd("QueuePriority90", scanner.Text()).Err()
		checkRedisErr(err)
	}

	check(scanner.Err())
}
