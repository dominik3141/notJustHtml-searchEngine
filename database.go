package main

import (
	"context"
	"database/sql"
	"log"

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
	rawDb, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("Error opening database. err=%v", err)
	}

	db := bun.NewDB(rawDb, sqlitedialect.New())

	// create new tables
	if createDbTables {
		_, err = db.NewCreateTable().Model(Link{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(GetErr{}).Exec(context.Background())
		check(err)
	}

	return db
}
