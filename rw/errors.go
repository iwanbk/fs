package rw

import (
	"bazil.org/fuse"
	"os"
	"syscall"
)

func ErrnoFromPathError(err error) fuse.Errno {
	if err, ok := err.(*os.PathError); ok {
		if errno, ok := err.Err.(syscall.Errno); ok {
			return fuse.Errno(errno)
		} else {
			return fuse.EIO
		}
	} else {
		return fuse.EIO
	}
}
