package rw

import (
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/config"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("rw")
)

type FS struct {
	root       *fsDir
	mountpoint string
}

func NewFS(mountpoint string, backendCfg config.Backend, storCfg config.Aydostor) *FS {
	return &FS{
		root:       newDir(backendCfg.Path, nil),
		mountpoint: mountpoint,
	}
}

func (f *FS) Root() (fs.Node, error) {
	log.Debug("Accessing filesystem root")

	return f.root, nil
}
