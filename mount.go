package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/metadata"
	"github.com/Jumpscale/aysfs/ro"
	"github.com/Jumpscale/aysfs/rw"
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/Jumpscale/aysfs/tracker"
	"github.com/Jumpscale/aysfs/utils"
	"github.com/Jumpscale/aysfs/watcher"
	"github.com/robfig/cron"
)

func mountFuse(filesys fs.FS, mountpoint string, readOnly bool) error {
	var c *fuse.Conn
	var err error

	if readOnly {
		c, err = fuse.Mount(mountpoint, fuse.MaxReadahead(ro.FileReadBuffer), fuse.ReadOnly())
	} else {
		c, err = fuse.Mount(mountpoint, fuse.MaxReadahead(ro.FileReadBuffer))
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

func watchReloadSignal(metadataDir string, flists [][]string, fs *ro.FS) {
	channel := make(chan os.Signal)
	signal.Notify(channel, syscall.SIGUSR1)
	go func(cfgPath string, fs *ro.FS) {
		defer close(channel)
		for {
			<-channel
			log.Info("Reloading ays mounts due to user signal")

			func() {
				//Put the fs down to prevent any access to filesystem
				fs.Down()
				defer fs.Up()

				log.Debug("Puring metadata")
				// delete Metadata
				fs.PurgeMetadata()

				fs.DiscoverMetadata(metadataDir)
				for _, flist := range flists {
					fs.AttachFList(flist)
				}

			}()
		}
	}(metadataDir, fs)
}

func mountROFS(mountCfg config.Mount, stor *config.Aydostor, opts Options) error {
	cacheMgr := cache.NewCacheManager()
	var meta metadata.Metadata

	switch opts.MetaEngine {
	case MetaEngineBolt:
		os.Remove(boltdb)
		if m, err := metadata.NewBoltMetadata(mountCfg.Path, boltdb); err != nil {
			log.Fatal("Failed to intialize metaengine", err)
		} else {
			meta = m
		}
	case MetaEngineMem:
		if m, err := metadata.NewMemMetadata(mountCfg.Path, nil); err != nil {
			log.Fatal("Failed to intialize metaengine", err)
		} else {
			meta = m
		}
	default:
		log.Fatalf("Unknown metadata engine '%s'", opts.MetaEngine)
	}

	fs := ro.NewFS(mountCfg.Path, meta, cacheMgr)
	var metadataDir string

	//auto discover local caches
	fs.AutoConfigCaches()

	//add stor from config
	cacheMgr.AddLayer(cache.NewHTTPCache(stor.Addr, "dedupe"))

	if metadataDir == "" {
		// TODO Make portable
		metadataDir = "/etc/ays/local"
	}

	//purge all purgable cache layers.
	fs.DiscoverMetadata(metadataDir)

	flist, err := utils.ReadFlistFile(mountCfg.Flist)
	if err != nil {
		return err
	}
	fs.AttachFList(flist)

	fmt.Println(fs)

	watchReloadSignal(metadataDir, [][]string{flist}, fs)

	//bring fileystem UP
	fs.Up()

	log.Info("Mounting Fuse File system")
	if err := mountFuse(fs, mountCfg.Path, true); err != nil {
		log.Fatal(err)
	}

	return nil
}

func MountROFS(wg *sync.WaitGroup, mount config.Mount, stor *config.Aydostor, opts Options) {
	mountROFS(mount, stor, opts)
	wg.Done()
}

func mountRWFS(mountCfg config.Mount, backendCfg *config.Backend, storCfg *config.Aydostor, tracker tracker.Tracker) error {
	fs := rw.NewFS(mountCfg.Path, backendCfg, storCfg, tracker)

	log.Info("Mounting Fuse File system")
	if err := mountFuse(fs, mountCfg.Path, false); err != nil {
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
	mountRWFS(mount, backend, stor, tracker)

	wg.Done()
}

func MountOLFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor *config.Aydostor, opts Options) {
	//1- generate the metadata
	if err := meta.PopulateFromPList(backend, mount.Path, mount.Flist); err != nil {
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
	tracker := tracker.NewDummyTracker()
	mountRWFS(mount, backend, stor, tracker)
	wg.Done()
}
