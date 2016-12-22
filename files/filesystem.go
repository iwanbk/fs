package files

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dsnet/compress/brotli"
	"github.com/g8os/fs/crypto"
	"github.com/g8os/fs/meta"
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

func (fs *fileSystem) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	log.Debugf("GetAttr:%v", name)

	fs.populate(name, context)

	var st syscall.Stat_t
	attr := &fuse.Attr{}

	if err := syscall.Lstat(fs.GetPath(name), &st); err != nil {
		if err != syscall.ENOENT {
			log.Errorf("GetAttr failed for `%v` : %v", name, err)
		}
		return nil, fuse.ToStatus(err)
	}

	attr.FromStat(&st)
	return attr, fuse.OK
}

// Open opens a file.
// Download it from stor if file not exist
func (fs *fileSystem) Open(name string, flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	log.Debugf("Open %v", name)
	fs.populate(name, context)

	file, err := os.OpenFile(fs.GetPath(name), int(flags), 0)
	if err != nil {
		log.Errorf("failed to openfile `%v`:%v", name, err)
		return nil, fuse.ToStatus(err)
	}

	return NewLoopbackFile(file), fuse.OK
}

func (fs *fileSystem) Truncate(path string, offset uint64, context *fuse.Context) fuse.Status {
	fs.populate(path, context)

	err := os.Truncate(fs.GetPath(path), int64(offset))
	if err != nil {
		log.Errorf("Truncate `%v` failed : %v", err)
	}
	return fuse.ToStatus(err)
}

func (fs *fileSystem) Chmod(name string, mode uint32, context *fuse.Context) fuse.Status {
	fs.populate(name, context)

	err := os.Chmod(fs.GetPath(name), os.FileMode(mode))
	if err != nil {
		log.Errorf("os.Chmod failed for %v : %v", name, err)
	}
	return fuse.ToStatus(err)
}

func (fs *fileSystem) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	fs.populate(name, context)

	err := os.Lchown(fs.GetPath(name), int(uid), int(gid))
	if err != nil {
		log.Errorf("os.Chown failed for %v : %v", name, err)
	}
	return fuse.ToStatus(err)

}

func (fs *fileSystem) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	fs.populate(name, context)
	out, err := os.Readlink(fs.GetPath(name))
	if err != nil {
		log.Errorf("Readlink failed for `%v` : %v", name, err)
	}
	return out, fuse.ToStatus(err)
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (fs *fileSystem) Unlink(name string, context *fuse.Context) fuse.Status {
	fs.populate(name, context)

	err := syscall.Unlink(fs.GetPath(name))
	if err != nil {
		log.Errorf("Unlink failed for `%v` : %v", name, err)
	}

	return fuse.ToStatus(err)
}

func (fs *fileSystem) Symlink(pointedTo string, linkName string, context *fuse.Context) fuse.Status {
	log.Debugf("Symlink %v -> %v", pointedTo, linkName)

	fs.populate(linkName, context)
	fs.populateParentDir(linkName, context)

	return fuse.ToStatus(fs.symlink(pointedTo, linkName, context))
}

func (fs *fileSystem) symlink(pointedTo string, linkName string, context *fuse.Context) error {
	err := os.Symlink(pointedTo, fs.GetPath(linkName))
	if err != nil {
		log.Errorf("syscall symlink `%v` -> `%v` failed:%v", pointedTo, linkName, err)
	}
	return err
}

// Rename handles dir & file rename operation
func (fs *fileSystem) Rename(oldPath string, newPath string, context *fuse.Context) fuse.Status {
	fullOldPath := fs.GetPath(oldPath)
	fullNewPath := fs.GetPath(newPath)

	fs.populate(oldPath, context)
	fs.populate(newPath, context)
	fs.populateParentDir(newPath, context)

	log.Debugf("Rename (%v) -> (%v)", oldPath, newPath)

	// rename file
	err := os.Rename(fullOldPath, fullNewPath)
	if err != nil {
		log.Errorf("Rename failed:%v", err)
	}
	return fuse.ToStatus(err)
}

func (fs *fileSystem) Link(orig string, newName string, context *fuse.Context) fuse.Status {
	log.Debugf("Link `%v` -> `%v`", orig, newName)

	fs.populate(orig, context)
	fs.populate(newName, context)
	fs.populateParentDir(newName, context)

	err := os.Link(fs.GetPath(orig), fs.GetPath(newName))
	if err != nil {
		log.Errorf("os.Link failed `%v` -> `%v` :%v", orig, newName, err)
	}
	return fuse.ToStatus(err)
}

