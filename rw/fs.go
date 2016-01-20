package rw

import (
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/tracker"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("rw")
)

type FS struct {
	root       *fsDir
	mountpoint string
	backend    *config.Backend
	stor       *config.Aydostor
	factory    Factory
	tracker    tracker.Tracker
	overlay    bool
}

func NewFS(
	mountpoint string,
	backend *config.Backend,
	stor *config.Aydostor,
	tracker tracker.Tracker,
	overlay bool) *FS {

	fs := &FS{
		mountpoint: mountpoint,
		backend:    backend,
		stor:       stor,
		factory:    NewFactory(),
		tracker:    tracker,
		overlay:    overlay,
	}

	fs.root = fs.factory.Dir(fs, fs.backend.Path, nil).(*fsDir)
	return fs
}

func (f *FS) Backend() *config.Backend {
	return f.backend
}

func (f *FS) Stor() *config.Aydostor {
	return f.stor
}

func (f *FS) Root() (fs.Node, error) {
	log.Debug("Accessing filesystem root")

	return f.root, nil
}
