package main

import (
	"sync"

	"os"
	"os/signal"
	"strings"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/g8os/fs/config"
	"github.com/g8os/fs/files"
	"github.com/g8os/fs/meta"
	"github.com/g8os/fs/watcher"
	"github.com/robfig/cron"
	"github.com/g8os/fs/stor"
)

const (
	FileReadBuffer = 512 * 1024 //bytes [512K]
)

func mountFuse(filesys fs.FS, mountpoint string, readOnly bool) error {
	var c *fuse.Conn
	var err error

	if readOnly {
		c, err = fuse.Mount(mountpoint, fuse.MaxReadahead(FileReadBuffer), fuse.ReadOnly())
	} else {
		c, err = fuse.Mount(mountpoint, fuse.MaxReadahead(FileReadBuffer))
	}

	if err != nil {
		return err
	}

	defer c.Close()

	if err := fs.Serve(c, filesys); err != nil {
		return err
	}

	// check if the mount process has an error to report
	<-c.Ready
	if err := c.MountError; err != nil {
		return err
	}

	return nil
}

func watchReloadSignal(cfg *config.Config) {
	channel := make(chan os.Signal)
	signal.Notify(channel, syscall.SIGUSR1)
	go func(cfg *config.Config) {
		defer close(channel)
		for {
			<-channel
			log.Info("Reloading ays mounts due to user signal")

			for _, mount := range cfg.Mount {
				if strings.EqualFold(mount.Mode, config.RW) {
					continue
				}

				//process only RO, and OL
				backend, err := cfg.GetBackend(mount.Backend)
				if err != nil {
					log.Warningf("Couldn't retrive backend '%s'", backend.Name)
				}
				base := ""
				if mount.TrimBase {
					base = mount.Path
				}
				err = meta.PopulateFromPList(backend, base, mount.Flist, mount.Trim)
				if err != nil {
					log.Warningf("Couldn't reload backend meta: %s", err)
				}
			}
		}
	}(cfg)
}

func mountFS(
	mountCfg config.Mount,
	backendCfg *config.Backend,
	stor stor.Stor,
	overlay bool,
	readOnly bool) error {

	fs, err := files.NewFS(mountCfg.Path, backendCfg, stor, overlay, readOnly)
	if err != nil {
		return err
	}
	log.Info("Serving File system")
	fs.Serve()

	return nil
}

func MountRWFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor stor.Stor, opts Options) {
	//start the files watcher
	job := watcher.NewCleaner(backend)
	cron := backend.CleanupCron
	if cron == "" {
		cron = "@every 1d"
	}
	scheduler.AddJob(cron, job)

	//Mount file system
	if err := mountFS(mount, backend, stor, false, false); err != nil {
		log.Fatal(err)
	}

	wg.Done()
}

func MountOLFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor stor.Stor, opts Options) {
	//1- generate the metadata
	base := ""
	if mount.TrimBase {
		base = mount.Path
	}
	if err := meta.PopulateFromPList(backend, base, mount.Flist, mount.Trim); err != nil {
		log.Errorf("Failed to mount overllay fs '%s': %s", mount, err)
	}

	//2- Start the cleaner worker, but never the watcher since we don't push ever to stor in OL mode
	job := watcher.NewCleaner(backend)
	cron := backend.CleanupCron
	if cron == "" {
		cron = "@every 1d"
	}
	scheduler.AddJob(cron, job)

	//TODO: 3- start RWFS with overlay compatibility.
	if err := mountFS(mount, backend, stor, true, false); err != nil {
		log.Fatal(err)
	}
	wg.Done()
}

func MountROFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor stor.Stor, opts Options) {
	//1- generate the metadata
	base := ""
	if mount.TrimBase {
		base = mount.Path
	}
	if err := meta.PopulateFromPList(backend, base, mount.Flist, mount.Trim); err != nil {
		log.Errorf("Failed to mount overllay fs '%s': %s", mount, err)
	}

	//2- Start the cleaner worker, but never the watcher since we don't push ever to stor in OL mode
	job := watcher.NewCleaner(backend)
	cron := backend.CleanupCron
	if cron == "" {
		cron = "@every 1d"
	}
	scheduler.AddJob(cron, job)

	if err := mountFS(mount, backend, stor, true, true); err != nil {
		log.Fatal(err)
	}
	wg.Done()
}
