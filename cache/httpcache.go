package cache

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/Jumpscale/aysfs/utils"
)

type httpCache struct {
	addr   string
	dedupe string
}

func NewHTTPCache(addr string, dedupe string) Cache {
	return &httpCache{
		addr:   addr,
		dedupe: dedupe,
	}
}

func (f *httpCache) String() string {
	return fmt.Sprintf("%s/%s", f.addr, f.dedupe)
}

func (f *httpCache) Open(path string) (io.ReadSeeker, error) {
	url := fmt.Sprintf("%s/%s/files/%s", f.addr, f.dedupe, path)
	resp, err := http.Get(url)
	if err != nil {
		// log.Printf("can't get file from %s: %v\n", url, err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("can't get file from %s: http status code is %d\n", url, resp.StatusCode)
	}

	return utils.NewReadSeeker(resp.Body), nil
}

func (f *httpCache) GetMetaData(id string) ([]string, error) {
	url := fmt.Sprintf("%s/%s/md/%s.flist", f.addr, f.dedupe, id)
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
	resp, err := http.Head(url)
	if err != nil {
		return false
	}

	if resp.StatusCode != http.StatusOK {
		return false
	}

	return true
}

func (f *httpCache) BasePath() string {
	u, e := url.Parse(f.addr)
	if e != nil {
		return ""
	}

	u.Path = path.Join(u.Path, f.dedupe)
	return u.String()
}
