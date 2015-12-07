package metadata

import (
	"github.com/op/go-logging"
	"strings"
	"fmt"
)

var (
	PathSep = "/"
	log     = logging.MustGetLogger("metadata")
	ignoreLine = fmt.Errorf("Ignore Line")
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
}

type Entry struct {
	Path string
	Hash string
	Size int64
}

func ParseLine(base string, line string) (*Entry, error) {
	entry := Entry{}

	lineParts := strings.Split(line, "|")
	if len(lineParts) != 3 {
		return nil, fmt.Errorf("Wrong metadata line syntax '%s'", line)
	}

	path := lineParts[0]
	if strings.HasPrefix(path, base) {
		path = strings.TrimPrefix(path, base)
	} else {
		return nil, ignoreLine
	}

	//remove perfix / if exists.
	entry.Path = strings.TrimLeft(path, PathSep)
	entry.Hash = lineParts[1]
	count, err := fmt.Sscanf(lineParts[2], "%d", &entry.Size)
	if err != nil || count != 1 {
		return nil, fmt.Errorf("Invalid metadata line '%s' (%d, %s)", line, count, err)
	}

	return &entry, nil
}