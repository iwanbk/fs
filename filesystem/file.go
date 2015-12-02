package filesystem

import (
	"io"
	"path/filepath"
	"sync"

	"golang.org/x/net/context"

	"path"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/metadata"
)

type File interface {
	fs.Node
	fs.NodeOpener
	Parent() Dir
	Read(seek int64, buffer []byte) (int, error)
	Release()
}

type fileImpl struct {
	parent Dir
	info   metadata.Leaf
	reader io.ReadSeeker
	opener int

	mu sync.Mutex
}

func NewFile(parent Dir, leaf metadata.Leaf) File {
	return &fileImpl{
		parent: parent,
		info:   leaf,
	}
}

func (f *fileImpl) Parent() Dir {
	return f.parent
}

func (f *fileImpl) String() string {
	return path.Join(f.parent.String(), f.info.Name())
}

func (f *fileImpl) binPath() string {
	hash := f.info.Hash()
	return filepath.Join(string(hash[0]), string(hash[1]), hash)
}

func (f *fileImpl) path() string {
	return filepath.Join(f.parent.String(), f.info.Name())
}

func (f *fileImpl) loadFromCache(ctx context.Context, fn func(io.ReadSeeker) error) error {
	file, err := f.parent.FS().cache.Open(f.binPath())
	if err != nil {
		return err
	}

	return fn(file)
}

func (f *fileImpl) Attr(ctx context.Context, a *fuse.Attr) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	a.Mode = 0554
	a.Size = uint64(f.info.Size())
	return nil
}

var _ = fs.NodeOpener(&fileImpl{})

func (f *fileImpl) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	log.Debug("Opening file '%s' for reading", f)
	f.mu.Lock()
	defer f.mu.Unlock()

	resp.Flags = fuse.OpenKeepCache | fuse.OpenNonSeekable

	if f.opener > 0 {
		f.opener++
		return NewFileBuffer(f), nil
	}

	handleOpen := func(r io.ReadSeeker) error {
		f.reader = r
		f.opener = 1
		return nil
	}

	// then try to get from grid caches
	err := f.loadFromCache(ctx, handleOpen)
	if err == nil {
		return NewFileBuffer(f), nil
	}

	return nil, fuse.ENOENT
}

func (f *fileImpl) Read(seek int64, buffer []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	_, err := f.reader.Seek(seek, 0)
	if err != nil {
		return 0, err
	}
	return f.reader.Read(buffer)
}

func (f *fileImpl) Release() {
	f.mu.Lock()
	f.mu.Unlock()
	log.Debug("Release file '%v'", f)

	f.opener--
	if f.opener <= 0 {
		// Closing the file. we do that inside a go routine so
		// cache manager can take it's time deduping this file to
		// other writtable caches.
		go func(reader io.ReadSeeker) {
			log.Debug("Closing file '%s'", f)
			if reader, ok := reader.(io.Closer); ok {
				reader.Close()
			}
		}(f.reader)

		f.reader = nil
	}
}
