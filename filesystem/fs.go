package filesystem

import (
	"github.com/op/go-logging"
	"os"
	"path/filepath"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/metadata"
	"sync"
)

var (
	log = logging.MustGetLogger("filesystem")
)

type FS struct {
	metadata metadata.Metadata
	local    cache.Cache
	caches   []cache.Cache
	stores   []cache.Cache
}

func NewFS(mountpoint string, cfg *config.Config) *FS {
	localRoot := filepath.Join(os.TempDir(), "aysfs_cahce")
	localCache := cache.NewFSCache(localRoot, "dedupe")
	localCache.Purge()

	meta, _ := metadata.NewMetadata(mountpoint, nil)

	return &FS{
		metadata: meta,
		local:  localCache,
		caches: []cache.Cache{localCache},
		stores: make([]cache.Cache, 0),
	}
}

func (f *FS) AddCache(cache cache.Cache) {
	f.caches = append(f.caches, cache)
}

func (f *FS) AddStore(store cache.Cache) {
	f.stores = append(f.stores, store)
}

func (f *FS) AttachFList(ID string) error {
	partialMetadata, err := f.GetMetaData("dedupe", ID)
	if err != nil {
		return err
	}

	for _, line := range partialMetadata {
		f.metadata.Index(line)
	}

	return nil
}

var _ = fs.FS(&FS{})

func (f *FS) Root() (fs.Node, error) {
	log.Debug("Accessing filesystem root")

	return NewDir(f, nil, f.metadata), nil
}

func (f *FS) GetMetaData(dedupe string, id string) ([]string, error) {
	log.Debug("Getting metadata for '%s' from '%s' cache", id, dedupe)
	result, err := getMetaData(f.caches, time.Second, dedupe, id)
	if err == nil {
		return result, nil
	}

	return getMetaData(f.stores, time.Second * 10, dedupe, id)
}

func getMetaData(caches []cache.Cache, timeout time.Duration, dedupe, id string) ([]string, error) {
	result := make(chan []string)
	defer close(result)
	wait := make(chan int)
	defer close(wait)

	var wg sync.WaitGroup
	wg.Add(len(caches))
	for _, c := range caches {
		go func(c cache.Cache, out chan []string) {
			defer func() { recover() }()
			log.Debug("Trying cache %v", c)
			content, err := c.GetMetaData(dedupe, id)
			if err == nil {
				select {
				case out <- content:
				default:
				}
			} else {
				log.Warning("Cache %v does not provide meta data: %s", c, err)
			}

			wg.Done()
		}(c, result)
	}

	go func() {
		defer func() { recover() }()
		wg.Wait()
		select{
		case wait <- 1:
		default:
		}
	}()

	log.Debug("Waiting for cache response")
	select {
	case content := <-result:
		if content == nil {
			return nil, fuse.ENOENT
		}
		return content, nil
	case <-wait:
		//all exited, but no output was provided.
		return nil, fuse.ENOENT
	case <-time.After(timeout):
		return nil, fuse.ENOENT
	}

	return nil, fuse.ENOENT
}
