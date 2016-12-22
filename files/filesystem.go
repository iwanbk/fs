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
	"path"
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

	fullPath := fs.GetPath(name)

	f := func() (*fuse.Attr, fuse.Status) {
		var st syscall.Stat_t
		attr := &fuse.Attr{}

		if err := syscall.Lstat(fullPath, &st); err != nil {
			return nil, fuse.ToStatus(err)
		}

		attr.FromStat(&st)
		return attr, fuse.OK
	}
	a, st := f()
	if st != fuse.ENOENT {
		return a, st
	}
	if !fs.populate(name, context) {
		return a, st
	}

	attr, st := f()
	if st != fuse.OK {
		log.Errorf("GetAttr failed for `%v` : %v", name, st)
	}
	return attr, st
}

// Open opens a file.
// Download it from stor if file not exist
func (fs *fileSystem) Open(name string, flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	log.Debugf("Open %v", name)

	f := func() (nodefs.File, fuse.Status) {
		file, err := os.OpenFile(fs.GetPath(name), int(flags), 0)
		if err != nil {
			log.Errorf("failed to openfile `%v`:%v", name, err)
			return nil, fuse.ToStatus(err)
		}

		return NewLoopbackFile(file), fuse.OK
	}

	file, st := f()
	if st != fuse.ENOENT {
		return file, st
	}

	if !fs.populate(name, context) {
		return file, st
	}
	return f()
}

func (fs *fileSystem) Truncate(path string, offset uint64, context *fuse.Context) fuse.Status {
	f := func() fuse.Status {
		return fuse.ToStatus(os.Truncate(fs.GetPath(path), int64(offset)))
	}
	st := f()
	if st != fuse.ENOENT {
		return st
	}

	if !fs.populate(path, context) {
		return st
	}
	return f()
}

func (fs *fileSystem) populate(path string, ctx *fuse.Context) bool {
	if _, exists := fs.meta.Get(path); !exists {
		if path == "lib64/ld-linux-x86-64.so.2" {
			log.Errorf("meta for lib64/ld-linux-x86-64.so.2 doesnt exist")
		}
		return false
	}

	fs.populateDirFile(path, ctx)
	return true
}

func (fs *fileSystem) Chmod(name string, mode uint32, context *fuse.Context) fuse.Status {
	fullPath := fs.GetPath(name)

	f := func() fuse.Status {
		err := os.Chmod(fullPath, os.FileMode(mode))
		if err != nil {
			log.Errorf("os.Chmod failed for %v : %v", name)
		}
		return fuse.ToStatus(err)
	}

	st := f()
	if st != fuse.ENOENT {
		return st
	}

	if !fs.populate(name, context) {
		return st
	}
	return f()
}

func (fs *fileSystem) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	fullPath := fs.GetPath(name)

	f := func() fuse.Status {
		err := os.Lchown(fullPath, int(uid), int(gid))

		if err != nil {
			log.Errorf("os.Chown failed for %v : %v", name)
		}
		return fuse.ToStatus(err)
	}
	st := f()
	if st != fuse.ENOENT {
		return st
	}
	if !fs.populate(name, context) {
		return st
	}

	return f()
}

