package rw

import (
	"bazil.org/fuse"
	"golang.org/x/net/context"
	"os"
	"syscall"
)

type fsBase struct {
	path string
}

func (n *fsBase) Attr(ctx context.Context, attr *fuse.Attr) error {
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
