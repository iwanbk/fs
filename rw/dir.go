package rw

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"fmt"
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/Jumpscale/aysfs/utils"
	"golang.org/x/net/context"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

type fsDir struct {
	fsBase
	fs     *FS
	parent *fsDir
}

func newDir(fs *FS, path string, parent *fsDir) *fsDir {
	return &fsDir{
		fsBase: fsBase{
			path: path,
		},
		fs:     fs,
		parent: parent,
	}
}

func (n *fsDir) getDirent(entry os.FileInfo) (fuse.Dirent, bool) {
	name := entry.Name()

	dirEntry := fuse.Dirent{
		Name: name,
	}

	if entry.IsDir() {
		dirEntry.Type = fuse.DT_Dir
	} else {
		dirEntry.Type = fuse.DT_File
	}

	if !entry.IsDir() && strings.HasSuffix(name, meta.MetaSuffix) {
		name = strings.TrimSuffix(name, meta.MetaSuffix)
		if utils.Exists(path.Join(n.path, name)) {
			return dirEntry, false
		}

		dirEntry.Name = name
	}

	return dirEntry, true
}

func (n *fsDir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	files, err := ioutil.ReadDir(n.path)
	if err != nil {
		return nil, utils.ErrnoFromPathError(err)
	}

	entries := make([]fuse.Dirent, 0)

	for _, entry := range files {
		if ent, ok := n.getDirent(entry); ok {
			entries = append(entries, ent)
		}
	}

	return entries, nil
}

func (n *fsDir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	fullPath := path.Join(n.path, name)
	stat, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		metaPath := fmt.Sprintf("%s%s", fullPath, meta.MetaSuffix)
		stat, err = os.Stat(metaPath)
		if err != nil {
			return nil, utils.ErrnoFromPathError(err)
		}
	} else if err != nil {
		return nil, utils.ErrnoFromPathError(err)
	}

	if stat.IsDir() {
		return newDir(n.fs, fullPath, n), nil
	} else {
		return newFile(n.fs, fullPath, n), nil
	}
}

func (n *fsDir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	fullPath := path.Join(n.path, req.Name)
	err := os.Mkdir(fullPath, req.Mode)
	if err != nil {
		return nil, utils.ErrnoFromPathError(err)
	}

	return newDir(n.fs, fullPath, n), nil
}

func (n *fsDir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	fullPath := path.Join(n.path, req.Name)
	node := newFile(n.fs, fullPath, n)
	handle, err := node.open(req.Flags)

	return node, handle, err
}

func (n *fsDir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	fullPath := path.Join(n.path, req.Name)
	fullMetaPath := fmt.Sprintf("%s%s", fullPath, meta.MetaSuffix)

	err := os.Remove(fullPath)
	if merr := os.Remove(fullMetaPath); merr == nil {
		if os.IsNotExist(err) {
			//the file itself doesn't exist but the meta does.
			return nil
		}
	}

	if err != nil {
		return utils.ErrnoFromPathError(err)
	}

	return err
}
