package filesystem

import (
	"bytes"
	"fmt"

	"github.com/op/go-logging"

	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/metadata"
)

var (
	log = logging.MustGetLogger("filesystem")
)

type FS struct {
	mountpoint string
	metadata   metadata.Metadata
	cache      cache.CacheManager

	factory    FileFactory
}

func NewFS(mountpoint string, meta metadata.Metadata, cache cache.CacheManager) *FS {
	return &FS{
		mountpoint: mountpoint,
		metadata:   meta,
		cache:      cache,
		factory:    NewFileFactory(),
	}
}

func (f *FS) String() string {
	buffer := &bytes.Buffer{}
	fmt.Fprintf(buffer, "Caches:\n")
	for _, c := range f.cache.Layers() {
		fmt.Fprintf(buffer, "\t%s\n", c)
	}
	return buffer.String()
}

func (f *FS) AttachFList(ID string) error {
	partialMetadata, err := f.cache.GetMetaData(ID)
	if err != nil {
		return err
	}

	for _, line := range partialMetadata {
		err := f.metadata.Index(line)
		if err != nil {
			log.Error("Failed to index: %s", err)
		}
	}

	return nil
}

func (f *FS) PurgeMetadata() error {
	err := f.metadata.Purge()
	if err != nil {
		log.Error("Error while purging metadata :%s", err)
	}
	return err
}

var _ = fs.FS(&FS{})

func (f *FS) Root() (fs.Node, error) {
	log.Debug("Accessing filesystem root")

	return NewDir(f, nil, f.metadata), nil
}