func (fs *fileSystem) Access(name string, mode uint32, context *fuse.Context) fuse.Status {
	fs.populate(name, context)

	err := syscall.Access(fs.GetPath(name), mode)
	if err != nil {
		log.Errorf("Access failed for `%v` : %v", name, err)
	}
	return fuse.ToStatus(err)
}

func (fs *fileSystem) Create(name string, flags uint32, mode uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	fs.populate(name, context)
	fs.populateParentDir(name, context)

	f, err := os.OpenFile(fs.GetPath(name), int(flags), os.FileMode(mode))
	if err != nil {
		log.Errorf("Create `%v` failed : %v", name, err)
		return nil, fuse.EIO
	}
	return NewLoopbackFile(f), fuse.OK
}

// download file from stor
func (fs *fileSystem) download(meta meta.Meta, path string) error {
	log.Infof("Downloading file '%s'", path)

	data, err := meta.Load()
	if err != nil {
		return err
	}

	body, err := fs.stor.Get(data.Hash)
	if err != nil {
		return err
	}

	defer body.Close()

	broReader, err := brotli.NewReader(body, nil)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	if fs.backend.Encrypted {
		if data.UserKey == "" {
			return fmt.Errorf("encryption key is empty, can't decrypt file %v", path)
		}

		r := bytes.NewBuffer([]byte(data.UserKey))
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
			_ = os.Remove(path)
			return err
		}
	}

	// setting locally file permission
	err = os.Chown(path, int(data.Uid), int(data.Gid))
	if err != nil {
		log.Errorf("Cannot chown %v to (%d, %d): %v", path, data.Uid, data.Gid, err)
	}

	// err = syscall.Chmod(path, 04755)
	err = syscall.Chmod(path, data.Permissions)
	if err != nil {
		log.Errorf("Cannot chmod %v to %d: %v", path, data.Permissions, err)
	}

	utbuf := &syscall.Utimbuf{
		Actime:  int64(data.Ctime),
		Modtime: int64(data.Mtime),
	}

	err = syscall.Utime(path, utbuf)
	if err != nil {
		log.Errorf("Cannot utime %v: %v", path, err)
	}

	return err
}

func (fs *fileSystem) Meta(path string) (meta.Meta, *meta.MetaData, fuse.Status) {
	m, exists := fs.meta.Get(path)
	if !exists {
		return nil, nil, fuse.ENOENT
	}

	md, err := m.Load()
	if err != nil {
		return nil, nil, fuse.EIO
	}
	return m, md, fuse.OK
}

// Utimens changes the access and modification times of the inode specified by filename to the actime and modtime fields of times respectively.
func (fs *fileSystem) Utimens(name string, aTime *time.Time, mTime *time.Time, context *fuse.Context) fuse.Status {
	fs.populate(name, context)
	fullPath := fs.GetPath(name)

	if fs.isSymlink(fullPath) { // TODO : add support for symlink
		return fuse.OK
	}
	err := os.Chtimes(fullPath, *aTime, *mTime)
	if err != nil {
		log.Errorf("Utimens `%v` failed:%v", name, err)
	}
	return fuse.ToStatus(err)
}

// StatFs get filesystem statistics
func (fs *fileSystem) StatFs(name string) *fuse.StatfsOut {
	buf := syscall.Statfs_t{}

	if err := syscall.Statfs(fs.GetPath(name), &buf); err != nil {
		log.Errorf("StatFs failed on `%v` : %v", fs.GetPath(name), err)
		return nil
	}

	out := &fuse.StatfsOut{}
	out.FromStatfsT(&buf)
	return out
}

func (fs *fileSystem) SetXAttr(name string, attr string, data []byte, flags int, context *fuse.Context) fuse.Status {
	log.Debugf("SetXAttr:%v", name)

	fs.populate(name, context)

	fullPath := fs.GetPath(name)

	err := syscall.Setxattr(fullPath, attr, data, flags)
	if err != nil {
		log.Errorf("setxattr failed for `%v`: %v", name, err)
	}
	return fuse.ToStatus(err)
}

func (fs *fileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	log.Debugf("GetXAttr:%v", name)

	fs.populate(name, context)

	dest := []byte{}

	_, err := syscall.Getxattr(fs.GetPath(name), attr, dest)
	if err != nil && err != syscall.ENODATA {
		log.Errorf("getxattr failed for `%v` : %v", name, err)
	}
	return dest, fuse.ToStatus(err)

}

