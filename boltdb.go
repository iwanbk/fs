package main

import (
	"bytes"
	"errors"
	"io"

	"github.com/boltdb/bolt"
)

var errNotFound = errors.New("not fount in boltdb")

func storeInBolt(db *bolt.DB, key []byte, content []byte) error {
	return db.Update(func(tx *bolt.Tx) error {

		bucket, err := tx.CreateBucketIfNotExists([]byte("main"))
		if err != nil {
			return err
		}

		return bucket.Put(key, content)
	})
}

func loadFromBolt(db *bolt.DB, key []byte) ([]byte, error) {
	result := []byte{}

	get := func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("main"))
		if bucket == nil {
			// log.Fatalln(err)
			return errNotFound
		}

		result = bucket.Get(key)
		if result == nil || len(result) == 0 {
			return errNotFound
		}

		return nil
	}

	err := db.View(get)
	return result, err
}

func lazyLoadFromBolt(db *bolt.DB, key []byte) (buff io.ReadSeeker, err error) {

	get := func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("main"))
		if bucket == nil {
			// log.Fatalln(err)
			return errNotFound
		}

		b := bucket.Get(key)
		if b == nil || len(b) == 0 {
			return errNotFound
		}
		buff = bytes.NewReader(b)

		return nil
	}

	err = db.View(get)
	return buff, err
}
