package rw

import (
	"bazil.org/fuse"
	"os"
	"syscall"
)

func ErrnoFromPathError(err error) fuse.Errno {
	if err, ok := err.(*os.PathError); ok {
		log.Debugf("Error is a path error: %s", err)
		if errno, ok := err.Err.(syscall.Errno); ok {
			log.Debugf("Error is a syscall.Errono error: %s", errno)
			return fuse.Errno(errno)
		} else {
			log.Debugf("Error is NOT a syscall.Errono error: %s", errno)
			return fuse.EIO
		}
	} else {
		log.Debugf("Error is NOT a path error: %s", err)
		return fuse.EIO
	}
}
