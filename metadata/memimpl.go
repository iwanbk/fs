package metadata

import (
	"fmt"
	"strings"
)

type metadataImpl struct {
	Node
	base string
}

func NewMetadata(base string, lines []string) (Metadata, error) {
	root := NewBranch("/", nil)

	meta := &metadataImpl{Node: root, base: base}
	for _, line := range lines {
		if err := meta.Index(line); err != nil {
			return nil, err
		}
	}

	return meta, nil
}

func (m *metadataImpl) Index(line string) error {
	lineParts := strings.Split(line, "|")
	if len(lineParts) != 3 {
		return fmt.Errorf("Wrong metadata line syntax '%s'", line)
	}

	path := lineParts[0]
	if strings.HasPrefix(path, m.base) {
		path = strings.TrimPrefix(path, m.base)
	} else {
		return nil
	}

	//remove perfix / if exists.
	path = strings.TrimLeft(path, PathSep)
	hash := lineParts[1]
	var size int64
	count, err := fmt.Sscanf(lineParts[2], "%d", &size)
	if err != nil || count != 1 {
		return fmt.Errorf("Invalid metadata line '%s' (%d, %s)", line, count, err)
	}

	parts := strings.Split(path, PathSep)
	node := m.Node
	for i, part := range parts {
		if node.IsLeaf() {
			return fmt.Errorf("Invalid line '%s' found nested leafs", line)
		}

		children := node.Children()
		if i == len(parts)-1 {
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
	return nil
}
