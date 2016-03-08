package files

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"github.com/dsnet/compress/brotli"
	"github.com/g8os/fs/crypto"
	"github.com/g8os/fs/rw/meta"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

// filesystem represents g8os filesystem
type fileSystem struct {
	// TODO - this should need default fill in.
	pathfs.FileSystem
	Root string
	*FS
}

// create filesystem object
func newFileSystem(fs *FS) pathfs.FileSystem {
	return &fileSystem{
		FileSystem: NewDefaultFileSystem(),
		Root:       fs.backend.Path,
		FS:         fs,
	}
}

func (fs *fileSystem) OnMount(nodeFs *pathfs.PathNodeFs) {
	log.Debug("OnMount")
}

func (fs *fileSystem) OnUnmount() {
	log.Debug("OnUnmount")
}

func (fs *fileSystem) GetPath(relPath string) string {
	return filepath.Join(fs.Root, relPath)
}

func (fs *fileSystem) GetAttr(name string, context *fuse.Context) (a *fuse.Attr, code fuse.Status) {
	log.Debugf("GetAttr %v", fs.GetPath(name))
	fullPath := fs.GetPath(name)
	var err error = nil
	st := syscall.Stat_t{}
	if name == "" {
		// When GetAttr is called for the toplevel directory, we always want
		// to look through symlinks.
		err = syscall.Stat(fullPath, &st)
	} else {
		err = syscall.Lstat(fullPath, &st)
	}

	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	a = &fuse.Attr{}
	a.FromStat(&st)
	return a, fuse.OK
}

// Open opens a file.
// Download it from stor if file not exist
func (fs *fileSystem) Open(name string, flags uint32, context *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	log.Debug("Open %v", name)
	f, err := os.OpenFile(fs.GetPath(name), int(flags), 0)
	if os.IsNotExist(err) {
		//probably ReadOnly mode. if meta exist, get the file from stor.
		if err := fs.download(fs.GetPath(name)); err != nil {
			return nil, fuse.ToStatus(err)
		}
		return fs.Open(name, flags, context)
	} else if err != nil {
		return nil, fuse.ToStatus(err)
	}
	return nodefs.NewLoopbackFile(f), fuse.OK
}

func (fs *fileSystem) Chmod(path string, mode uint32, context *fuse.Context) (code fuse.Status) {
	err := os.Chmod(fs.GetPath(path), os.FileMode(mode))
	return fuse.ToStatus(err)
}

func (fs *fileSystem) Chown(path string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(os.Chown(fs.GetPath(path), int(uid), int(gid)))
}

func (fs *fileSystem) Truncate(path string, offset uint64, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(os.Truncate(fs.GetPath(path), int64(offset)))
}

func (fs *fileSystem) Readlink(name string, context *fuse.Context) (out string, code fuse.Status) {
	f, err := os.Readlink(fs.GetPath(name))
	return f, fuse.ToStatus(err)
}

func (fs *fileSystem) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(syscall.Mknod(fs.GetPath(name), mode, int(dev)))
}

func (fs *fileSystem) Mkdir(path string, mode uint32, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(os.Mkdir(fs.GetPath(path), os.FileMode(mode)))
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (fs *fileSystem) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(syscall.Unlink(fs.GetPath(name)))
}

func (fs *fileSystem) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(syscall.Rmdir(fs.GetPath(name)))
}

func (fs *fileSystem) Symlink(pointedTo string, linkName string, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(os.Symlink(pointedTo, fs.GetPath(linkName)))
}

func (fs *fileSystem) Rename(oldPath string, newPath string, context *fuse.Context) (codee fuse.Status) {
	err := os.Rename(fs.GetPath(oldPath), fs.GetPath(newPath))
	return fuse.ToStatus(err)
}

func (fs *fileSystem) Link(orig string, newName string, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(os.Link(fs.GetPath(orig), fs.GetPath(newName)))
}

func (fs *fileSystem) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	log.Debugf("Access %v", fs.GetPath(name))
	return fuse.ToStatus(syscall.Access(fs.GetPath(name), mode))
}

func (fs *fileSystem) Create(path string, flags uint32, mode uint32, context *fuse.Context) (fuseFile nodefs.File, code fuse.Status) {
	f, err := os.OpenFile(fs.GetPath(path), int(flags)|os.O_CREATE, os.FileMode(mode))
	return nodefs.NewLoopbackFile(f), fuse.ToStatus(err)
}

// download file from stor
func (fs *fileSystem) download(path string) error {
	log.Infof("Downloading file '%s'", path)
	meta, err := fs.Meta(path)
	if err != nil {
		log.Errorf("Failed to download due to metadata loading failed: %s", err)
		return err
	}

	url, err := fs.url(meta.Hash)
	if err != nil {
		log.Errorf("Failed to build file url: %s", err)
		return err
	}

	log.Info("Downloading: %s", url)

	response, err := http.Get(url)
	if err != nil {
		log.Errorf("Failed to download file from stor: %s", err)
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(response.Body)
		log.Errorf("Invalid response from stor(%d): %s", response.StatusCode, body)
		return syscall.ENOENT
	}

	broReader := brotli.NewReader(response.Body)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	if fs.backend.Encrypted {
		if meta.UserKey == "" {
			err := fmt.Errorf("encryption key is empty, can't decrypt file %v", path)
			log.Errorf("download failed:%v", err)
			return err
		}

		r := bytes.NewBuffer([]byte(meta.UserKey))
		bKey := []byte{}
		fmt.Fscanf(r, "%x", &bKey)

		sessionKey, err := crypto.DecryptAsym(fs.backend.ClientKey, bKey)
		if err != nil {
			log.Errorf("Error decrypting session key: %v", err)
			return err
		}

		if err := crypto.DecryptSym(sessionKey, broReader, file); err != nil {
			log.Errorf("Error decrypting data: %v", err)
			return err
		}
	} else {
		if _, err = io.Copy(file, broReader); err != nil {
			log.Errorf("Error downloading data: %v", err)
			return err
		}
	}

	return err
}

func (fs *fileSystem) Meta(path string) (*meta.MetaFile, error) {
	m := meta.GetMeta(path)
	return m.Load()
}

func (fs *fileSystem) url(hash string) (string, error) {
	u, err := url.Parse(fs.stor.Addr)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "store", fs.backend.Namespace, hash)

	return u.String(), nil
}
