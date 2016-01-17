package rw

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
	"io/ioutil"
	"os"
	"path"
)

type fsDir struct {
	fsBase
	parent *fsDir
}

func newDir(path string, parent *fsDir) *fsDir {
	return &fsDir{
		fsBase: fsBase{
			path: path,
		},
		parent: parent,
	}
}

func (n *fsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	files, err := ioutil.ReadDir(n.path)
	if err != nil {
		return nil, ErrnoFromPathError(err)
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
	if os.IsNotExist(err) {
		return nil, fuse.ENOENT
	}

	if err != nil {
		return nil, ErrnoFromPathError(err)
	}

	if stat.IsDir() {
		return newDir(fullPath, n), nil
	} else {
		return newFile(fullPath, n), nil
	}
}

func (n *fsDir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	fullPath := path.Join(n.path, req.Name)
	err := os.Mkdir(fullPath, req.Mode)
	if err != nil {
		return nil, ErrnoFromPathError(err)
	}

	return newDir(fullPath, n), nil
}

func (n *fsDir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	fullPath := path.Join(n.path, req.Name)
	node := newFile(fullPath, n)
	handle, err := node.open(req.Flags)

	return node, handle, err
}

func (n *fsDir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	fullPath := path.Join(n.path, req.Name)

	err := os.Remove(fullPath)
	if err != nil {
		return ErrnoFromPathError(err)
	}

	return err
}
