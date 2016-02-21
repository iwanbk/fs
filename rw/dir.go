package rw

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
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
	} else if entry.Mode()&os.ModeSymlink > 0 {
		dirEntry.Type = fuse.DT_Link
		return dirEntry, true
	} else {
		dirEntry.Type = fuse.DT_File
	}

	fullPath := path.Join(n.path, name)
	if strings.HasSuffix(fullPath, meta.MetaSuffix) {
		//We are processing a meta file.
		fullPath = strings.TrimSuffix(fullPath, meta.MetaSuffix)
		if utils.Exists(fullPath) {
			//if the file itself is there just skip because it will get processed anyway
			return dirEntry, false
		}
		m := meta.GetMeta(fullPath)
		if m.Stat().Deleted() {
			//file was deleted
			return dirEntry, false
		}
		dirEntry.Name = strings.TrimSuffix(name, meta.MetaSuffix)
	} else {
		//normal file.
		m := meta.GetMeta(fullPath)
		if m.Stat().Deleted() {
			return dirEntry, false
		}
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
	m := meta.GetMeta(fullPath)
	if m.Stat().Deleted() {
		log.Debugf("File '%s' is deleted according to meta", fullPath)
		return nil, fuse.ENOENT
	}
	stat, err := os.Lstat(fullPath)
	if os.IsNotExist(err) {
		stat, err = os.Lstat(string(m))
		if os.IsNotExist(err) {
			return nil, fuse.ENOENT
		} else if err != nil {
			return nil, utils.ErrnoFromPathError(err)
		}
	} else if err != nil {
		return nil, utils.ErrnoFromPathError(err)
	}

	if stat.IsDir() {
		return n.fs.factory.Dir(n.fs, fullPath, n), nil
	} else if stat.Mode()&os.ModeSymlink > 0 {
		return n.fs.factory.Link(n.fs, fullPath, n), nil
	} else {
		return n.fs.factory.File(n.fs, fullPath, n), nil
	}
}

func (n *fsDir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	fullPath := path.Join(n.path, req.Name)
	err := os.Mkdir(fullPath, req.Mode)
	if err != nil && !os.IsExist(err) {
		return nil, utils.ErrnoFromPathError(err)
	}
	n.fs.tracker.Touch(fullPath)
	return n.fs.factory.Dir(n.fs, fullPath, n), nil
}

func (n *fsDir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	fullPath := path.Join(n.path, req.Name)
	node := n.fs.factory.File(n.fs, fullPath, n).(*fsFile)
	handle, err := node.open(req.Flags)

	return node, handle, err
}

func (n *fsDir) Symlink(ctx context.Context, req *fuse.SymlinkRequest) (fs.Node, error) {
	fullPath := path.Join(n.path, req.NewName)
	err := os.Symlink(req.Target, fullPath)
	if err != nil {
		return nil, err
	}
	return n.fs.factory.Link(n.fs, fullPath, n), nil
}

func (n *fsDir) touchDeleted(name string) {
	m := meta.GetMeta(name)
	if !m.Exists() {
		m.Save(&meta.MetaFile{})
	}

	m.SetStat(m.Stat().SetDeleted(true).SetModified(true))
}

func (n *fsDir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	fullPath := path.Join(n.path, req.Name)
	m := meta.GetMeta(fullPath)

	defer func() {
		n.fs.factory.Forget(fullPath)
		if n.fs.overlay {
			//Set delete mark
			n.touchDeleted(fullPath)
		}
	}()

	err := os.Remove(fullPath)
	if !n.fs.overlay {
		if merr := os.Remove(string(m)); merr == nil {
			if os.IsNotExist(err) {
				//the file itself doesn't exist but the meta does.
				return nil
			}
		}
	}

	if !n.fs.overlay && err != nil && !os.IsNotExist(err) {
		return utils.ErrnoFromPathError(err)
	}

	return nil
}

func (d *fsDir) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	if dir, ok := newDir.(*fsDir); ok {
		log.Debugf("Rename (%s/%s) to (%s/%s)'", d.path, req.OldName, dir.path, req.NewName)
		oldPath := path.Join(d.path, req.OldName)
		newPath := path.Join(dir.path, req.NewName)

		oldNode, ok := d.fs.factory.Get(oldPath)
		if ok {
			defer func() {
				switch node := oldNode.(type) {
				case *fsFile:
					node.path = newPath
				case *fsDir:
					node.path = newPath
				default:
					log.Errorf("Failed to update node path to '%s'", newPath)
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
		if err != nil && !os.IsNotExist(err) {
			return utils.ErrnoFromPathError(err)
		}
		if d.fs.overlay {
			m := meta.GetMeta(oldPath)
			if m.Exists() {
				info, err := m.Load()
				if err != nil {
					return utils.ErrnoFromPathError(err)
				}
				nm := meta.GetMeta(newPath)
				nm.Save(info)
			}
		} else {
			os.Rename(meta.GetMeta(oldPath).String(), meta.GetMeta(newPath).String())
		}

		return nil
	} else {
		log.Errorf("Not the expected directory type")
		return fuse.EIO
	}
}
