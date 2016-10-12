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
	"github.com/g8os/fs/rw"
	"github.com/g8os/fs/rw/meta"
	"github.com/g8os/fs/tracker"
	"github.com/g8os/fs/watcher"
	"github.com/robfig/cron"
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
	storCfg *config.Aydostor,
	tracker tracker.Tracker,
	overlay bool,
	readOnly bool) error {

	if backendCfg.Lib == "bazil" {
		fs := rw.NewFS(mountCfg.Path, backendCfg, storCfg, tracker, overlay)
		log.Info("Mounting Fuse File system")
		return mountFuse(fs, mountCfg.Path, readOnly)
	} else {
		fs, err := files.NewFS(mountCfg.Path, backendCfg, storCfg, tracker, overlay, readOnly)
		if err != nil {
			return err
		}
		log.Info("Serving File system")
		fs.Serve()
	}
	return nil
}

func MountRWFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor *config.Aydostor, opts Options) {
	//start the files watcher
	tracker := tracker.NewTracker()
	if backend.Upload {
		job, err := watcher.NewWatcher(backend, stor, tracker)
		if err != nil {
			log.Errorf("Failed to create backend watcher")
		} else {
			cron := backend.AydostorPushCron
			if cron == "" {
				cron = "@every 60m"
			}
			scheduler.AddJob(cron, job)
		}
	}

	job := watcher.NewCleaner(backend)
	cron := backend.CleanupCron
	if cron == "" {
		cron = "@every 1d"
	}
	scheduler.AddJob(cron, job)

	//Mount file system
	if err := mountFS(mount, backend, stor, tracker, false, false); err != nil {
		log.Fatal(err)
	}

	wg.Done()
}

func MountOLFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor *config.Aydostor, opts Options) {
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
	tracker := tracker.NewPurgeTracker()
	if err := mountFS(mount, backend, stor, tracker, true, false); err != nil {
		log.Fatal(err)
	}
	wg.Done()
}

func MountROFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor *config.Aydostor, opts Options) {
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
	tracker := tracker.NewPurgeTracker()
	if err := mountFS(mount, backend, stor, tracker, true, true); err != nil {
		log.Fatal(err)
	}
	wg.Done()
}
