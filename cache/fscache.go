package cache

import (
	"path/filepath"
	"io"
	"fmt"
	"os"
	"bufio"
	"path"
)

type fsCache struct {
	root   string
	dedupe string
}

func NewFSCache(root string, dedupe string) Cache {
	return &fsCache{
		root: root,
		dedupe: dedupe,
	}
}

func (f *fsCache) Purge() error {
	if err := os.RemoveAll(f.root); err != nil {
		return err
	}

	if err := os.MkdirAll(f.root, 0660); err != nil {
		return err
	}

	return nil
}

func (f *fsCache) GetFileContent(path string) (io.ReadSeeker, error) {
	chrootPath := chroot(f.root, filepath.Join(f.dedupe, "files", path))
	file, err := os.Open(chrootPath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fsCache) GetMetaData(dedup, id string) ([]string, error) {
	path := filepath.Join(f.dedupe, "md", fmt.Sprintf("%s.flist", id))
	chrootPath := chroot(f.root, path)
	file, err := os.Open(chrootPath)
	if err != nil {
		return nil, err
	}

	metadata := []string{}
	scanner := bufio.NewScanner(file)
	var line string
	for scanner.Scan() {
		line = scanner.Text()
		if err := scanner.Err(); err != nil {
			// log.Printf("reading %s: %s", name, err)
			return nil, err
		}
		metadata = append(metadata, line)
	}

	return metadata, nil
}

func (f *fsCache) Exists(path string) bool {
	_, err := os.Stat(chroot(f.root, path))
	if err != nil {
		return false
	}
	return true
}

func (f *fsCache) BasePath() string {
	return path.Join(f.root, f.dedupe)
}