package metadata

import (
	"github.com/op/go-logging"
)

var (
	PathSep = "/"
	log     = logging.MustGetLogger("metadata")
)

type Node interface {
	Name() string
	Path() string
	Parent() Node
	Children() map[string]Node
	IsLeaf() bool
	Search(path string) Node
}

type Metadata interface {
	Node
	Index(line string) error
	Purge() error
}
