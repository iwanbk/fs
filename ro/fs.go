package ro

import (
	"bytes"
	"fmt"

	"github.com/op/go-logging"

	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/metadata"
	"sync"
)

var (
	log = logging.MustGetLogger("filesystem")
)

type FS struct {
	mountpoint string
	metadata   metadata.Metadata
	cache      cache.CacheManager

	factory NodeFactory

	m          sync.Mutex
	state      bool
	terminator chan int
	generator  chan int
}

func NewFS(mountpoint string, meta metadata.Metadata, cache cache.CacheManager) *FS {
	fs := &FS{
		mountpoint: mountpoint,
		metadata:   meta,
		cache:      cache,
		factory:    NewNodeFactory(),
		terminator: make(chan int),
		generator:  make(chan int),
	}

	//make sure to initially put factory in locked state.
	fs.factory.Lock()

	return fs
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

	return f.factory.GetDir(f, nil, f.metadata), nil
}

func (f *FS) Up() {
	defer f.factory.Unlock()

	f.m.Lock()
	defer f.m.Unlock()
	if !f.state {
		f.state = true
		go f.serve()
	}
}

func (f *FS) Down() {
	f.factory.Lock()

	f.m.Lock()
	defer f.m.Unlock()

	if f.state {
		f.state = false
		f.terminator <- 1
	}
}

func (f *FS) serve() {
loop:
	for {
		select {
		case <-f.terminator:
			break loop
		case f.generator <- 1:
		}
	}
}

//waits until the filesystem is accessible.
func (f *FS) access() {
	<-f.generator
}
