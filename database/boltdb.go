package database

import (
	"errors"

	"github.com/boltdb/bolt"
)

var NotFound = errors.New("not fount in boltdb")

func StoreInBolt(db *bolt.DB, key []byte, content []byte) error {
	return db.Update(func(tx *bolt.Tx) error {

		bucket, err := tx.CreateBucketIfNotExists([]byte("main"))
		if err != nil {
			return err
		}

		return bucket.Put(key, content)
	})
}

func LoadFromBolt(db *bolt.DB, key []byte) ([]byte, error) {
	result := []byte{}

	get := func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("main"))
		if bucket == nil {
			// log.Fatalln(err)
			return NotFound
		}

		result = bucket.Get(key)
		if result == nil || len(result) == 0 {
			return NotFound
		}

		return nil
	}

	err := db.View(get)
	return result, err
}
