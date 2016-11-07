package stor

import (
	"net/url"
	"io"
	"fmt"
	"path"
	"net/http"
	"io/ioutil"
)

type aydoStor struct {
	baseURL string
	client *http.Client
}

func NewAydoStor(u *url.URL) (Stor, error) {
	if u.Scheme != "aydo" {
		return nil, fmt.Errorf("invalid scheme, expecting URL of format aydo://uname:password@host/store/<namespace>")
	}

	us := url.URL{
		Scheme: "http",
		User: u.User,
		Path: u.Path,
	}

	return &aydoStor{
		client: &http.Client{},
		baseURL: us.String(),
	}, nil
}

func (s *aydoStor) Get(hash string) (io.ReadCloser, error) {
	u := path.Join(s.baseURL, hash)
	log.Info("Downloading: %s", u)

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Accept", "application/brotli")

	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		body, _ := ioutil.ReadAll(response.Body)
		return nil, fmt.Errorf("invalid response from stor (%d): %s", response.StatusCode, body)
	}

	return response.Body, nil
}
