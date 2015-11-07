package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	"bazil.org/fuse/fuseutil"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type FileInfo struct {
	Size     int
	Hash     string
	Filename string
}

type File struct {
	dir  *Dir
	info *FileInfo

	mu sync.Mutex

	file *os.File
}

func (f *File) binPath() string {
	return filepath.Join(f.dir.fs.binStore, f.info.Hash[:2], f.info.Hash)
}

var _ = fs.Node(&File{})

var _ = fs.Handle(&File{})

func (f *File) Attr(a *fuse.Attr) {
	f.mu.Lock()
	defer f.mu.Unlock()

	a.Mode = 0444
	a.Size = uint64(f.info.Size)
}

var _ = fs.NodeOpener(&File{})

func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var err error
	f.file, err = os.Open(f.binPath())
	if err != nil {
		log.Printf("can't open %s: %v\n", f.binPath(), err)
		return nil, err
	}

	return f, err
}

var _ = fs.HandleReleaser(&File{})

func (f *File) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.file != nil {
		f.file.Close()
		f.file = nil
	}

	return nil
}

var _ = fs.HandleReader(&File{})

func (f *File) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var err error

	if f.file == nil {
		f.file, err = os.Open(f.binPath())
		if err != nil {
			log.Printf("can't open %s: %v\n", f.binPath(), err)
			return err
		}
	}

	data, err := ioutil.ReadAll(f.file)
	if err != nil {
		log.Println("error during read of %v: %v", f.binPath(), err)
		return err
	}
	fuseutil.HandleRead(req, resp, data)

	return nil
}
