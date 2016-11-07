package stor

import (
	"io"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("stor")
)

type Stor interface {
	Get(key string) (io.ReadCloser, error)
}
