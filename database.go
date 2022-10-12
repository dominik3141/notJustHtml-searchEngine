package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/go-redis/redis"
	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func getRedisClient() *redis.Client {
	rdb := redis.NewClient(&redis.Options{})

	return rdb
}

func getDb(dbPath string) *bun.DB {
	var err error

	if createNewDb {
		f, err := os.Create(dbPath)
		check(err)
		f.Close()
	}

	rawDb, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("Error opening database. err=%v", err)
		panic(err)
	} else if rawDb == nil {
		log.Printf("Error opening database.")
		panic(err)
	}

	db := bun.NewDB(rawDb, sqlitedialect.New())

	// create new tables
	if createNewDb {
		_, err = db.NewCreateTable().Model(&LinkRel{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&Errors{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&Content{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&Site{}).Exec(context.Background())
		check(err)
	}

	return db
}
