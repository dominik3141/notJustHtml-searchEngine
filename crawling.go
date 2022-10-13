package main

import (
	"bufio"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/bits-and-blooms/bloom/v3"
)

func addToQueue(queueIn chan *url.URL) {
	filter := bloom.NewWithEstimates(maxNumOfUrls, 0.01)
	var err error
	var queueName string
	var priority int

	for {
		url := <-queueIn

		if filter.Test([]byte(url.String())) {
			continue
		}

		sitesIndexed++

		// decide to which queue the link should be added
		priority = calcUrlPriority(url)
		if priority == 0 {
			continue
		}
		queueName = fmt.Sprintf("QueuePriority%v", priority)
		err = rdb.SAdd(queueName, url.String()).Err()
		checkRedisErr(err)

		filter.Add([]byte(url.String()))
	}
}

func calcUrlPriority(url *url.URL) int {
	knownDomains := make(map[string]bool)
	boringDomains := *loadBoringDomainsList()
	interestingDomains := *loadInterestingDomainsList()

	// check if too popular
	if boringDomains[url.Hostname()] {
		return 0
	}

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

	// check if domain has been discovered before
	if !knownDomains[url.Hostname()] {
		log.Printf("Found new site: %v. Known sites:%v,", url.Hostname(), len(knownDomains))
		return 30
	}

	return 20
}

func addStartSites(out chan *url.URL) {
	const filename = "links.txt"

	f, err := os.Open(filename)
	check(err)
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		url, err := url.Parse(scanner.Text())
		check(err)
		out <- url
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
