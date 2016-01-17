package rw

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"fmt"
	"golang.org/x/net/context"
	"io/ioutil"
	"os"
	"path"
	"syscall"
)

type fsDir struct {
	path   string
	parent *fsDir
}

func newDir(path string, parent *fsDir) *fsDir {
	return &fsDir{
		path:   path,
		parent: parent,
	}
}

func (n *fsDir) Attr(ctx context.Context, attr *fuse.Attr) error {
	stat, err := os.Stat(n.path)

	if err != nil {
		return err
	}

	attr.Mtime = stat.ModTime()
	attr.Mode = stat.Mode()
	attr.Size = uint64(stat.Size())
	if sys_stat := stat.Sys(); sys_stat != nil {
		if stat, ok := sys_stat.(*syscall.Stat_t); ok {
			attr.Inode = stat.Ino
		} else {
			log.Warning("Invalid system stat struct returned")
		}
	} else {
		log.Warning("Underlying fs stat is not available")
	}

	return nil
}

func (n *fsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	files, err := ioutil.ReadDir(n.path)
	if err != nil {
		return nil, err
	}

	entries := make([]fuse.Dirent, 0)

	for _, entry := range files {
		dirEntry := fuse.Dirent{
			Name: entry.Name(),
		}

		if entry.IsDir() {
			dirEntry.Type = fuse.DT_Dir
		} else {
			dirEntry.Type = fuse.DT_File
		}
		entries = append(entries, dirEntry)
	}

	return entries, nil
}

func (n *fsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	fullPath := path.Join(n.path, name)
	stat, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return newDir(fullPath, n), nil
	} else {
		return nil, fmt.Errorf("Not implemented")
	}
}
