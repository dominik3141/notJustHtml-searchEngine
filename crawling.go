package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/bits-and-blooms/bloom/v3"
)

func addToQueue(queueIn chan *Link, flaggedWords *[]FlaggedWord) {
	filter := bloom.NewWithEstimates(maxNumOfUrls, 0.01)
	knownDomains := make(map[string]bool)
	// boringDomains := *loadBoringDomainsList()
	interestingDomains := *loadInterestingDomainsList()
	var err error
	var queueName string
	var priority int

	calcUrlPriority := func(link *Link) int {
		url := link.DestUrl

		// check if too popular
		// if boringDomains[url.Hostname()] {
		// 	return 0
		// }

		// identify ending
		urlStr := strings.ToLower(url.String())
		if strings.HasSuffix(urlStr, ".mp4") || strings.HasSuffix(urlStr, ".jpg") || strings.HasSuffix(urlStr, ".png") || strings.HasSuffix(urlStr, ".csv") || strings.HasSuffix(urlStr, ".pdf") || strings.HasSuffix(urlStr, ".tex") {
			return 80
		} else if strings.HasSuffix(urlStr, ".exe") || strings.HasSuffix(urlStr, ".apk") || strings.HasSuffix(urlStr, ".jar") || strings.HasSuffix(urlStr, ".msi") || strings.HasSuffix(urlStr, ".doc") {
			return 100
		}

		// check if url leads to an interesting domain
		if interestingDomains[url.Hostname()] {
			return 50
		}

		// check for keywords
		if link.Keywords != nil {
			rating := checkForFlaggedWords(flaggedWords, link.Keywords)
			if rating != 0 {
				log.Printf("URL: %v. Rating: %v", url.String(), rating)
			}
			if rating >= 10 {
				return 70
			}
		}

		// check if domain has been discovered before
		if !knownDomains[url.Hostname()] {
			log.Printf("Found new site: %v. Known sites: %v,", url.Hostname(), len(knownDomains))
			knownDomains[url.Hostname()] = true
			return 30
		}

		return 20
	}

	for {
		link := <-queueIn
		url := link.DestUrl

		if filter.Test([]byte(url.String())) {
			continue
		}

		sitesIndexed++

		// decide to which queue the link should be added
		priority = calcUrlPriority(link)
		// log.Printf("URL: %v. Priority: %v", link.DestUrl, priority)
		if priority == 0 {
			continue
		}
		queueName = fmt.Sprintf("QueuePriority%v", priority)
		err = rdb.SAdd(queueName, url.String()).Err()
		checkRedisErr(err)

		filter.Add([]byte(url.String()))
	}
}

type FlaggedWord struct {
	Priority int // higher means more important
	Word     string
}

func checkForFlaggedWords(flaggedWords *[]FlaggedWord, keywords *[]HtmlText) float64 {
	var rating float64

	for i := range *keywords {
		for j := range *flaggedWords {
			if strings.Contains((*keywords)[i].Text, (*flaggedWords)[j].Word) {
				rating += float64((*keywords)[i].Visibility) * float64(10*(*flaggedWords)[j].Priority)
			}
		}
	}

	return rating
}

func loadFlaggedWords() *[]FlaggedWord {
	const filename = "flaggedWords.csv"

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
	const filename = "links.txt"

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

func loadInterestingDomainsList() *map[string]bool {
	const filename = "interestingDomains.txt"
	interestingDomains := make(map[string]bool)

	f, err := os.Open(filename)
	check(err)
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		interestingDomains[scanner.Text()] = true
	}

	check(scanner.Err())

	return &interestingDomains
}

func loadBoringDomainsList() *map[string]bool {
	const filename = "top-1000-websites.txt"
	boringDomains := make(map[string]bool)

	f, err := os.Open(filename)
	check(err)
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		boringDomains[scanner.Text()] = true
	}

	check(scanner.Err())

	return &boringDomains
}
