package files

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

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
	var err error = nil
	var st syscall.Stat_t
	attr := &fuse.Attr{}

	log.Debugf("GetAttr %v", fs.GetPath(name))
	fullPath := fs.GetPath(name)

	if name == "" {
		// When GetAttr is called for the toplevel directory, we always want
		// to look through symlinks.
		err = syscall.Stat(fullPath, &st)
	} else {
		err = syscall.Lstat(fullPath, &st)
	}

	metadata := &meta.MetaFile{}
	m := meta.GetMeta(fullPath)

	if m.Exists() {
		// metadata exists but flagged as deleted
		if m.Stat().Deleted() {
			log.Errorf("%v: found but flagged as deleted", fullPath)
			return nil, fuse.ENOENT
		}

		// metadata exists, it's not modified and the file is not yet downloaded
		if !m.Stat().Modified() || st.Size == 0 {
			metadata, err = m.Load()

			if err != nil {
				log.Errorf("GetAttr: Meta failed to load '%s.meta': %s", fullPath, err)
				return attr, fuse.ToStatus(err)
			}

			/*
			if err := syscall.Lstat(string(m), &st); err != nil {
				return nil, fuse.ToStatus(err)
			}
			*/
		}

		if st.Size > 0 {
			log.Debugf("GetAttr %v: metadata, forwarding from backend", fs.GetPath(name))
			attr.FromStat(&st)
			return attr, fuse.OK
		}

	} else {
		// no metadata, no physical file, it doesn't exists
		if os.IsNotExist(err) {
			log.Debugf("GetAttr %v: not found at all", fs.GetPath(name))
			return nil, fuse.ENOENT
		}
	}

	//
	// metadata doesn't exists but physical filesize if greater than 0
	// this seems a valid file
	//
	if !m.Exists() || st.Size > 0 {
		log.Debugf("GetAttr %v: no metadata, forwarding from backend", fs.GetPath(name))
		attr.FromStat(&st)
		return attr, fuse.OK
	}

	//
	// now, metadata exists and physical file existe
	// but file is empty (not downloaded yet)
	// populating stat from metadata
	//

	attr.Size = metadata.Size
	attr.Mode = metadata.Filetype | metadata.Permissions
	attr.Ctime = metadata.Ctime
	attr.Mtime = metadata.Mtime
	attr.Uid = metadata.Uid
	attr.Gid = metadata.Gid

	if metadata.Filetype == syscall.S_IFREG {
		attr.Ino = metadata.Inode
	}

	// block and character devices
	if metadata.Filetype == syscall.S_IFCHR || metadata.Filetype == syscall.S_IFBLK {
		attr.Rdev = uint32((metadata.DevMajor * 256) + metadata.DevMinor)
	}

	return attr, fuse.OK
}

// Open opens a file.
// Download it from stor if file not exist
func (fs *fileSystem) Open(name string, flags uint32, context *fuse.Context) (fuseFile nodefs.File, status fuse.Status) {
	var st syscall.Stat_t
	var tmpsize = int64(1)

	log.Debug("Open %v", name)
	err := syscall.Lstat(fs.GetPath(name), &st)

	m := meta.GetMeta(fs.GetPath(name))
	if(m.Exists()) {
		metadata, err := m.Load()
		if err == nil {
			tmpsize = int64(metadata.Size)
		}

		if m.Stat().Deleted() {
			return nil, fuse.ENOENT
		}
	}

	if os.IsNotExist(err) || (st.Size == 0 && m.Exists() && tmpsize > 0) {
		//probably ReadOnly mode. if meta exist, get the file from stor.
		if err := fs.download(fs.GetPath(name)); err != nil {
			log.Errorf("Error getting file from stor: %s", err)
			return nil, fuse.EIO
		}
		return fs.Open(name, flags, context)
	}

	f, err := os.OpenFile(fs.GetPath(name), int(flags), 0)
	if err != nil {
		return nil, fuse.ToStatus(err)
	}

	return nodefs.NewLoopbackFile(f), fuse.OK
}

func (fs *fileSystem) Truncate(path string, offset uint64, context *fuse.Context) (code fuse.Status) {
	touchModify(fs.GetPath(path))
	return fuse.ToStatus(os.Truncate(fs.GetPath(path), int64(offset)))
}

func (fs *fileSystem) Chmod(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	fullPath := fs.GetPath(name)
	log.Debugf("Chmod %v", fullPath)

	return fuse.ToStatus(os.Chmod(fullPath, os.FileMode(mode)))
}

func (fs *fileSystem) Chown(name string, uid uint32, gid uint32, context *fuse.Context) (code fuse.Status) {
	fullPath := fs.GetPath(name)
	log.Debugf("Chown %v", fullPath)

	return fuse.ToStatus(os.Chown(fullPath, int(uid), int(gid)))
}

func (fs *fileSystem) Readlink(name string, context *fuse.Context) (out string, code fuse.Status) {
	var err error = nil
	var st syscall.Stat_t

	log.Debugf("ReadLink %v", fs.GetPath(name))

	fullPath := fs.GetPath(name)
	err = syscall.Stat(fullPath, &st)

	metadata := &meta.MetaFile{}

	if os.IsNotExist(err) || st.Size == 0 {
		m := meta.GetMeta(fullPath)
		metadata, err = m.Load()

		if err != nil {
			log.Errorf("ReadLink: Meta failed to load '%s.meta': %s", fullPath, err)
			return "", fuse.ToStatus(err)
		}

	} else if err != nil {
		return "", fuse.ToStatus(err)

	} else {
		f, err := os.Readlink(fs.GetPath(name))
		return f, fuse.ToStatus(err)
	}

	f := metadata.Extended

	return f, fuse.ToStatus(err)
}

