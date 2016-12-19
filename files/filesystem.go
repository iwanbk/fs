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

	var st syscall.Stat_t
	attr := &fuse.Attr{}

	_, exists := fs.meta.Get(name)
	if !exists {
		return nil, fuse.ENOENT
	}

	f := func() (*fuse.Attr, fuse.Status) {
		if err := syscall.Lstat(fs.GetPath(name), &st); err != nil {
			return nil, fuse.ToStatus(err)
		}
		attr.FromStat(&st)
		return attr, fuse.OK
	}
	if a, st := f(); st != fuse.ENOENT {
		return a, st
	}

	fs.populateDirFile(name, context)
	return f()
}

/*
func (fs *fileSystem) GetAttr1(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	var err error
	attr := &fuse.Attr{}

	m, exists := fs.meta.Get(name)

	if !exists {
		return nil, fuse.ENOENT
	}

	metadata, err := m.Load()
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	var st syscall.Stat_t
	err = syscall.Stat(fs.GetPath(name), &st)
	if err == nil {
		log.Debugf("GetAttr %v: metadata, forwarding from backend", fs.GetPath(name))
		attr.FromStat(&st)
		attr.Ino = metadata.Inode
		return attr, fuse.OK
	}

	attr.Size = metadata.Size
	attr.Mode = metadata.Filetype | metadata.Permissions

	if metadata.Filetype == syscall.S_IFLNK {
		attr.Mode = metadata.Filetype | 0777
		if err := syscall.Lstat(metadata.Extended, &st); err == nil {
			attr.Uid = st.Uid
			attr.Gid = st.Gid
			attr.Ctime = uint64(st.Ctim.Sec)
			attr.Mtime = uint64(st.Mtim.Sec)
		}
	} else {
		attr.Ctime = metadata.Ctime
		attr.Mtime = metadata.Mtime
		attr.Uid = metadata.Uid
		attr.Gid = metadata.Gid
	}

	attr.Ino = metadata.Inode

	// block and character devices
	if metadata.Filetype == syscall.S_IFCHR || metadata.Filetype == syscall.S_IFBLK {
		attr.Rdev = uint32((metadata.DevMajor * 256) + metadata.DevMinor)
	}

	return attr, fuse.OK
}
*/

// Open opens a file.
// Download it from stor if file not exist
func (fs *fileSystem) Open(name string, flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	var st syscall.Stat_t

	log.Debugf("Open %v", name)

	// check file meta
	m, exists := fs.meta.Get(name)
	if !exists {
		return nil, fuse.ENOENT
	}

	// check dir meta
	dir := path.Dir(name)
	if _, exists := fs.meta.Get(dir); !exists {
		return nil, fuse.ENOENT
	}

	// populate parent dir if needed
	if !fs.checkExist(fs.GetPath(dir)) {
		fs.populateDirFile(dir, context)
	}

	// download file if exist in meta
	// but not exist in backend
	err := syscall.Lstat(fs.GetPath(name), &st)
	if os.IsNotExist(err) {
		if err := fs.download(m, fs.GetPath(name)); err != nil {
			log.Errorf("Error getting file from stor: %s", err)
			return nil, fuse.EIO
		}
		return fs.Open(name, flags, context)
	}

	file, err := os.OpenFile(fs.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	return NewLoopbackFile(m, file), fuse.OK
}

// generate metadata from real file
func (fs *fileSystem) metaFromRealFile(fullPath string, md *meta.MetaData) (*meta.MetaData, error) {
	var st syscall.Stat_t
	if err := syscall.Lstat(fullPath, &st); err != nil {
		return nil, err
	}

	newMd := &meta.MetaData{
		Inode:       st.Ino,
		Size:        uint64(st.Size),
		Filetype:    st.Mode & uint32(os.ModeType),
		Uid:         st.Uid,
		Gid:         st.Gid,
		Permissions: st.Mode & uint32(os.ModePerm),
		Ctime:       uint64(st.Ctim.Sec),
		Mtime:       uint64(st.Mtim.Sec),
	}
	if md == nil {
		return newMd, nil
	}
	newMd.Hash = md.Hash
	newMd.Uname = md.Uname
	newMd.Gname = md.Gname
	newMd.Extended = md.Extended
	newMd.DevMajor = md.DevMajor
	newMd.DevMinor = md.DevMinor
	newMd.UserKey = md.UserKey
	newMd.StoreKey = md.StoreKey
	return newMd, nil
}

func (fs *fileSystem) Truncate(path string, offset uint64, context *fuse.Context) (code fuse.Status) {
	m, err := fs.meta.CreateFile(path)
	if err != nil {
		return fuse.ToStatus(err)
	}
	m.SetStat(m.Stat().SetModified(true))
	return fuse.ToStatus(os.Truncate(fs.GetPath(path), int64(offset)))
}

func (fs *fileSystem) Chmod(name string, mode uint32, context *fuse.Context) fuse.Status {
	fullPath := fs.GetPath(name)

	m, md, st := fs.Meta(name)
	if st != fuse.OK {
		return st
	}

	f := func() fuse.Status {
		if err := os.Chmod(fullPath, os.FileMode(mode)); err != nil {
			log.Errorf("Chmod failed for `%v` : %v", name, err)
			return fuse.ToStatus(err)
		}
		md.Permissions = mode & uint32(os.ModePerm)
		return fuse.ToStatus(m.Save(md))
	}

	if status := f(); status != fuse.ENOENT {
		return status
	}

	if status := fs.populateDirFile(name, context); status != fuse.OK {
		return status
	}
	return f()
}

func (fs *fileSystem) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	m, md, st := fs.Meta(name)
	if st != fuse.OK {
		return st
	}

	fullPath := fs.GetPath(name)

	f := func() fuse.Status {
		if err := syscall.Lchown(fullPath, int(uid), int(gid)); err != nil {
			log.Errorf("Chown failed for `%v` : %v", name, err)
			return fuse.ToStatus(err)
		}
		md.Uid = uid
		md.Gid = gid
		return fuse.ToStatus(m.Save(md))
	}

	if status := f(); status != fuse.ENOENT {
		return status
	}

	if status := fs.populateDirFile(name, context); status != fuse.OK {
		return status
	}
	return f()
}

