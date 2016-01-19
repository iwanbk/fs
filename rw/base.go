package rw

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"fmt"
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/Jumpscale/aysfs/utils"
	"golang.org/x/net/context"
	"os"
	"path"
	"syscall"
)

type fsBase struct {
	path string
}

func (n *fsBase) Attr(ctx context.Context, attr *fuse.Attr) error {
	stat, err := os.Stat(n.path)
	var size uint64 = 0

	if os.IsNotExist(err) {
		metaPath := fmt.Sprintf("%s%s", n.path, meta.MetaSuffix)

		stat, err = os.Stat(metaPath)
		if err != nil {
			return utils.ErrnoFromPathError(err)
		}

		meta, err := meta.Load(metaPath)
		if err != nil {
			return utils.ErrnoFromPathError(err)
		}

		size = meta.Size
	} else if err != nil {
		return utils.ErrnoFromPathError(err)
	} else {
		size = uint64(stat.Size())
	}

	attr.Mtime = stat.ModTime()
	attr.Mode = stat.Mode()
	attr.Size = size
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

func (b *fsBase) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	if dir, ok := newDir.(*fsDir); ok {
		log.Debugf("Rename (%s/%s) to (%s/%s)'", b.path, req.OldName, dir.path, req.NewName)
		err := os.Rename(path.Join(b.path, req.OldName),
			path.Join(dir.path, req.NewName))
		if err != nil {
			return utils.ErrnoFromPathError(err)
		}
		//rename meta if exists
		os.Rename(path.Join(b.path, fmt.Sprintf("%s%s", req.OldName, meta.MetaSuffix)),
			path.Join(dir.path, fmt.Sprintf("%s%s", req.NewName, meta.MetaSuffix)))
		return nil
	} else {
		log.Errorf("Not the expected directory type")
		return fuse.EIO
	}

}
