package metadata

import (
	"fmt"
	"github.com/op/go-logging"
	"strings"
)

var (
	PathSep = "/"
	log = logging.MustGetLogger("metadata")
)

type Node interface{
	Name() string
	Path() string
	Parent() Node
	Children() map[string]Node
	IsLeaf() bool
	Search(path string) Node
}

type Leaf interface {
	Node
	Hash() string
	Size() int64
}

type Metadata Node

func NewMetadata(base string, lines []string) (Metadata, error) {
	root := NewBranch("/", nil)

	for _, line := range lines {
		lineParts := strings.Split(line, "|")
		if len(lineParts) != 3 {
			log.Error("Wrong metadata line syntax '%s'", line)
			continue
		}
		path := lineParts[0]
		if strings.HasPrefix(path, base) {
			path = strings.TrimPrefix(path, base)
		} else {
			continue
		}
		//remove perfix / if exists.
		path = strings.TrimLeft(path, PathSep)
		hash := lineParts[1]
		var size int64
		count, err := fmt.Sscanf(lineParts[2], "%d", &size)
		if err != nil || count != 1{
			log.Error("Invalid metadata line '%s' (%d, %s)", line, count, err)
			continue
		}

		parts := strings.Split(path, PathSep)
		node := root
		for i, part := range parts {
			if node.IsLeaf() {
				return nil, fmt.Errorf("Invalid line '%s' found nested leafs", line)
				break
			}

			children := node.Children()
			if i == len(parts) - 1 {
				//add the leaf node.
				children[part] = NewLeaf(part, node, hash, size)
				//loop will break here.
			} else {
				//branch node
				if child, ok := children[part]; ok {
					node = child
				} else {
					node = NewBranch(part, node)
					children[part] = node
				}
			}
		}

	}

	return Metadata(root), nil
}