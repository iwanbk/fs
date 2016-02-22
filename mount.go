package main

import (
	"sync"

	"os"
	"os/signal"
	"strings"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/rw"
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/Jumpscale/aysfs/tracker"
	"github.com/Jumpscale/aysfs/watcher"
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
				err = meta.PopulateFromPList(backend, base, mount.Flist)
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

	fs := rw.NewFS(mountCfg.Path, backendCfg, storCfg, tracker, overlay)

	log.Info("Mounting Fuse File system")
	if err := mountFuse(fs, mountCfg.Path, readOnly); err != nil {
		log.Fatal(err)
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
	mountFS(mount, backend, stor, tracker, false, false)

	wg.Done()
}

func MountOLFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor *config.Aydostor, opts Options) {
	//1- generate the metadata
	base := ""
	if mount.TrimBase {
		base = mount.Path
	}
	if err := meta.PopulateFromPList(backend, base, mount.Flist); err != nil {
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
	mountFS(mount, backend, stor, tracker, true, false)
	wg.Done()
}

func MountROFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor *config.Aydostor, opts Options) {
	//1- generate the metadata
	base := ""
	if mount.TrimBase {
		base = mount.Path
	}
	if err := meta.PopulateFromPList(backend, base, mount.Flist); err != nil {
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
	mountFS(mount, backend, stor, tracker, true, true)
	wg.Done()
}
