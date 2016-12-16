package files

import (
	"os"

	"github.com/hanwen/go-fuse/fuse"
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
	fs.populateParentDir(path)

	return f()
	// This line break mkdir on OL
	// fs.tracker.Touch(fullPath)
}

// Rmdir deletes a directory
func (fs *fileSystem) Rmdir(name string, context *fuse.Context) fuse.Status {
	fullPath := fs.GetPath(name)
	log.Debugf("Rmdir %v", fullPath)
	m, exists := fs.meta.Get(name)
	if !exists {
		return fuse.ENOENT
	}

	f := func() fuse.Status {
		if err := os.Remove(fullPath); err != nil {
			return fuse.ToStatus(err)
		}
		return fuse.ToStatus(fs.meta.Delete(m))
	}

	if st := f(); st != fuse.ENOENT {
		return st
	}
	fs.populateDirFile(name)
	return f()
}

// OpenDir opens a directory and return all files/dir in the directory.
// If it finds .meta file, it shows the file represented by that meta
func (fs *fileSystem) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	log.Debugf("OpenDir %v", fs.GetPath(name))
	m, exists := fs.meta.Get(name)
	if !exists {
		return nil, fuse.ENOENT
	}

	fs.populateDirFile(name)

	var output []fuse.DirEntry
	log.Debugf("Listing children in directory %s", name)

	for child := range m.Children() {
		data, err := child.Load()
		if err != nil {
			return nil, fuse.ToStatus(err)
		}
		output = append(output, fuse.DirEntry{
			Name: child.Name(),
			Mode: data.Filetype,
		})
	}

	return output, fuse.OK
}
