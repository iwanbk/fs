package cache

import (
	"io"
	"path/filepath"
)

type Cache interface {
	GetFileContent(path string) (io.ReadSeeker, error)
	GetMetaData(dedupe, id string) ([]string, error)
	Exists(path string) bool
	BasePath() string
}

// chroot return the absolute path of path but chrooted at root
func chroot(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Join(root, path[1:])
	}
	return filepath.Join(root, path)
}
