package cache

import (
	"io"
	"path/filepath"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("cache")
)

type Cache interface {
	Open(path string) (io.ReadSeeker, error)
	GetMetaData(id string) ([]string, error)
	Exists(path string) bool
	BasePath() string
}

type CachePurger interface {
	Purge() error
}

type CacheWriter interface {
	SetMetaData([]string) error
	DeDupe(string, io.ReadSeeker) error
}

type CacheManager interface {
	Cache
	CachePurger
	AddLayer(Cache)
	Layers() []Cache
}

// Chroot return the absolute path of path but chrooted at root
func chroot(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Join(root, path[1:])
	}
	return filepath.Join(root, path)
}
