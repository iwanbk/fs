package stor

import (
	"net/url"
	"io"
	"github.com/ipfs/go-ipfs-api"
)

type ipfsStor struct {
	shell *shell.Shell
}

func NewIPFSStor(u *url.URL) (Stor, error) {
	us := url.URL{
		Scheme: "http",
		Host: u.Host,
		Path: u.Path,
	}

	return &ipfsStor{
		shell: shell.NewShell(us.String()),
	}, nil
}

func (s *ipfsStor) Get(hash string) (io.ReadCloser, error) {
	return s.shell.Cat(hash)
}