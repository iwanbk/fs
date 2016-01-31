package utils

import (
	"bazil.org/fuse"
	"os"
	"syscall"
)

func ErrnoFromPathError(base error) error {
	if err, ok := base.(*os.PathError); ok {
		if errno, ok := err.Err.(syscall.Errno); ok {
			return fuse.Errno(errno)
		} else {
			return base
		}
	} else {
		return base
	}
}
