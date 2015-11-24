package cache

import (
	"github.com/boltdb/bolt"
	"github.com/Jumpscale/aysfs/database"
	"fmt"
	"io"
	"bytes"
)

type boltCache struct {
	db *bolt.DB
}

func NewBoldCache(db *bolt.DB) Cache {
	return &boltCache{
		db: db,
	}
}

func (f *boltCache) GetMetaData(dedupe, id string) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *boltCache) GetFileContent(path string) (io.ReadSeeker, error) {
	return lazyLoadFromBolt(f.db, []byte(path))
}

func (f *boltCache) Exists(path string) bool {
	return true
}

func lazyLoadFromBolt(db *bolt.DB, key []byte) (buff io.ReadSeeker, err error) {

	get := func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("main"))
		if bucket == nil {
			// log.Fatalln(err)
			return database.NotFound
		}

		b := bucket.Get(key)
		if b == nil || len(b) == 0 {
			return database.NotFound
		}
		buff = bytes.NewReader(b)

		return nil
	}

	err = db.View(get)
	return buff, err
}

func (f *boltCache) BasePath() string {
	return ""
}