// Don't use os.Remove, it removes twice (unlink followed by rmdir).
func (fs *fileSystem) Unlink(name string, context *fuse.Context) (code fuse.Status) {
	fullPath := fs.GetPath(name)
	m := meta.GetMeta(fullPath)

	defer func() {
		if fs.overlay {
			//Set delete mark
			touchDeleted(fullPath)
		}
	}()

	err := os.Remove(fullPath)
	if !fs.overlay {
		if merr := os.Remove(string(m)); merr == nil {
			if os.IsNotExist(err) {
				//the file itself doesn't exist but the meta does.
				return fuse.OK
			}
		}
	}

	if !fs.overlay && err != nil && !os.IsNotExist(err) {
		return fuse.ToStatus(err)
	}

	return fuse.OK
}

func (fs *fileSystem) Symlink(pointedTo string, linkName string, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(os.Symlink(pointedTo, fs.GetPath(linkName)))
}

// Rename handles dir & file rename operation
func (fs *fileSystem) Rename(oldPath string, newPath string, context *fuse.Context) (codee fuse.Status) {
	fullOldPath := fs.GetPath(oldPath)
	fullNewPath := fs.GetPath(newPath)

	log.Debugf("Rename (%v) -> (%v)", oldPath, newPath)

	defer func() {
		//make sure we mark the new path as changed.
		if fs.overlay {
			//touch old path as deleted
			touchDeleted(fullOldPath)
		}
	}()

	// rename file
	err := os.Rename(fullOldPath, fullNewPath)
	if err != nil && !os.IsNotExist(err) {
		return fuse.ToStatus(err)
	}

	// adjust metadata
	if fs.overlay {
		m := meta.GetMeta(fullOldPath)
		if m.Exists() {
			info, err := m.Load()
			if err != nil {
				return fuse.ToStatus(err)
			}
			nm := meta.GetMeta(fullNewPath)
			nm.Save(info)
		}
	} else {
		os.Rename(meta.GetMeta(fullOldPath).String(), meta.GetMeta(fullNewPath).String())
	}

	return fuse.ToStatus(nil)
}

func (fs *fileSystem) Link(orig string, newName string, context *fuse.Context) (code fuse.Status) {
	return fuse.ToStatus(os.Link(fs.GetPath(orig), fs.GetPath(newName)))
}

func (fs *fileSystem) Access(name string, mode uint32, context *fuse.Context) (code fuse.Status) {
	log.Debugf("Access %v", fs.GetPath(name))
	return fuse.ToStatus(syscall.Access(fs.GetPath(name), mode))
}

func (fs *fileSystem) Create(path string, flags uint32, mode uint32, context *fuse.Context) (fuseFile nodefs.File, code fuse.Status) {
	log.Debugf("Create %v", path)

	f, err := os.OpenFile(fs.GetPath(path), int(flags)|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return nil, fuse.EIO
	}

	m := meta.GetMeta(fs.GetPath(path))
	if m.Exists() {
		m.SetStat(m.Stat().SetDeleted(false))
	}

	return NewLoopbackFile(f), fuse.ToStatus(err)
}

// download file from stor
func (fs *fileSystem) download(path string) error {
	log.Infof("Downloading file '%s'", path)
	meta, err := fs.Meta(path)
	if err != nil {
		return err
	}

	body, err := fs.stor.Get(meta.Hash)
	if err != nil {
		return err
	}

	defer body.Close()

	broReader := brotli.NewReader(body)

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	if fs.backend.Encrypted {
		if meta.UserKey == "" {
			return fmt.Errorf("encryption key is empty, can't decrypt file %v", path)
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
			_ = os.Remove(path)
			return err
		}
	}

	// setting locally file permission
	err = os.Chown(path, int(meta.Uid), int(meta.Gid))
	if err != nil {
		log.Errorf("Cannot chown %v to (%d, %d): %v", path, meta.Uid, meta.Gid, err)
	}

	// err = syscall.Chmod(path, 04755)
	err = syscall.Chmod(path, meta.Permissions)
	if err != nil {
		log.Errorf("Cannot chmod %v to %d: %v", path, meta.Permissions, err)
	}

	utbuf := &syscall.Utimbuf{
		Actime:  int64(meta.Ctime),
		Modtime: int64(meta.Mtime),
	}

	err = syscall.Utime(path, utbuf)
	if err != nil {
		log.Errorf("Cannot utime %v: %v", path, err)
	}

	return err
}

func (fs *fileSystem) Meta(path string) (*meta.MetaFile, error) {
	m := meta.GetMeta(path)
	return m.Load()
}

func touchDeleted(name string) {
	m := meta.GetMeta(name)
	if !m.Exists() {
		m.Save(&meta.MetaFile{})
	}

	m.SetStat(m.Stat().SetDeleted(true).SetModified(true))
}

func touchModify(name string) {
	m := meta.GetMeta(name)
	if !m.Exists() {
		m.Save(&meta.MetaFile{})
	}

	m.SetStat(m.Stat().SetDeleted(false).SetModified(true))
}
