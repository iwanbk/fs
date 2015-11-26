package cache

import (
	"io"
	"fmt"
	"net/http"
	"net/url"
	"bufio"
	"path"
)

type httpCache struct {
	addr   string
	dedupe string
}

func NewHTTPCache(addr string, dedupe string) Cache {
	return &httpCache{
		addr: addr,
		dedupe: dedupe,
	}
}

func (f *httpCache) Purge() error {
	//not supported by this cache.
	return nil
}

//TODO this must be change (the body reader) to be more efficient than this impl
//since now we have to read the entire file (that can be big) before we can read data.
type bodyReader struct {
	body io.ReadCloser
}

func (reader *bodyReader) Read(p []byte) (int, error) {
	log.Debug("Reading from HTTP cache...")
	return reader.body.Read(p)
}

func (reader *bodyReader) Close() error {
	return reader.body.Close()
}

func (reader *bodyReader) Seek(offset int64, whence int) (int64, error){
	return 0, fmt.Errorf("Not implemented")
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

	return &bodyReader{
		body: resp.Body,
	}, nil
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