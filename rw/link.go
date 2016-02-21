package rw

import (
	"bazil.org/fuse"
	"golang.org/x/net/context"
	"os"
)

type fsLink struct {
	fsBase
	fs     *FS
	parent *fsDir
}

func newLink(fs *FS, path string, parent *fsDir) *fsLink {
	return &fsLink{
		fsBase: fsBase{
			path: path,
		},
		fs:     fs,
		parent: parent,
	}
}

func (l *fsLink) Readlink(ctx context.Context, req *fuse.ReadlinkRequest) (string, error) {
	log.Debugf("Reading symlink: %s", l.path)
	return os.Readlink(l.path)
}