func (fs *fileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	log.Error("RemoveXAttr")

	fs.populate(name, context)

	err := syscall.Removexattr(fs.GetPath(name), attr)
	if err != nil {
		log.Errorf("RemoveXAttr failed for `%v`: %v", name, err)
	}
	return fuse.ToStatus(err)
}

func (fs *fileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	log.Debugf("ListXAttr:%v", name)

	fs.populate(name, context)

	dest := []byte{}
	if _, err := syscall.Listxattr(fs.GetPath(name), dest); err != nil {
		log.Errorf("Listxattr failed for `%v` : %v", name, err)
		return []string{}, fuse.ToStatus(err)
	}

	// split by 0 char
	blines := bytes.Split(dest, []byte{0})

	lines := make([]string, 0, len(blines))
	for _, bl := range blines {
		lines = append(lines, string(bl))
	}
	return lines, fuse.OK
}

func (fs *fileSystem) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) fuse.Status {
	log.Debugf("Mknod:%v", name)

	fs.populate(name, context)
	fs.populateParentDir(name, context)

	return fuse.ToStatus(fs.mknod(name, mode, dev, context))
}

func (fs *fileSystem) mknod(name string, mode uint32, dev uint32, context *fuse.Context) error {
	err := syscall.Mknod(fs.GetPath(name), mode, int(dev))
	if err != nil {
		log.Errorf("Mknod `%v` failed : %v", name, err)
	}
	return err
}

// populate dir/file when needed
// to handle cases where we need access to directory/file
// while the directory, file, or directories above it
// hasn't been populated yet.
func (fs *fileSystem) populate(path string, ctx *fuse.Context) bool {
	m, exists := fs.meta.Get(path)
	if !exists || m.Stat() == meta.MetaPopulated {
		return false
	}

	fs.populateDirFile(path, ctx)
	return true
}

func (fs *fileSystem) populateParentDir(name string, ctx *fuse.Context) bool {
	return fs.populate(filepath.Dir(strings.TrimSuffix(name, "/")), ctx)
}

func (fs *fileSystem) populateDirFile(name string, ctx *fuse.Context) fuse.Status {
	log.Debugf("fuse : populate %v", name)

	// populate it, starting from the top
	var path string
	paths := strings.Split(name, "/")

	for _, p := range paths {
		path = filepath.Join(path, p)
		fullPath := fs.GetPath(path)

		// get meta
		m, md, st := fs.Meta(path)
		if st != fuse.OK {
			return st
		}
		if m.Stat() == meta.MetaPopulated {
			fs.cleanupMeta(m, md, 0)
			continue
		}

		// populate dir/file
		err := func() error {
			switch md.Filetype {
			case syscall.S_IFDIR: // it is a directory
				return os.Mkdir(fullPath, os.FileMode(md.Permissions))
			case syscall.S_IFREG:
				return fs.download(m, fullPath)
			case syscall.S_IFLNK:
				return fs.symlink(md.Extended, path, ctx)
			default:
				return fs.mknod(path, md.Permissions|md.Filetype, uint32((md.DevMajor*256)+md.DevMinor), ctx)
			}
		}()
		if err != nil {
			log.Errorf("populateDirFile `%v` failed : %v", path, err)
			return fuse.ToStatus(err)
		}
		m.SetStat(meta.MetaPopulated)
		fs.cleanupMeta(m, md, 0)
	}
	return fuse.OK
}

// delete meta and it's children after being populated
// TODO : optimize this code
func (fs *fileSystem) cleanupMeta(m meta.Meta, md *meta.MetaData, level int) error {
	if level == 2 {
		return nil
	}
	if md.Filetype != syscall.S_IFDIR {
		return fs.meta.Delete(m)
	}

	children := []meta.Meta{}
	for child := range m.Children() {
		children = append(children, child)
	}

	if len(children) == 0 && m.Stat() == meta.MetaPopulated {
		return fs.meta.Delete(m)
	}
	for _, child := range children {
		childMd, err := child.Load()
		if err != nil {
			continue
		}
		if childMd.Filetype == syscall.S_IFDIR {
			fs.cleanupMeta(child, childMd, level+1)
		}
	}
	return nil
}

func (fs *fileSystem) isSymlink(fullPath string) bool {
	fi, err := os.Lstat(fullPath)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink == os.ModeSymlink
}
