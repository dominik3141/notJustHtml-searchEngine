package main

import (
	"log"
	"time"
)

type Link struct {
	TimeFound time.Time
	OrigUrl   string
	DestUrl   string
	LinkText  string
}

type GetErr struct {
	Time           time.Time
	Url            string
	ParsingError   bool // shall be true if the error has been a parsing error
	HttpStatusCode int  // shall be zero if the error has been a parsing error
}

const createDbTables = true

func main() {
	const testUrl = "https://heise.de"

	rdb := getRedisClient()
	db := getDb("testDb001.sqlite")
	linksChan := make(chan Link, 10)

	linksChan <- Link{DestUrl: testUrl}

	for i := 1; i < 2; i++ {
		go handleNewPage(linksChan, db, rdb)
	}

	for {
		time.Sleep(2 * time.Second)

		visited, err := rdb.SCard("visitedLinks").Result()
		check(err)
		log.Printf("Visited %v links", visited)
	}
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
