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
	fs.populate(path, context)

	err := os.Mkdir(fs.GetPath(path), os.FileMode(mode))
	if err != nil {
		log.Errorf("Mkdir failed for `%v` : %v", err)
	}
	return fuse.ToStatus(err)

}

// Rmdir deletes a directory
func (fs *fileSystem) Rmdir(name string, context *fuse.Context) fuse.Status {
	fs.populate(name, context)

	err := os.Remove(fs.GetPath(name))
	if err != nil {
		log.Errorf("Rmdir failed for `%v` : %v", name, err)
	}
	return fuse.ToStatus(err)
}

// OpenDir opens a directory and return all files/dir in the directory.
// If it finds .meta file, it shows the file represented by that meta
func (fs *fileSystem) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	log.Debugf("OpenDir %v", name)
	fs.populate(name, context)
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
		fs.populate(filepath.Join(dir, child.Name()), ctx)
	}

}
