package filesystem

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/metadata"
	"path"
)

type File interface {
	fs.Node
	fs.Handle
	fs.HandleReleaser
	fs.HandleReader
	Parent() Dir
}

type fileImpl struct {
	parent Dir
	info   metadata.Leaf
	reader io.ReadSeeker
	opener int

	mu     sync.Mutex
}

func NewFile(parent Dir, leaf metadata.Leaf) File {
	return &fileImpl{
		parent: parent,
		info: leaf,
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
						log.Debug("Closing unused file '%s' from cache '%v'", path, c)
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

func (f *fileImpl) loadFromCache(ctx context.Context, fn func(io.ReadSeeker) error) error {
	r, err := getFileContent(ctx, f.binPath(), f.parent.FS().caches, time.Second*1)
	if err == nil {
		return fn(r)
	}

	r, err = getFileContent(ctx, f.binPath(), f.parent.FS().stores, time.Second*10)
	if err != nil {
		return err

	}

	return fn(r)
}

func (f *fileImpl) saveLocal() error {
	path := filepath.Join(f.parent.FS().local.BasePath(), "files", f.binPath())
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(path), 0660)

		outFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		defer outFile.Close()
		if err != nil {
			log.Error("error while saving %s into local cache. open error %s\n", f.info.Name(), err)
			os.Remove(path)
		}

		// move to begining of the file to be sure to copy all the data
		_, err = f.reader.Seek(0, 0)
		if err != nil {
			log.Error("error while saving %s into local cache. seek error: %s\n", f.info.Name(), err)
		}
		_, err = io.Copy(outFile, f.reader)
		if err != nil {
			log.Error("error while saving %s into local cache. copy error %s\n", f.info.Name(), err)
			os.Remove(path)
			return err
		}
	}
	return nil
}

var _ = fs.Node(&fileImpl{})

var _ = fs.Handle(&fileImpl{})

func (f *fileImpl) Attr(ctx context.Context, a *fuse.Attr) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	a.Mode = 0554
	a.Size = uint64(f.info.Size())
	return nil
}

var _ = fs.NodeOpener(&fileImpl{})

func (f *fileImpl) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	log.Debug("Opening file '%v' for reading", f)
	f.mu.Lock()
	defer f.mu.Unlock()

	resp.Flags = fuse.OpenKeepCache | fuse.OpenNonSeekable

	if f.opener > 0 {
		f.opener++
		return f, nil
	}

	handleOpen := func(r io.ReadSeeker) error {
		f.reader = r
		f.opener = 1
		return nil
	}

	// then try to get from grid caches
	err := f.loadFromCache(ctx, handleOpen)
	if err == nil {
		return f, nil
	}

	return nil, fuse.ENOENT
}

func (f *fileImpl) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	log.Debug("Release file '%v'", f)
	f.mu.Lock()
	defer f.mu.Unlock()

	f.opener--
	if f.opener <= 0 {

		// save file into local cache
		go func() {
			if err := f.saveLocal(); err != nil {
				log.Error("Can't save file %s into local cache: %v", f, err)
			}

			if r, ok := f.reader.(io.Closer); ok {
				log.Debug("Closing file '%s'", f)
				r.Close()
			}
		}()
	}

	return nil
}

func (f *fileImpl) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reader.Seek(req.Offset, 0)
	buff := make([]byte, req.Size)
	n, err := f.reader.Read(buff)
	resp.Data = buff[:n]

	if err != nil && err != io.EOF {
		log.Error("Error read", err)
		return err
	}

	return nil
}
