package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/boltdb/bolt"
)

type FS struct {
	db *bolt.DB
	// root     map[string]json.RawMessage
	metadata []string
	// binStore string
	caches []cacher
	stores []cacher
}

func newFS(mountpoint string, cfg *Config) *FS {

	_ = os.Remove(cfg.Main.Boltdb)
	db, err := bolt.Open(cfg.Main.Boltdb, 0600, nil)
	if err != nil {
		log.Fatalln("can't open boltdb database at %s: %s\n", cfg.Main.Boltdb, err)
	}

	caches := []cacher{}
	for _, c := range cfg.Cache {
		fmt.Println("add cache", c.Mnt)
		caches = append(caches, &fsCache{
			root: c.Mnt,
			// expiration: c.Expirtation,
			dedupe: "dedupe",
		})
	}

	stores := []cacher{}
	for _, s := range cfg.Store {
		fmt.Println("add Store", s.URL)
		stores = append(stores, &httpCache{
			addr: s.URL,
			// expiration: s.Expirtation,
			dedupe: "dedupe",
		})
	}

	filesys := &FS{
		db:       db,
		metadata: []string{},
		caches:   caches,
		stores:   stores,
	}

	for _, ays := range cfg.Ays {
		log.Println("fetching md for", ays.ID)
		metadata, err := filesys.GetMetaData("dedupe", ays.ID)
		if err != nil {
			log.Fatalln("error during metadata fetching", err)
		}

		for i, line := range metadata {
			if strings.HasPrefix(line, mountpoint) {
				metadata[i] = strings.TrimPrefix(line, mountpoint)
			}
		}
		sort.StringSlice(metadata).Sort()
		filesys.metadata = append(filesys.metadata, metadata...)
	}

	return filesys
}

var _ = fs.FS(&FS{})

func (f *FS) Root() (fs.Node, error) {

	n := &dir{
		fs:   f,
		name: "/",
	}
	return n, nil
}

func (f *FS) GetMetaData(dedupe, id string) ([]string, error) {
	// first try to get from caches
	metadata, err := getMetaData(f.caches, time.Second, dedupe, id)
	if err == nil {
		return metadata, nil
	}

	// if not in caches try to get from stores
	metadata, err = getMetaData(f.stores, time.Second*10, dedupe, id)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func getMetaData(caches []cacher, timeout time.Duration, dedupe, id string) ([]string, error) {
	chRes := make(chan []string)
	chErr := make(chan error)
	cancels := make(chan struct{}, len(caches))
	running := 0

	defer func() {
		for _ = range caches {
			cancels <- struct{}{}
		}
	}()

	for _, cache := range caches {
		go func(cache cacher, out chan []string, chErr chan error) {
			running++
			defer func() { running-- }()

			content, err := cache.GetMetaData(dedupe, id)
			if err != nil {
				chErr <- err
			}

			select {
			case <-cancels:
				//if we can read from cancels, the file has been found
				//by another goroutine
				return
			default:
				// we are the first, send data
				out <- content
			}
		}(cache, chRes, chErr)
	}

	for {
		select {
		case content := <-chRes:
			if content == nil {
				return nil, fuse.ENOENT
			}
			return content, nil

		case <-chErr:
			if running <= 0 {
				return nil, fuse.ENOENT
			}

		case <-time.After(timeout):
			return nil, fuse.ENOENT
		}
	}
}
