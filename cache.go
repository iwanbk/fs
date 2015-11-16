package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"
)

type cacher interface {
	GetFileContent(path string) (io.ReadSeeker, error)
	GetMetaData(dedupe, id string) ([]string, error)
	Exists(path string) bool
}

type fsCache struct {
	root   string
	dedupe string
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
	// metadata, err := ioutil.ReadAll(file)
	// if err != nil {
	// 	log.Printf("can't read %s: %v\n", chrootPath, err)
	// 	return nil, err
	// }

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

// chroot return the absolute path of path but chrooted at root
func chroot(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Join(root, path[1:])
	}
	return filepath.Join(root, path)
}

type httpCache struct {
	addr   string
	dedupe string
}

func (f *httpCache) GetFileContent(path string) (io.ReadSeeker, error) {
	url := fmt.Sprintf("%s/%s/files/%s", f.addr, f.dedupe, path)
	resp, err := http.Get(url)
	if err != nil {
		// log.Printf("can't get file from %s: %v\n", url, err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("can't get file from %s: http status code is %d\n", url, resp.StatusCode)
	}

	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// log.Printf("can't read response from %s: %v\n", url, err)
		return nil, err
	}
	return bytes.NewReader(content), nil
}

func (f *httpCache) GetMetaData(dedupe, id string) ([]string, error) {
	url := fmt.Sprintf("%s/%s/md/%s.flist", f.addr, dedupe, id)
	resp, err := http.Get(url)
	if err != nil {
		// log.Printf("can't get file from %s: %v\n", url, err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("can't get file from %s: http status code is %d\n", url, resp.StatusCode)
	}

	defer resp.Body.Close()
	metadata := []string{}
	scanner := bufio.NewScanner(resp.Body)
	var line string
	for scanner.Scan() {
		line = scanner.Text()
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		metadata = append(metadata, line)
	}

	return metadata, nil
}

func (f *httpCache) Exists(path string) bool {
	url := fmt.Sprintf("%s/%s", f.addr, path)
	resp, err := http.Get(url)
	if err != nil {
		// log.Printf("can't get file from %s: %v\n", url, err)
		return false
	}
	if resp.StatusCode != http.StatusOK {
		return false
	}

	return true
}

type boltCache struct {
	db *bolt.DB
}

func (f *boltCache) GetMetaData(dedupe, id string) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *boltCache) GetFileContent(path string) (io.ReadSeeker, error) {
	return lazyLoadFromBolt(f.db, []byte(path))
}

func (f *boltCache) Exists(path string) bool {
	return true
}
