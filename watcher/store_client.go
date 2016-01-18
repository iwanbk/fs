package watcher

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
)

var storeClient *StoreClient

type StoreClient struct {
	Addr string
}

func NewStoreClient(addr string) *StoreClient {
	return &StoreClient{addr}
}

func (s *StoreClient) PutFile(namespace string, r io.Reader) error {
	urlRaw := fmt.Sprintf("%s/%s", s.Addr, namespace)
	endpoint, err := url.Parse(urlRaw)
	if err != nil {
		log.Errorf("Error parsing url (%v) : %v", urlRaw, err)
		return err
	}

	resp, err := http.Post(endpoint.EscapedPath(), "application/octet-stream", r)
	if err != nil {
		log.Errorf("Error during uploading of file: %v", err)
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		err := fmt.Errorf("Error during uploading of file, status code = %d", resp.StatusCode)
		log.Errorf(err.Error())
		return err
	}

	return nil
}
