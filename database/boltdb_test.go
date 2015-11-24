package database

import (
	"log"
	"os"
	"testing"

	"github.com/boltdb/bolt"
)

var db *bolt.DB

func TestMain(m *testing.M) {
	var err error
	db, err = bolt.Open("test-aysfs.db", 0600, nil)
	if err != nil {
		log.Fatalln(err)
	}

	os.Exit(m.Run())

}
func BenchmarkLoadFromBolt(b *testing.B) {

	if err := StoreInBolt(db, []byte("test"), []byte("hello world")); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buff, err := LoadFromBolt(db, []byte("test"))
			if err != nil {
				b.Fatal(err)
			}
			_ = buff
		}
	})

}
