package storage

import (
	"github.com/ipfs/go-ipfs-api"
	"io"
	"net/url"
)

type ipfsStor struct {
	shell *shell.Shell
}

func NewIPFSStorage(u *url.URL) (Storage, error) {
	us := url.URL{
		Scheme: "http",
		Host:   u.Host,
		Path:   u.Path,
	}

	return &ipfsStor{
		shell: shell.NewShell(us.String()),
	}, nil
}

func (s *ipfsStor) Get(hash string) (io.ReadCloser, error) {
	return s.shell.Cat(hash)
}
