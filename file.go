package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bazil.org/fuse/fuseutil"

	"github.com/boltdb/bolt"
	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type fileInfo struct {
	Size     int64
	Hash     string
	Filename string
}

//newFileInfo creates a new fileInfo struct by parsing the string s
//format of s shoud be 'filename|hash|size'
func newFileInfo(s string) (*fileInfo, error) {
	ss := strings.Split(s, "|")
	if len(ss) != 3 {
		return nil, errors.New("Bad format of file info")
	}

	size, err := strconv.ParseInt(ss[2], 10, 64)
	if err != nil {
		return nil, err
	}
	return &fileInfo{
		Filename: ss[0],
		Hash:     ss[1],
		Size:     size,
	}, nil
}

type file struct {
	dir  *dir
	info *fileInfo
	data []byte

	opener int

	mu sync.Mutex
}

func (f *file) dbKey() []byte {
	return []byte(fmt.Sprintf("file:%s", f.info.Hash))
}

func (f *file) binPath() string {
	return filepath.Join(string(f.info.Hash[0]), string(f.info.Hash[1]), f.info.Hash)
}

func (f *file) path() string {
	return filepath.Join(f.dir.Abs(), f.info.Filename)
}

func getFileContent(caches []cacher, timeout time.Duration, path string) ([]byte, error) {
	chRes := make(chan []byte)
	chErr := make(chan error)
	cancels := make(chan struct{}, len(caches))
	running := 0

	defer func() {
		for _ = range caches {
			cancels <- struct{}{}
		}
	}()

	for _, cache := range caches {
		go func(cache cacher, out chan []byte, chErr chan error) {
			running++
			defer func() { running-- }()

			content, err := cache.GetFileContent(path)
			if err != nil {
				chErr <- err
				return
			}

			select {
			case <-cancels:
				//if we can read from cancels, the file has been found
				//by another goroutine
				return
			default:
				// we are the first, send data
				out <- content
			}
		}(cache, chRes, chErr)
	}

	for {
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
	}
}

func (f *file) load() ([]byte, error) {
	content := make([]byte, f.info.Size)

	err := f.dir.fs.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("main"))
		if bucket == nil {
			return fuse.ENOENT
		}
		content = bucket.Get(f.dbKey())
		if content == nil {
			return fuse.ENOENT
		}

		return nil
	})

	return content, err
}

func (f *file) loadGridCache(path string) ([]byte, error) {
	return getFileContent(f.dir.fs.caches, time.Second, path)
}

func (f *file) loadStore(path string) ([]byte, error) {
	return getFileContent(f.dir.fs.stores, time.Second*10, path)
}

func (f *file) store(content []byte) error {
	err := f.dir.fs.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("main"))
		if err != nil {
			log.Println("error create bucket", err)
			return err
		}

		if err := bucket.Put(f.dbKey(), content); err != nil {
			log.Println("error during put dir into db", err)
			return err
		}

		return nil
	})
	if err != nil {
		log.Printf("error during populating cache for %s :%v\n", f.binPath(), err)
	}
	return err
}

var _ = fs.Node(&file{})

var _ = fs.Handle(&file{})

func (f *file) Attr(a *fuse.Attr) {
	f.mu.Lock()
	defer f.mu.Unlock()

	a.Mode = 0554
	a.Size = uint64(f.info.Size)
}

var _ = fs.NodeOpener(&file{})

func (f *file) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.data != nil {
		return f, nil
	}

	var err error
	content := make([]byte, f.info.Size)

	defer func(conten []byte) {
		f.opener++
		if err == nil && f.data != nil {
			go f.store(f.data)
		}
	}(content)

	content, err = f.load()
	if err == nil {
		f.data = content
		return f, nil
	}

	// first try to get from caches
	content, err = f.loadGridCache(f.binPath())
	if err == nil {
		f.data = content
		return f, nil
	}

	// if not in caches try to get from stores
	content, err = f.loadStore(f.binPath())
	if err != nil {
		return nil, err
	}

	f.data = content
	return f, nil
}

var _ = fs.HandleReleaser(&file{})

func (f *file) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.opener--
	if f.data != nil && f.opener <= 0 {
		f.data = nil
	}

	return nil
}

var _ = fs.HandleReader(&file{})

func (f *file) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.data != nil {
		fuseutil.HandleRead(req, resp, f.data)
		return nil
	}

	var err error
	content := make([]byte, f.info.Size)

	defer func(conten []byte) {
		if err == nil {
			f.data = content
			go f.store(f.data)
			fuseutil.HandleRead(req, resp, f.data)
		}
	}(content)

	content, err = f.load()
	if err == nil {
		return nil
	}

	// first try to get from caches
	content, err = f.loadGridCache(f.binPath())
	if err == nil {
		return nil
	}

	// if not in caches try to get from stores
	content, err = f.loadStore(f.binPath())
	if err != nil {
		return err
	}

	return nil
}
