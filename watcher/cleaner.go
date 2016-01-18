package watcher

//
//import (
//	"github.com/Jumpscale/aysfs/config"
//	"github.com/Jumpscale/aysfs/rw/meta"
//	"github.com/robfig/cron"
//	"os"
//	"path/filepath"
//	"strings"
//)
//
//type backenCleaner struct {
//	backend *config.Backend
//}
//
//func NewCleaner(backend *config.Backend) cron.Job {
//	return &backenCleaner{
//		backend: backend,
//	}
//}
//
//func (c *backenCleaner) Run() {
//	if err := filepath.Walk(c.backend.Path, c.walkFN); err != nil {
//		log.Errorf("Failed to walk backend '%s': %s", c.backend.Path, err)
//	}
//}
//
//func (c *backenCleaner) walkFN(name string, info os.FileInfo, err error) {
//	if strings.HasSuffix(name, meta.MetaSuffix) {
//		return nil
//	}
//
//
//}
