package cache

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

type fsCache struct {
	root   string
	dedupe string
	purge  bool
}

func NewFSCache(root string, dedupe string, purge bool) Cache {
	return &fsCache{
		root:   root,
		dedupe: dedupe,
		purge:  purge,
	}
}

func (f *fsCache) String() string {
	return fmt.Sprintf("file://%s/%s [%t]", f.root, f.dedupe, f.purge)
}

func (f *fsCache) Purge() error {
	if !f.purge {
		return nil
	}

	if err := os.RemoveAll(f.root); err != nil {
		return err
	}

	if err := os.MkdirAll(f.root, 0660); err != nil {
		return err
	}

	return nil
}

func (f *fsCache) DeDupe(binpath string, file io.ReadSeeker) error {

	path := filepath.Join(f.BasePath(), "files", binpath)
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(path), 0660)

		outFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		defer outFile.Close()

		if err != nil {
			log.Error("error while saving %s into local cache. open error %s\n", path, err)
			os.Remove(path)
			return err
		}

		// move to begining of the file to be sure to copy all the data
		_, err = file.Seek(0, 0)
		if err != nil {
			os.Remove(path)
			return err
		}

		_, err = io.Copy(outFile, file)
		if err != nil {
			log.Error("error while saving %s into local cache. copy error %s\n", path, err)
			os.Remove(path)
			return err
		}
	}
	return nil
}

func (f *fsCache) SetMetaData([]string) error {
	return fmt.Errorf("Not Implemented")
}

func (f *fsCache) Open(path string) (io.ReadSeeker, error) {
	chrootPath := chroot(f.root, filepath.Join(f.dedupe, "files", path))
	file, err := os.Open(chrootPath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *fsCache) GetMetaData(id string) ([]string, error) {
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