func (fs *fileSystem) Readlink(name string, context *fuse.Context) (out string, code fuse.Status) {
	var err error

	m, exists := fs.meta.Get(name)
	if !exists {
		return "", fuse.ENOENT
	}
	metadata, err := m.Load()
	if err != nil {
		return "", fuse.ToStatus(err)
	}

	if metadata.Filetype != syscall.S_IFLNK {
		return "", fuse.EIO
	}

	return metadata.Extended, fuse.OK
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (fs *fileSystem) Unlink(name string, context *fuse.Context) fuse.Status {
	log.Debugf("Unlink:%v", name)
	fullPath := fs.GetPath(name)

	m, exists := fs.meta.Get(name)
	if !exists {
		log.Errorf("Unlink failed:`%v` not exist in meta", name)
		return fuse.ENOENT
	}

	if err := syscall.Unlink(fullPath); err != nil {
		log.Warning("data file '%s' doesn't exist", fullPath)
	}

	fs.meta.Delete(m)
	return fuse.OK
}

func (fs *fileSystem) Symlink(pointedTo string, linkName string, context *fuse.Context) fuse.Status {
	log.Debugf("Symlink %v -> %v", pointedTo, linkName)
	// check if linkName exist
	if _, exist := fs.meta.Get(linkName); exist {
		return fuse.EIO
	}

	if st := fs.symlink(pointedTo, linkName, context, true); st != fuse.ENOENT {
		return st
	}
	fs.populateParentDir(linkName, context)
	return fs.symlink(pointedTo, linkName, context, true)
}

func (fs *fileSystem) symlink(pointedTo string, linkName string, context *fuse.Context, createMeta bool) fuse.Status {
	if err := syscall.Symlink(pointedTo, fs.GetPath(linkName)); err != nil {
		log.Errorf("syscall symlink failed:%v", err)
		return fuse.ToStatus(err)
	}

	if !createMeta {
		return fuse.OK
	}

	m, err := fs.meta.CreateFile(linkName)
	if err != nil {
		syscall.Unlink(linkName) // clean it up
		return fuse.ToStatus(err)
	}
	return fuse.ToStatus(m.Save(&meta.MetaData{
		Filetype:    syscall.S_IFLNK,
		Extended:    pointedTo,
		Permissions: 0777,
	}))

}

// Rename handles dir & file rename operation
func (fs *fileSystem) Rename(oldPath string, newPath string, context *fuse.Context) fuse.Status {
	fullOldPath := fs.GetPath(oldPath)
	fullNewPath := fs.GetPath(newPath)

	log.Errorf("Rename (%v) -> (%v)", oldPath, newPath)

	m, exists := fs.meta.Get(oldPath)
	if !exists {
		return fuse.ENOENT
	}

	f := func() fuse.Status {
		// rename file
		if err := syscall.Rename(fullOldPath, fullNewPath); err != nil {
			log.Warning("Rename : data file doesn't exist")
			return fuse.ToStatus(err)
		}

		// adjust metadata
		info, err := m.Load()
		if err != nil {
			return fuse.ToStatus(err)
		}

		fs.meta.Delete(m)

		nm, err := fs.meta.CreateFile(newPath)
		if err != nil {
			return fuse.ToStatus(err)
		}

		return fuse.ToStatus(nm.Save(info))
	}
	if st := f(); st != fuse.ENOENT {
		return st
	}
	fs.populateDirFile(oldPath, context)
	fs.populateParentDir(newPath, context)
	return f()
}

func (fs *fileSystem) Link(orig string, newName string, context *fuse.Context) fuse.Status {
	log.Errorf("Link `%v` -> `%v`", orig, newName)

	_, origMd, st := fs.Meta(orig)
	if st != fuse.OK {
		return st
	}

	f := func() fuse.Status {
		if err := syscall.Link(fs.GetPath(orig), fs.GetPath(newName)); err != nil {
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

func (fs *fileSystem) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	log.Debugf("Access %v", fs.GetPath(name))
	return fuse.OK
	//return fuse.ToStatus(syscall.Access(fs.GetPath(name), mode))
}

func (fs *fileSystem) Create(name string, flags uint32, mode uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	log.Debugf("Create:%v", name)
	dir := path.Dir(name)

	if _, ok := fs.meta.Get(dir); !ok {
		return nil, fuse.ENOENT
	}
	fs.populateDirFile(dir, context)

	f, err := os.OpenFile(fs.GetPath(name), int(flags)|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		log.Errorf("Create `%v` failed : %v", name, err)
		return nil, fuse.EIO
	}

	m, err := fs.meta.CreateFile(name)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	md, err := fs.metaFromRealFile(fs.GetPath(name), nil)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	m.Save(md)

	return NewLoopbackFile(m, f), fuse.OK
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
	// check if exist
	m, md, st := fs.Meta(name)
	if st != fuse.OK {
		return st
	}

	f := func() fuse.Status {
		// modify backend
		ts := []syscall.Timespec{
			syscall.Timespec{Sec: int64(aTime.Second()), Nsec: int64(aTime.Nanosecond())},
			syscall.Timespec{Sec: int64(mTime.Second()), Nsec: int64(mTime.Nanosecond())},
		}

		if err := syscall.UtimesNano(fs.GetPath(name), ts); err != nil {
			log.Errorf("UtimesNano `%v` failed:%v", name, err)
			return fuse.ToStatus(err)
		}

		// modify metadata
		md.Mtime = uint64(mTime.Second())
		return fuse.ToStatus(m.Save(md))
	}

	if st := f(); st != fuse.ENOENT {
		return st
	}
	fs.populateDirFile(name, context)
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
	if err := syscall.Setxattr(fs.GetPath(name), attr, data, flags); err != syscall.ENOENT {
		return fuse.ToStatus(err)
	}
	fs.populateDirFile(name, context)

	return fuse.ToStatus(syscall.Setxattr(fs.GetPath(name), attr, data, flags))
}

func (fs *fileSystem) GetXAttr(name string, attr string, context *fuse.Context) ([]byte, fuse.Status) {
	log.Debugf("GetXAttr:%v", name)

	dest := []byte{}
	if _, err := syscall.Getxattr(fs.GetPath(name), attr, dest); err != syscall.ENOENT {
		return nil, fuse.ToStatus(err)
	}
	fs.populateDirFile(name, context)
	_, err := syscall.Getxattr(fs.GetPath(name), attr, dest)
	return dest, fuse.ToStatus(err)
}

func (fs *fileSystem) RemoveXAttr(name string, attr string, context *fuse.Context) fuse.Status {
	log.Error("RemoveXAttr")
	return fuse.ToStatus(syscall.Removexattr(fs.GetPath(name), attr))
}

func (fs *fileSystem) ListXAttr(name string, context *fuse.Context) ([]string, fuse.Status) {
	log.Errorf("ListXAttr:%v", name)
	dest := []byte{}

	if _, err := syscall.Listxattr(fs.GetPath(name), dest); err != nil {
		return []string{}, fuse.ToStatus(err)
	}
	blines := bytes.Split(dest, []byte{0})

	lines := make([]string, 0, len(blines))
	for _, bl := range blines {
		lines = append(lines, string(bl))
	}
	return lines, fuse.ToStatus(nil)
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
		return fuse.ToStatus(err)
	}

	if !createMeta {
		return fuse.OK
	}

	// create meta
	m, err := fs.meta.CreateFile(name)
	if err != nil {
		// FIXME : clean it up
		log.Errorf("Mknod failed : can't create meta:%v", err)
		return fuse.EIO
	}
	md, err := m.Load()
	if err != nil {
		return fuse.ToStatus(err)
	}
	md.Permissions = mode & uint32(os.ModePerm)
	md.Filetype = mode & uint32(os.ModeType)
	return fuse.ToStatus(m.Save(md))

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

		// populate dir/file
		switch md.Filetype {
		case syscall.S_IFDIR: // it is a directory
			if err := os.Mkdir(fullPath, os.FileMode(md.Permissions)); err != nil {
				return fuse.ToStatus(err)
			}
		case syscall.S_IFREG:
			if err := fs.download(m, fullPath); err != nil {
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
	}
	return fuse.OK
}

func (fs *fileSystem) populateParentDir(name string, ctx *fuse.Context) fuse.Status {
	return fs.populateDirFile(filepath.Dir(strings.TrimSuffix(name, "/")), ctx)
}

func (fs *fileSystem) checkExist(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
