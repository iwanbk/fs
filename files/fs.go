package files

import (
	"os"
	"time"

	"github.com/g8os/fs/config"
	"github.com/g8os/fs/tracker"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("files")
)

type FS struct {
	//root       *fsDir
	mountpoint string
	backend    *config.Backend
	stor       *config.Aydostor
	//factory    Factory
	tracker tracker.Tracker
	overlay bool
	conn    *nodefs.FileSystemConnector
	pathFs  *pathfs.PathNodeFs
	server  *fuse.Server
}

func NewFS(mountpoint string, backend *config.Backend, stor *config.Aydostor, tracker tracker.Tracker, overlay, readOnly bool) *FS {
	fs := &FS{
		mountpoint: mountpoint,
		backend:    backend,
		stor:       stor,
		//factory:    NewFactory(),
		tracker: tracker,
		overlay: overlay,
	}
	loopbackfs := NewLoopbackFileSystem(fs.backend.Path)

	opts := &nodefs.Options{
		// These options are to be compatible with libfuse defaults,
		// making benchmarking easier.
		NegativeTimeout: time.Second,
		AttrTimeout:     time.Second,
		EntryTimeout:    time.Second,
	}
	fs.pathFs = pathfs.NewPathNodeFs(loopbackfs, nil)
	fs.conn = nodefs.NewFileSystemConnector(fs.pathFs.Root(), opts)

	mOpts := &fuse.MountOptions{
		AllowOther: false,
		Name:       "loopbackfs",
		FsName:     fs.backend.Path,
	}
	state, err := fuse.NewServer(fs.conn.RawFS(), mountpoint, mOpts)
	if err != nil {
		os.Exit(1)
	}
	fs.server = state
	//fs.server.SetDebug(true)

	return fs
}

func (fs *FS) Serve() {
	fs.server.Serve()
}
