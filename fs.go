package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/boltdb/bolt"
	"github.com/Jumpscale/aysfs/cache"
)

type FS struct {
	db *bolt.DB
	// root     map[string]json.RawMessage
	metadata []string

	boltdb cache.Cache
	local  cache.Cache
	caches []cache.Cache
	stores []cache.Cache
}

func newFS(mountpoint string, cfg *Config) *FS {

	_ = os.Remove(cfg.Main.Boltdb)
	db, err := bolt.Open(cfg.Main.Boltdb, 0600, nil)
	if err != nil {
		log.Fatalln("can't open boltdb database at %s: %s\n", cfg.Main.Boltdb, err)
	}

	localRoot := filepath.Join(os.TempDir(), "aysfs_cahce")
	os.RemoveAll(localRoot)
	os.MkdirAll(localRoot, 0660)
	localCache := cache.NewFSCache(localRoot, "dedupe")

	caches := []cache.Cache{}
	for _, c := range cfg.Cache {
		fmt.Println("add cache", c.Mnt)
		caches = append(caches, cache.NewFSCache(c.Mnt, "dedupe"))
	}

	stores := []cache.Cache{}
	for _, s := range cfg.Store {
		fmt.Println("add Store", s.URL)
		stores = append(stores, cache.NewHTTPCache(s.URL, "dedupe"))
	}

	filesys := &FS{
		db:       db,
		metadata: []string{},

		boltdb: cache.NewBoldCache(db),
		local:  localCache,
		caches: caches,
		stores: stores,
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
	// try from local
	// metadata, err := f.local.GetMetaData(dedupe, id)
	// if err == nil {
	// 	return metadata, nil
	// }

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

func getMetaData(caches []cache.Cache, timeout time.Duration, dedupe, id string) ([]string, error) {
	chRes := make(chan []string)
	chErr := make(chan error)
	cancels := make(chan struct{}, len(caches))
	running := 0

	defer func() {
		for _ = range caches {
			cancels <- struct{}{}
		}
	}()

	for _, c := range caches {
		go func(c cache.Cache, out chan []string, chErr chan error) {
			running++
			defer func() { running-- }()

			content, err := c.GetMetaData(dedupe, id)
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
		}(c, chRes, chErr)
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
