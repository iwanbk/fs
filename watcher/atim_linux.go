// +build linux
package watcher

import (
	"syscall"
)

func atim(s *syscall.Stat_t) syscall.Timespec {
	return s.Atim
}
