package metadata

import (
	"fmt"
	"github.com/op/go-logging"
	"strings"
	//"os"
	"os"
)

var (
	log = logging.MustGetLogger("metadata")
)

type Node interface{
	Name() string
	Path() string
	Parent() Node
	Children() map[string]Node
	IsLeaf() bool
}

type Leaf interface {
	Node
	Hash() string
	Size() int
}

type Metadata interface {
	Node
	Search(path string) Node
}

type metadataImpl struct {
	Node
}

func NewMetadata(base string, lines []string) (Metadata, error) {
	root := NewBranch("/", nil)

	for _, line := range lines {
		lineParts := strings.Split(line, "|")
		if len(lineParts) != 3 {
			log.Error("Wrong metadata line syntax '%s'", line)
			continue
		}
		path := strings.TrimLeft(lineParts[0], string(os.PathSeparator))
		hash := lineParts[1]
		var size int
		count, err := fmt.Sscanf(lineParts[2], "%d", &size)
		if err != nil || count != 1{
			log.Error("Invalid metadata line '%s' (%d, %s)", line, count, err)
			continue
		}

		parts := strings.Split(path, "/")
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

	return &metadataImpl{root}, nil
}

func (meta *metadataImpl) Search(path string) Node {
	path = strings.TrimLeft(path, string(os.PathSeparator))
	var node Node = meta
	for _, part := range strings.Split(path, string(os.PathSeparator)) {
		if child, ok := node.Children()[part]; ok {
			node = child
		} else {
			return nil
		}
	}

	return node
}