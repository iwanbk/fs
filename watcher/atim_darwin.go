// +build darwin
package watcher

import (
	"syscall"
)

func atim(s *syscall.Stat_t) syscall.Timespec {
	return s.Atimespec
}
