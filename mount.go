package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/metadata"
	"github.com/Jumpscale/aysfs/ro"
	"github.com/Jumpscale/aysfs/rw"
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

func readFlistFile(path string) ([]string, error) {
	flistFile, err := os.Open(path)
	defer flistFile.Close()
	if err != nil {
		log.Errorf("Error opening flist %s :%v", path, err)
		return nil, err
	}
	metadata := []string{}
	scanner := bufio.NewScanner(flistFile)
	var line string
	for scanner.Scan() {
		line = scanner.Text()
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		metadata = append(metadata, line)
	}
	return metadata, nil
}

func mountROFS(mountCfg config.Mount, opts Options) error {
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
		log.Fatal("Unknown metadata engine '%s'", opts.MetaEngine)
	}

	fs := ro.NewFS(mountCfg.Path, meta, cacheMgr)
	var metadataDir string

	fs.AutoConfigCaches()

	// if _, err := os.Stat(opts.ConfigPath); err == nil {
	// 	cfg := config.LoadConfig(opts.ConfigPath)
	// 	metadataDir = cfg.Main.Metadata
	// 	// attaching cache layers to the fs
	// 	for _, c := range cfg.Cache {
	// 		u, err := url.Parse(c.URL)
	// 		if err != nil {
	// 			log.Fatalf("Invalid URL for cache '%s'", c.URL)
	// 		}
	// 		if u.Scheme == "" || u.Scheme == "file" {
	// 			//add FS layer
	// 			cacheMgr.AddLayer(cache.NewFSCache(u.Path, "dedupe", c.Purge))
	// 		} else if u.Scheme == "http" || u.Scheme == "https" {
	// 			if c.Purge {
	// 				log.Warning("HTTP cache '%s' doesn't support purging", c.URL)
	// 			}
	// 			cacheMgr.AddLayer(cache.NewHTTPCache(c.URL, "dedupe"))
	// 		} else if u.Scheme == "ssh" {
	// 			layer, err := cache.NewSFTPCache(c.URL, "dedupe")
	// 			if err != nil {
	// 				log.Fatalf("Failed to intialize cach layer '%s': %s", c.URL, err)
	// 			}
	// 			cacheMgr.AddLayer(layer)
	// 		}
	// }
	// }

	if metadataDir == "" {
		// TODO Make portable
		metadataDir = "/etc/ays/local"
	}

	//purge all purgable cache layers.
	// fs.DiscoverMetadata(metadataDir)

	flist, err := readFlistFile(mountCfg.Flist)
	if err != nil {
		return err
	}
	fs.AttachFList(flist)

	fmt.Println(fs)

	//watchReloadSignal(opts.ConfigPath, fs)

	//bring fileystem UP
	fs.Up()

	log.Info("Mounting Fuse File system")
	if err := mountFuse(fs, mountCfg.Path, true); err != nil {
		log.Fatal(err)
	}

	return nil
}

func MountROFS(wg *sync.WaitGroup, mount config.Mount, opts Options) {
	mountROFS(mount, opts)
	wg.Done()
}

func mountRWFS(mountCfg config.Mount, backendCfg *config.Backend, storCfg *config.Aydostor) error {
	fs := rw.NewFS(mountCfg.Path, backendCfg, storCfg)

	log.Info("Mounting Fuse File system")
	if err := mountFuse(fs, mountCfg.Path, false); err != nil {
		log.Fatal(err)
	}

	return nil
}

func MountRWFS(wg *sync.WaitGroup, scheduler *cron.Cron, mount config.Mount, backend *config.Backend, stor *config.Aydostor, opts Options) {
	//start the files watcher
	if backend.Upload {
		job, err := watcher.NewWatcher(backend, stor)
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
	mountRWFS(mount, backend, stor)

	wg.Done()
}
