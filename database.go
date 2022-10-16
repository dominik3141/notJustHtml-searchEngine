package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path"

	"github.com/go-redis/redis"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

type ContentTypes struct {
	ID   int64 `bun:",pk,autoincrement"`
	Name string
}

func getRedisClient() *redis.Client {
	rdb := redis.NewClient(&redis.Options{})

	return rdb
}

func getDb() *bun.DB {
	var err error

	connStr := "user=crawleru dbname=crawler password=crawlerP"
	rawDb, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Error opening database. err=%v", err)
		panic(err)
	} else if rawDb == nil {
		log.Printf("Error opening database.")
		panic(err)
	}

	db := bun.NewDB(rawDb, pgdialect.New())

	// create new tables
	if createNewDb {
		_, err = db.NewCreateTable().Model(&LinkRel{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&Errors{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&Site{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&ContentTypes{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&Content{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&LinkKeywordRel{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&ExifInfo{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&PerceptualHash{}).Exec(context.Background())
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
		err = db.NewSelect().Model(site).Where("url = ?", url).Scan(context.Background(), site)
		if err != nil || site.ID == 0 {
			// create new site
			_, err := db.NewInsert().Model(&Site{Url: url}).Exec(context.Background())
			handleBunSqlErr(err)
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
		err = db.NewSelect().Model(&ContentTypes{}).Column("id").Where("Name = ?", contentTypeStr).Scan(context.Background(), &id)
		if err != nil || id == 0 {
			_, err = db.NewInsert().Model(&ContentTypes{Name: contentTypeStr}).Exec(context.Background())
			handleBunSqlErr(err)
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

func saveToFile(filename string, data *[]byte) {
	filename = path.Join("downloaded", filename)

	f, err := os.Create(filename)
	check(err)
	defer f.Close()

	_, err = f.Write(*data)
	check(err)
}
