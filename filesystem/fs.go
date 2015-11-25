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
	}
}

func (f *FS) AddCache(cache cache.Cache) {
	f.caches = append(f.caches, cache)
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

	n := &dir{
		fs:   f,
		name: "/",
	}
	return n, nil
}

func (f *FS) GetMetaData(dedupe string, id string) ([]string, error) {
	log.Debug("Getting metadata for '%s' from '%s' cache", id, dedupe)
	return getMetaData(f.caches, time.Second*10, dedupe, id)
}

func getMetaData(caches []cache.Cache, timeout time.Duration, dedupe, id string) ([]string, error) {
	result := make(chan []string)
	wait := make(chan int)

	var wg sync.WaitGroup
	wg.Add(len(caches))
	for _, c := range caches {
		go func(c cache.Cache, out chan []string) {
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
		wg.Wait()
		wait <- 1
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
