package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"log"
	"net/url"

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
	ID       int64 `bun:",pk,autoincrement"`
	DomainId int64
	Url      string
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

type Domain struct {
	ID   int64  `bun:",pk,autoincrement"`
	Name string `bun:",unique"`
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
		_, err = db.NewCreateTable().Model(&Domain{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&Face{}).Exec(context.Background())
		check(err)
		_, err = db.NewCreateTable().Model(&DbFileEntry{}).Exec(context.Background())
		check(err)
	}

	return db
}

func getSiteID(urlP *url.URL) int64 {
	site := &Site{Url: urlP.String()}

	id, err := rdb.HGet("SiteIDs", urlP.String()).Int64()
	checkRedisErr(err)
	site.ID = id
	if id == 0 {
		err = db.NewSelect().Model(site).Where("url = ?", urlP).Scan(context.Background(), site)
		if err != nil || site.ID == 0 {
			// create new site
			site.Url = urlP.String()
			site.DomainId = getDomainId(urlP.Hostname())
			_, err := db.NewInsert().Model(site).Returning("id").Exec(context.Background())
			handleBunSqlErr(err)
		}
		if site.ID == 0 {
			panic("ERROR with siteId")
		}
		err = rdb.HSet("SiteIDs", urlP.String(), site.ID).Err()
		checkRedisErr(err)

		return site.ID
	}

	return site.ID
}

func getContentTypeId(contentTypeStr string) int64 {
	var id int64
	var err error

	val, found := contentTypeToIdCache.Load(contentTypeStr)
	if !found {
		err = db.NewSelect().Model(&ContentTypes{}).Column("id").Where("Name = ?", contentTypeStr).Scan(context.Background(), &id)
		if err != nil || id == 0 {
			cType := &ContentTypes{Name: contentTypeStr}
			_, err = db.NewInsert().Model(cType).Returning("id").Exec(context.Background())
			handleBunSqlErr(err)
			if cType.ID == 0 {
				panic("Error returning contentTypeId")
			}
			contentTypeToIdCache.Store(contentTypeStr, cType.ID)
			return cType.ID
		}
		contentTypeToIdCache.Store(contentTypeStr, id)
	} else {
		id = val.(int64)
	}

	return id
}

func getDomainId(domain string) int64 {
	var err error
	var id int64
	found := false
	if !found {
		err = db.NewSelect().Model(&Domain{}).Where("name = ?", domain).Column("id").Scan(context.Background(), &id)
		if err == nil {
		} else if err.Error() == "sql: no rows in result set" {
			newDomain := Domain{
				Name: domain,
			}
			_, err = db.NewInsert().Model(&newDomain).Returning("id").Exec(context.Background())
			check(err)
			id = newDomain.ID
		} else {
			panic(err)
		}
	}

	return id
}

type DbFileEntry struct {
	Sha1Sum *[sha1.Size]byte `bun:",pk"`
	Content *[]byte
}

// insert file into database if the file is not already inside
func saveFileToDatabase(sha1Sum *[sha1.Size]byte, file *[]byte) {
	dbFile := DbFileEntry{
		Sha1Sum: sha1Sum,
		Content: file,
	}

	result, err := db.NewInsert().Model(&dbFile).Ignore().Exec(context.Background())
	check(err)
	if debugMode {
		n, err := result.RowsAffected()
		check(err)
		log.Printf("Added %v files to database. Hash: %v", n, base64.URLEncoding.EncodeToString(sha1Sum[:]))
	}
}
