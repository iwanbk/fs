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
	"regexp"
	"strings"
)

var (
	SkipPattern = []*regexp.Regexp{
		regexp.MustCompile(`_\d+\.aydo$`), //backup extension before fs push.
		regexp.MustCompile(meta.OverlayDeletedSuffix + "$"),
	}
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

func (n *fsDir) skip(name string) bool {
	for _, r := range SkipPattern {
		if r.MatchString(name) {
			return true
		}
	}

	return false
}
func (n *fsDir) getDirent(entry os.FileInfo) (fuse.Dirent, bool) {
	name := entry.Name()

	dirEntry := fuse.Dirent{
		Name: name,
	}

	if n.skip(name) {
		return dirEntry, false
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
	log.Debugf("Listing dir: '%s'", n.path)
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
	log.Debugf("Looking up '%s/%s'", n.path, name)
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
		return n.fs.factory.Dir(n.fs, fullPath, n), nil
	} else {
		return n.fs.factory.File(n.fs, fullPath, n), nil
	}
}

func (n *fsDir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	fullPath := path.Join(n.path, req.Name)
	err := os.Mkdir(fullPath, req.Mode)
	if err != nil {
		return nil, utils.ErrnoFromPathError(err)
	}

	return n.fs.factory.Dir(n.fs, fullPath, n), nil
}

func (n *fsDir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	fullPath := path.Join(n.path, req.Name)
	node := n.fs.factory.File(n.fs, fullPath, n).(*fsFile)
	handle, err := node.open(req.Flags)

	return node, handle, err
}

func (n *fsDir) touchDeleted(name string) {
	markPath := fmt.Sprintf("%s%s", name, meta.OverlayDeletedSuffix)

	mark, err := os.Create(markPath)
	if err == nil {
		mark.Close()
	}
}

func (n *fsDir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	fullPath := path.Join(n.path, req.Name)
	fullMetaPath := fmt.Sprintf("%s%s", fullPath, meta.MetaSuffix)

	defer func() {
		if n.fs.overlay {
			//Set delete mark
			n.touchDeleted(fullPath)
		}
	}()

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

func (d *fsDir) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	if dir, ok := newDir.(*fsDir); ok {
		log.Debugf("Rename (%s/%s) to (%s/%s)'", d.path, req.OldName, dir.path, req.NewName)
		oldPath := path.Join(d.path, req.OldName)
		newPath := path.Join(dir.path, req.NewName)

		oldPathMeta := fmt.Sprintf("%s%s", oldPath, meta.MetaSuffix)
		newPathMeta := fmt.Sprintf("%s%s", newPath, meta.MetaSuffix)
		oldNode, ok := d.fs.factory.Get(oldPath)
		if ok {
			defer func() {
				if oldNode, ok := oldNode.(*fsFile); ok {
					log.Debugf("Changing node path to '%s'", newPath)
					oldNode.path = newPath
				}

				d.fs.factory.Forget(oldPath)
			}()
		}

		defer func() {
			//make sure we mark the new path as changed.
			d.fs.tracker.Touch(newPath)
			if d.fs.overlay {
				//touch old path as deleted
				d.touchDeleted(oldPath)
			}
		}()

		err := os.Rename(oldPath, newPath)
		if rerr := os.Rename(oldPathMeta, newPathMeta); rerr == nil {
			if os.IsNotExist(err) {
				log.Debugf("Rename file wasn't existing but the meta did.")
				//the file itself doesn't exist but the meta does.
				return nil
			}
		}

		if err != nil {
			return utils.ErrnoFromPathError(err)
		}

		return nil
	} else {
		log.Errorf("Not the expected directory type")
		return fuse.EIO
	}
}
