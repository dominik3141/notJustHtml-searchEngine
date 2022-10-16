package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"database/sql"
	"log"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/go-redis/redis"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

type HtmlText struct {
	Visibility int
	Text       string
}

type Link struct {
	TimeFound time.Time
	OrigUrl   *url.URL
	DestUrl   *url.URL
	Keywords  *[]HtmlText
	Rating    float64
	Priority  int
}

type LinkRel struct {
	ID          int64 `bun:",pk,autoincrement"`
	TimeFound   int64
	Origin      int64
	Destination int64
	Rating      float64
}

type LinkKeywordRel struct {
	ID         int64 `bun:",pk,autoincrement"`
	LinkId     int64
	Visibility int
	Text       string
}

type Site struct {
	ID  int64 `bun:",pk,autoincrement"`
	Url string
}

type Content struct {
	ID             int64 `bun:",pk,autoincrement"`
	TimeFound      int64
	SiteID         int64
	ContentTypeId  int64
	HttpStatusCode int
	Size           int
	Sha512Sum      *[sha512.Size]byte
	Sha1Sum        *[sha1.Size]byte
}

type PerceptualHash struct {
	ID             int64 `bun:",pk,autoincrement"`
	ContentId      int64
	AverageHash    uint64
	DifferenceHash uint64
	PerceptionHash uint64
}

type ExifInfo struct {
	ID        int64 `bun:",pk,autoincrement"`
	ContentId int64
	Camera    string
	Timestamp int64 // as UnixMicro
	Lat       float64
	Long      float64
}

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
