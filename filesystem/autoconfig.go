package filesystem

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jumpscale/aysfs/cache"
)

//DiscoverMetadata Walk over the directory pointed by root to discover .flist file and add them to the metadata list.
func (f *FS) DiscoverMetadata(root string) error {
	return discoverMetadata(f, root)
}

func discoverMetadata(fs *FS, root string) error {
	readFList := func(path string) ([]string, error) {
		file, err := os.Open(path)
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

	walkFunc := func(path string, info os.FileInfo, err error) error {
		if info.IsDir() || !strings.HasSuffix(path, ".flist") {
			return nil
		}

		log.Debug("Flist file found :%s\n", path)
		partialMetadata, err := readFList(path)
		if err != nil {
			//TODO check if we want to stop walking or not
			log.Errorf("Error while walking metadata dir at %s: %v\n", path, err)
			return err
		}

		for _, line := range partialMetadata {
			fs.metadata.Index(line)
		}

		return nil
	}

	if err := filepath.Walk(root, walkFunc); err != nil {
		log.Errorf("Error auto discover metadata: %v\n", err)
		return err
	}

	return nil
}

//AutoConfig tries to autoconfigure the caches and stores by checking predefined location
//in the local filesystem and predefined hostname
func (f *FS) AutoConfigCaches() {
	f.searchDefaultCache()
	f.searchDefaultStore()
}

func (f *FS) searchLocalStore() {
	localRoot := "/mnt/ays/cachelocal"
	_, err := os.Open(localRoot)
	if err == nil {
		f.cache.AddLayer(cache.NewFSCache(localRoot, "dedupe", true))
	}
}

// SearchDefaultStore tries to auto finds AYS Stores
// check if it can find aysmaster1(2...) as hostname & do tcp port test on port 443
// if tcp port test succeeds
// do a https test to download predefined ping file
// if all ok the nuse these as http master
// if not found
// check /mnt/ays/master or /mnt/ays/master1 or /mnt/ays/master2 exists
// use those as masters
func (f *FS) searchDefaultStore() {
	path := "/mnt/ays/master"
	_, err := os.Open(path)
	if err == nil {
		f.cache.AddLayer(cache.NewFSCache(path, "dedupe", false))
	}

	for i := 0; i < 5; i++ {
		fullPath := fmt.Sprintf("%s%d", path, i)
		_, err := os.Open(fullPath)
		if err == nil {
			f.cache.AddLayer(cache.NewFSCache(fullPath, "dedupe", false))
		}
	}

	hostname := "aysmaster"
	if err := testListen(fmt.Sprintf("%s:443", hostname)); err == nil {
		// TODO download test file
		httpAddr := fmt.Sprintf("https://%s/master", hostname)
		f.cache.AddLayer(cache.NewHTTPCache(httpAddr, "dedupe"))
	}

	for i := 0; i < 5; i++ {
		addr := fmt.Sprintf("%s%d:443", hostname, i)

		if err := testListen(addr); err == nil {
			// TODO download test file
			httpAddr := fmt.Sprintf("https://%s/master", addr)
			f.cache.AddLayer(cache.NewHTTPCache(httpAddr, "dedupe"))
		}

	}
}

//SearchDefaultCache tries to auto finds AYS Caches
// check if it can find ayscache1(2...) as hostname & do tcp port test on port 9990
// if tcp port test succeeds use these as http cache
// otherwise check /mnt/ays/cachelan or /mnt/ays/cachelan1 or /mnt/ays/cachelan2 exists
// use those as caches
func (f *FS) searchDefaultCache() {
	path := "/mnt/ays/cachelan"
	_, err := os.Open(path)
	if err == nil {
		f.cache.AddLayer(cache.NewFSCache(path, "dedupe", false))
	}

	for i := 0; i < 5; i++ {
		fullPath := fmt.Sprintf("%s%d", path, i)
		_, err := os.Open(fullPath)
		if err == nil {
			f.cache.AddLayer(cache.NewFSCache(fullPath, "dedupe", false))
		}
	}

	hostname := "ayscache"
	if err := testListen(fmt.Sprintf("%s:9990", hostname)); err == nil {
		httpAddr := fmt.Sprintf("https://%s/cache", hostname)
		f.cache.AddLayer(cache.NewHTTPCache(httpAddr, "dedupe"))
	}

	for i := 0; i < 5; i++ {
		addr := fmt.Sprintf("%s%d:9990", hostname, i)

		if err := testListen(addr); err == nil {
			httpAddr := fmt.Sprintf("https://%s/cache", addr)
			f.cache.AddLayer(cache.NewHTTPCache(httpAddr, "dedupe"))
		}
	}
}

//testListen try to connec to addr, if connection fail an error is returned otherwise nil is returned
// addr should have the form host:port.
func testListen(addr string) error {
	_, err := net.DialTimeout("tcp", addr, time.Second*3)
	if err != nil {
		log.Debug("Can't connect to %s: %s", addr, err)
		return err
	}
	return nil
}
