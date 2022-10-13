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

type ContentTypes struct {
	ID   int64 `bun:",pk,autoincrement"`
	Name string
}

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
		_, err = db.NewCreateTable().Model(&ContentTypes{}).Exec(context.Background())
		check(err)
	}

	return db
}

func getSiteID(url string) int64 {
	site := new(Site)
	var i int
start:

	id, err := rdb.HGet("SiteIDs", url).Int64()
	checkRedisErr(err)
	site.ID = id
	if id == 0 {
		lockDb.Lock()
		err = db.NewSelect().Model(site).Where("url = ?", url).Scan(context.Background(), site)
		lockDb.Unlock()
		if err != nil || site.ID == 0 {
			// create new site
			lockDb.Lock()
			_, err := db.NewInsert().Model(&Site{Url: url}).Exec(context.Background())
			handleSqliteErr(err)
			lockDb.Unlock()
			if i > 3 {
				panic("loop")
			}
			i++
			goto start
		}
		err = rdb.HSet("SiteIDs", url, site.ID).Err()
		checkRedisErr(err)
	}

	return site.ID
}

func getContentTypeId(contentTypeStr string) int64 {
	var i int
	var id int64
	var err error

start:
	val, found := contentTypeToIdCache.Load(contentTypeStr)
	if !found {
		lockDb.Lock()
		err = db.NewSelect().Model(&ContentTypes{}).Column("id").Where("Name = ?", contentTypeStr).Scan(context.Background(), &id)
		lockDb.Unlock()
		if err != nil || id == 0 {
			lockDb.Lock()
			_, err = db.NewInsert().Model(&ContentTypes{Name: contentTypeStr}).Exec(context.Background())
			handleSqliteErr(err)
			lockDb.Unlock()
			if i > 3 {
				panic("loop")
			}
			i++
			goto start
		}
		contentTypeToIdCache.Store(contentTypeStr, id)
	} else {
		id = val.(int64)
	}

	return id
}
