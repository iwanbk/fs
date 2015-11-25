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
	chRes := make(chan []string)
	cancels := make(chan int, len(caches))
	running := 0

	defer func() {
		for _ = range caches {
			cancels <- 1
		}
	}()

	for _, c := range caches {
		go func(c cache.Cache, out chan []string) {
			log.Debug("Trying cache %v", c)
			running++
			defer func() { running-- }()

			content, err := c.GetMetaData(dedupe, id)
			if err != nil {
				<-cancels
				return
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
		}(c, chRes)
	}

	log.Debug("Waiting for cache response")
	select {
	case content := <-chRes:
		if content == nil {
			return nil, fuse.ENOENT
		}
		return content, nil
	case <-time.After(timeout):
		return nil, fuse.ENOENT
	}

	return nil, fuse.ENOENT
}
