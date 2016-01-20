package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/metadata"
	"github.com/Jumpscale/aysfs/ro"
	"github.com/Jumpscale/aysfs/rw"
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

func MountROFS(mountCfg config.Mount, stor *config.Aydostor, opts Options) error {

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

	flist, err := readFlistFile(mountCfg.Flist)
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

func MountRWFS(mountCfg config.Mount, backendCfg *config.Backend, storCfg *config.Aydostor) error {
	fs := rw.NewFS(mountCfg.Path, backendCfg, storCfg)

	log.Info("Mounting Fuse File system")
	if err := mountFuse(fs, mountCfg.Path, false); err != nil {
		log.Fatal(err)
	}

	return nil
}
