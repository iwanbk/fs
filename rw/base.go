package rw

import (
	"bazil.org/fuse"
	"fmt"
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/Jumpscale/aysfs/utils"
	"golang.org/x/net/context"
	"os"
	"syscall"
)

const (
	DefaultFileMode os.FileMode = 0755
)

type fsBase struct {
	path string
}

func (n *fsBase) Attr(ctx context.Context, attr *fuse.Attr) error {
	log.Debugf("Attr %s", n.path)
	stat, err := os.Stat(n.path)
	var size uint64 = 0

	if os.IsNotExist(err) {
		log.Debugf("Attr: File does not exist '%s'", n.path)
		metaPath := fmt.Sprintf("%s%s", n.path, meta.MetaSuffix)

		stat, err = os.Stat(metaPath)
		if err != nil {
			log.Debugf("Attr: File meta does not exist '%s.meta'", n.path)
			return utils.ErrnoFromPathError(err)
		}

		meta, err := meta.Load(metaPath)
		if err != nil {
			log.Debugf("Attr: Meta failed to load '%s.meta'", n.path)
			return utils.ErrnoFromPathError(err)
		}

		size = meta.Size
	} else if err != nil {
		log.Debugf("Attr: File '%s' error: %s", n.path, err)
		return utils.ErrnoFromPathError(err)
	} else {
		size = uint64(stat.Size())
	}

	attr.Mtime = stat.ModTime()
	attr.Mode = DefaultFileMode
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