func (fs *fileSystem) Readlink(name string, context *fuse.Context) (string, fuse.Status) {
	f := func() (string, fuse.Status) {
		out, err := os.Readlink(fs.GetPath(name))
		if err != nil {
			log.Errorf("Readlink failed for `%v` : %v", name, err)
		}
		return out, fuse.ToStatus(err)
	}

	out, st := f()
	if st != fuse.ENOENT {
		return out, st
	}

	if !fs.populate(name, context) {
		return out, st
	}
	return f()
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (fs *fileSystem) Unlink(name string, context *fuse.Context) fuse.Status {
	fullPath := fs.GetPath(name)

	f := func() fuse.Status {
		return fuse.ToStatus(syscall.Unlink(fullPath))
	}

	st := f()
	if st != fuse.ENOENT {
		return st
	}
	if !fs.populate(name, context) {
		return st
	}
	return f()
}

func (fs *fileSystem) Symlink(pointedTo string, linkName string, context *fuse.Context) fuse.Status {
	log.Debugf("Symlink %v -> %v", pointedTo, linkName)
	// check if linkName exist
	if st := fs.symlink(pointedTo, linkName, context, true); st != fuse.ENOENT {
		return st
	}
	fs.populateParentDir(linkName, context)
	return fs.symlink(pointedTo, linkName, context, true)
}

func (fs *fileSystem) symlink(pointedTo string, linkName string, context *fuse.Context, createMeta bool) fuse.Status {
	if err := os.Symlink(pointedTo, fs.GetPath(linkName)); err != nil {
		log.Errorf("syscall symlink `%v` -> `%v` failed:%v", pointedTo, linkName, err)
		return fuse.ToStatus(err)
	}
	return fuse.OK
}

// Rename handles dir & file rename operation
func (fs *fileSystem) Rename(oldPath string, newPath string, context *fuse.Context) fuse.Status {
	fullOldPath := fs.GetPath(oldPath)
	fullNewPath := fs.GetPath(newPath)

	log.Debugf("Rename (%v) -> (%v)", oldPath, newPath)

	f := func() fuse.Status {
		// rename file
		err := os.Rename(fullOldPath, fullNewPath)
		if err != nil {
			log.Errorf("Rename failed:%v", err)
		}
		return fuse.ToStatus(err)
	}
	if st := f(); st != fuse.ENOENT {
		return st
	}

	fs.populateDirFile(oldPath, context)
	fs.populateParentDir(newPath, context)
	return f()
}

func (fs *fileSystem) Link(orig string, newName string, context *fuse.Context) fuse.Status {
	log.Debugf("Link `%v` -> `%v`", orig, newName)

	_, origMd, st := fs.Meta(orig)
	if st != fuse.OK {
		return st
	}

	f := func() fuse.Status {
		if err := os.Link(fs.GetPath(orig), fs.GetPath(newName)); err != nil {
			log.Errorf("os.Link failed:%v", err)
			return fuse.ToStatus(err)
		}
		m, err := fs.meta.CreateFile(newName)
		if err != nil {
			return fuse.ToStatus(err)
		}
		return fuse.ToStatus(m.Save(origMd))
	}
	if st := f(); st != fuse.ENOENT {
		return st
	}
	fs.populateDirFile(orig, context)
	fs.populateParentDir(newName, context)
	return f()

}

func (fs *fileSystem) Access(name string, mode uint32, context *fuse.Context) fuse.Status {
	f := func() fuse.Status {
		return fuse.ToStatus(syscall.Access(fs.GetPath(name), mode))
	}
	st := f()
	if st != fuse.ENOENT {
		return st
	}
	if !fs.populate(name, context) {
		return st
	}
	return f()
}

func (fs *fileSystem) Create(name string, flags uint32, mode uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	dir := path.Dir(name)
	fs.populate(dir, context)

	f := func() (nodefs.File, fuse.Status) {
		f, err := os.OpenFile(fs.GetPath(name), int(flags), os.FileMode(mode))
		if err != nil {
			log.Errorf("Create `%v` failed : %v", name, err)
			return nil, fuse.EIO
		}
		return NewLoopbackFile(f), fuse.OK
	}
	return f()
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
	fullPath := fs.GetPath(name)
	f := func() fuse.Status {
		// modify backend
		/*ts := []syscall.Timeval{
			syscall.Timeval{Sec: int64(aTime.Second()), Usec: int64(aTime.Nanosecond() / 1000)},
			syscall.Timeval{Sec: int64(mTime.Second()), Usec: int64(mTime.Nanosecond() / 1000)},
		}*/

		//err := syscall.Utimes(fs.GetPath(name), ts)
		if fs.isSymlink(fullPath) {
			return fuse.OK
		}
		err := os.Chtimes(fullPath, *aTime, *mTime)
		if err != nil {
			log.Errorf("UtimesNano `%v` failed:%v", name, err)
		}
		return fuse.ToStatus(err)
	}

	st := f()
	if st != fuse.ENOENT {
		return st
	}
	if !fs.populate(name, context) {
		return st
	}
	return f()
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

	fullPath := fs.GetPath(name)

	f := func() fuse.Status {
		err := syscall.Setxattr(fullPath, attr, data, flags)
		if err != nil {
			log.Errorf("setxattr failed for `%v`: %v", name, err)
		}
		return fuse.ToStatus(err)
	}
	st := f()
	if st != fuse.ENOENT {
		return st
	}
	if !fs.populate(name, context) {
		return st
	}
	return f()
}

func (fs *fileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	log.Debugf("GetXAttr:%v", name)

	fullPath := fs.GetPath(name)

	f := func() ([]byte, fuse.Status) {
		dest := []byte{}
		_, err := syscall.Getxattr(fullPath, attr, dest)
		return dest, fuse.ToStatus(err)
	}

	dest, st := f()
	if st != fuse.ENOENT {
		return dest, st
	}
	if !fs.populate(name, context) {
		return dest, st
	}
	dest, st = f()
	if st != fuse.OK {
		log.Errorf("getxattr failed for `%v` : %v", name, st)
	}
	return dest, st
}

func (fs *fileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	log.Error("RemoveXAttr")

	f := func() fuse.Status {
		err := syscall.Removexattr(fs.GetPath(name), attr)
		if err != nil {
			log.Errorf("Getxattr failed for `%v`: %v", name)
		}
		return fuse.ToStatus(err)
	}
	st := f()
	if st != fuse.ENOENT {
		return st
	}
	if !fs.populate(name, context) {
		return st
	}
	return f()
}

func (fs *fileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	log.Debugf("ListXAttr:%v", name)

	f := func() ([]string, fuse.Status) {
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

	out, st := f()
	if st != fuse.ENOENT {
		return out, st
	}

	if !fs.populate(name, context) {
		return out, st
	}
	return f()
}

func (fs *fileSystem) Mknod(name string, mode uint32, dev uint32, context *fuse.Context) fuse.Status {
	log.Debugf("Mknod:%v", name)

	// if already exist in meta, but not exist in backend
	if _, exist := fs.meta.Get(name); exist && !fs.checkExist(fs.GetPath(name)) {
		return fs.populateDirFile(name, context)
	}

	if st := fs.mknod(name, mode, dev, context, true); st != fuse.ENOENT {
		return st
	}
	fs.populateParentDir(name, context)
	return fs.mknod(name, mode, dev, context, true)
}

func (fs *fileSystem) mknod(name string, mode uint32, dev uint32, context *fuse.Context, createMeta bool) fuse.Status {
	if err := syscall.Mknod(fs.GetPath(name), mode, int(dev)); err != nil {
		log.Errorf("Mknod `%v` failed : %v", name, err)
		return fuse.ToStatus(err)
	}
	return fuse.OK
}

// populate dir/file when needed
// to handle cases where we need access to directory/file
// while the directory, file, or directories above it
// hasn't been populated yet.
func (fs *fileSystem) populateDirFile(name string, ctx *fuse.Context) fuse.Status {
	log.Debugf("fuse : populate %v", name)
	// check meta
	_, _, st := fs.Meta(name)
	if st != fuse.OK {
		return fuse.OK
	}

	// check in backend
	if fs.checkExist(fs.GetPath(name)) {
		return fuse.OK
	}

	// populate it, starting from the top
	var path string
	paths := strings.Split(name, "/")
	for _, p := range paths {
		path = path + "/" + p
		fullPath := fs.GetPath(path)

		// check if already exist in backend
		if fs.checkExist(fullPath) {
			continue
		}

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
		switch md.Filetype {
		case syscall.S_IFDIR: // it is a directory
			if err := os.Mkdir(fullPath, os.FileMode(md.Permissions)); err != nil {
				log.Errorf("populate dir `%v` failed:%v", path, err)
				return fuse.ToStatus(err)
			}
		case syscall.S_IFREG:
			if err := fs.download(m, fullPath); err != nil {
				log.Errorf("populate file `%v` failed:%v", path, err)
				return fuse.EIO
			}
		case syscall.S_IFLNK:
			if st := fs.symlink(md.Extended, path, ctx, false); st != fuse.OK {
				log.Errorf("failed to populate link `%v` -> `%v` : %v", path, md.Extended, st)
				return st
			}
		default:
			if st := fs.mknod(path, md.Permissions|md.Filetype, uint32((md.DevMajor*256)+md.DevMinor), ctx, false); st != fuse.OK {
				log.Errorf("failed to populate special file : `%v` : %v", path, st)
				return st
			}
		}
		m.SetStat(meta.MetaPopulated)
		fs.cleanupMeta(m, md, 0)
	}
	return fuse.OK
}

func (fs *fileSystem) populateParentDir(name string, ctx *fuse.Context) fuse.Status {
	return fs.populateDirFile(filepath.Dir(strings.TrimSuffix(name, "/")), ctx)
}

func (fs *fileSystem) checkExist(path string) bool {
	_, err := os.Lstat(path)
	return !os.IsNotExist(err)
}

// delete meta and it's children after being populated
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
