package main

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/ro"
)

func mount(filesys fs.FS, mountpoint string) error {
	c, err := fuse.Mount(mountpoint, fuse.MaxReadahead(ro.FileReadBuffer), fuse.ReadOnly())
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
