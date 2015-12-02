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
}

func NewFS(mountpoint string, cache cache.CacheManager) *FS {

	meta, _ := metadata.NewMetadata(mountpoint, nil)

	return &FS{
		mountpoint: mountpoint,
		metadata:   meta,
		cache:      cache,
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
		f.metadata.Index(line)
	}

	return nil
}

func (f *FS) PurgeMetadata() {
	meta, _ := metadata.NewMetadata(f.mountpoint, nil)
	f.metadata = meta
}

var _ = fs.FS(&FS{})

func (f *FS) Root() (fs.Node, error) {
	log.Debug("Accessing filesystem root")

	return NewDir(f, nil, f.metadata), nil
}
