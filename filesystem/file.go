package filesystem

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"path"
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
	dir    *dir
	info   *fileInfo
	reader io.ReadSeeker
	opener int

	mu     sync.Mutex
}

func (f *file) String() string {
	return path.Join(f.dir.String(), f.info.Filename)
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

func getFileContent(ctx context.Context, path string, caches []cache.Cache, timeout time.Duration) (io.ReadSeeker, error) {
	result := make(chan io.ReadSeeker)
	wait := make(chan int)

	var wg sync.WaitGroup
	wg.Add(len(caches))

	for _, c := range caches {
		go func(c cache.Cache, out chan io.ReadSeeker) {
			log.Debug("Loading file from cache '%v' / '%v'", c, path)
			r, err := c.GetFileContent(path)
			if err == nil {
				select{
				case out <- r:
				default:
					f, ok := r.(io.ReadCloser)
					if ok {
						log.Debug("Closing unused file")
						f.Close()
					}
				}
			}
			wg.Done()
		}(c, result)
	}

	go func() {
		wg.Wait()
		wait <- 1
	}()

	select {
	case r := <-result:
		if r == nil {
			return nil, fuse.ENOENT
		}
		return r, nil
	case <-wait:
		//all exited with no response.
		return nil, fuse.ENOENT
	case <-time.After(timeout):
		return nil, fuse.ENOENT

	case <-ctx.Done():
		return nil, fuse.EINTR
	}
}

func (f *file) loadFromCache(ctx context.Context, fn func(io.ReadSeeker) error) error {
	r, err := getFileContent(ctx, f.binPath(), f.dir.fs.caches, time.Second*1)
	if err == nil {
		return fn(r)
	}

	r, err = getFileContent(ctx, f.binPath(), f.dir.fs.stores, time.Second*10)
	if err != nil {
		return err

	}

	return fn(r)
}

func (f *file) saveLocal() error {
	path := filepath.Join(f.dir.fs.local.BasePath(), "files", f.binPath())
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(path), 0660)

		outFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		defer outFile.Close()
		if err != nil {
			log.Error("error while saving %s into local cache. open error %s\n", f.info.Filename, err)
			os.Remove(path)
		}

		// move to begining of the file to be sure to copy all the data
		_, err = f.reader.Seek(0, 0)
		if err != nil {
			log.Error("error while saving %s into local cache. seek error: %s\n", f.info.Filename, err)
		}
		_, err = io.Copy(outFile, f.reader)
		if err != nil {
			log.Error("error while saving %s into local cache. copy error %s\n", f.info.Filename, err)
			os.Remove(path)
			return err
		}
	}
	return nil
}

var _ = fs.Node(&file{})

var _ = fs.Handle(&file{})

func (f *file) Attr(ctx context.Context, a *fuse.Attr) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	a.Mode = 0554
	a.Size = uint64(f.info.Size)
	return nil
}

var _ = fs.NodeOpener(&file{})

func (f *file) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	log.Debug("Opening file '%v' for reading", f)
	f.mu.Lock()
	defer f.mu.Unlock()

	resp.Flags = fuse.OpenKeepCache | fuse.OpenNonSeekable

	if f.opener > 0 {
		return f, nil
	}

	handleOpen := func(r io.ReadSeeker) error {
		f.reader = r
		f.opener++
		return nil
	}

	// then try to get from grid caches
	err := f.loadFromCache(ctx, handleOpen)
	if err == nil {
		return f, nil
	}

	return nil, fuse.ENOENT
}

var _ = fs.HandleReleaser(&file{})

func (f *file) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	log.Debug("Release file '%v'", f)
	f.mu.Lock()
	defer f.mu.Unlock()

	f.opener--
	if f.opener <= 0 {

		// save file into local cache
		go func() {
			if err := f.saveLocal(); err != nil {
				log.Error("can't save file %s into local cache: %v", f.info.Filename, err)
			}

			if r, ok := f.reader.(io.Closer); ok {
				r.Close()
			}
		}()
	}

	return nil
}

var _ = fs.HandleReader(&file{})

func (f *file) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	log.Debug("Read file '%v' '%v'", f, req)
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reader.Seek(req.Offset, 0)
	buff := make([]byte, req.Size)
	n, err := f.reader.Read(buff)

	resp.Data = buff[:n]

	if err != nil && err != io.EOF {
		log.Error("error read", err)
		return err
	}

	return nil
}
