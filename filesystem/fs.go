package filesystem

import (
	"fmt"
	"github.com/op/go-logging"
	"os"
	"path/filepath"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/boltdb/bolt"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/metadata"
)

var (
	log = logging.MustGetLogger("filesystem")
)

type FS struct {
	db *bolt.DB
	// root     map[string]json.RawMessage
	metadata metadata.Metadata

	local  cache.Cache
	caches []cache.Cache
	stores []cache.Cache
}

func NewFS(mountpoint string, cfg *config.Config) *FS {

	_ = os.Remove(cfg.Main.Boltdb)
	db, err := bolt.Open(cfg.Main.Boltdb, 0600, nil)
	if err != nil {
		log.Fatalf("can't open boltdb database at %s: %s\n", cfg.Main.Boltdb, err)
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
	meta, _ := metadata.NewMetadata(mountpoint, nil)

	filesys := &FS{
		db:       db,
		metadata: meta,
		local:  localCache,
		caches: caches,
		stores: stores,
	}

	for _, ays := range cfg.Ays {
		partialMetadata, err := filesys.GetMetaData("dedupe", ays.ID)
		if err != nil {
			log.Fatal("error during metadata fetching", err)
		}

		for _, line := range partialMetadata {
			meta.Index(line)
		}
	}

	return filesys
}

var _ = fs.FS(&FS{})

func (f *FS) Root() (fs.Node, error) {
	log.Debug("Accessing filesystem root")

	n := &dir{
		fs:   f,
		name: "/",
	}
	return n, nil
}

func (f *FS) GetMetaData(dedupe string, id string) ([]string, error) {
	// try from local
	// metadata, err := f.local.GetMetaData(dedupe, id)
	// if err == nil {
	// 	return metadata, nil
	// }

	// first try to get from caches
	log.Debug("Getting metadata for '%s' from '%s' cache", id, dedupe)
	metadata, err := getMetaData(f.caches, time.Second, dedupe, id)
	if err == nil {
		return metadata, nil
	}

	// if not in caches try to get from stores
	log.Debug("Getting metadata for '%s' from '%s' store", id, dedupe)
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
			log.Debug("Trying cache %v", c)
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
				log.Debug("Metadata found from cache %v", c)
				out <- content
			}
		}(c, chRes, chErr)
	}

	log.Debug("Waiting for cache response")
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

	return nil, fuse.ENOENT
}
