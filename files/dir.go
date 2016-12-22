package files

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanwen/go-fuse/fuse"

	"github.com/g8os/fs/meta"
)

// Mkdir creates a directory
func (fs *fileSystem) Mkdir(path string, mode uint32, context *fuse.Context) fuse.Status {
	fullPath := fs.GetPath(path)

	f := func() fuse.Status {
		if err := os.Mkdir(fullPath, os.FileMode(mode)); err != nil {
			return fuse.ToStatus(err)
		}
		_, err := fs.meta.CreateDir(path)
		return fuse.ToStatus(err)
	}

	if st := f(); st != fuse.ENOENT {
		return st
	}

	// only populate directories above it.
	fs.populateParentDir(path, context)

	return f()
	// This line break mkdir on OL
	// fs.tracker.Touch(fullPath)
}

// Rmdir deletes a directory
func (fs *fileSystem) Rmdir(name string, context *fuse.Context) fuse.Status {
	fullPath := fs.GetPath(name)
	log.Debugf("Rmdir %v", fullPath)

	f := func() fuse.Status {
		err := os.Remove(fullPath)
		if err != nil {
			log.Errorf("Rmdir failed for `%v` : %v", name, err)
		}
		return fuse.ToStatus(err)
	}

	if st := f(); st != fuse.ENOENT {
		return st
	}
	fs.populateDirFile(name, context)
	return f()
}

// OpenDir opens a directory and return all files/dir in the directory.
// If it finds .meta file, it shows the file represented by that meta
func (fs *fileSystem) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	log.Debugf("OpenDir %v", name)
	fs.populateDirFile(name, context)
	if m, exists := fs.meta.Get(name); exists {
		fs.populateDirents(name, m, context)
	}

	fullPath := fs.GetPath(name)

	d, err := os.Open(fullPath)
	if err != nil {
		log.Errorf("Opendir failed to os.Open:%v", err)
		return nil, fuse.ToStatus(err)
	}
	fis, err := d.Readdir(0)
	if err != nil {
		log.Errorf("readdir %v failed : %v", name, err)
		return nil, fuse.ToStatus(err)
	}
	dirents := make([]fuse.DirEntry, 0, len(fis))
	for _, fi := range fis {
		dirents = append(dirents, fuse.DirEntry{
			Name: strings.TrimPrefix(fi.Name(), fs.Root),
			Mode: uint32(fi.Mode()),
		})
	}
	return dirents, fuse.OK
}

func (fs *fileSystem) populateDirents(dir string, m meta.Meta, ctx *fuse.Context) {
	for child := range m.Children() {
		fs.populateDirFile(filepath.Join(dir, child.Name()), ctx)
	}

}
