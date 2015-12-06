package metadata

import (
	"fmt"
	"path"
	"strings"
)

type memMetadataImpl struct {
	Node
	base string
}

type memBranch struct {
	name     string
	parent   Node
	children map[string]Node
}

func newMemBranch(name string, parent Node) Node {
	return &memBranch{
		name:     name,
		parent:   parent,
		children: make(map[string]Node),
	}
}

func (b *memBranch) IsLeaf() bool {
	return false
}

func (b *memBranch) Name() string {
	return b.name
}

func (b *memBranch) Path() string {
	if b.parent == nil {
		return b.name
	}

	return path.Join(b.parent.Path(), b.name)
}

func (b *memBranch) Parent() Node {
	return b.parent
}

func (b *memBranch) Children() map[string]Node {
	return b.children
}

func (b *memBranch) Search(path string) Node {
	path = strings.TrimLeft(path, PathSep)
	var node Node = b
	if path == "" {
		return node
	}
	for _, part := range strings.Split(path, PathSep) {
		if child, ok := node.Children()[part]; ok {
			node = child
		} else {
			return nil
		}
	}

	return node
}

func NewMemMetadata(base string, lines []string) (Metadata, error) {
	root := newMemBranch("/", nil)

	meta := &memMetadataImpl{Node: root, base: base}
	for _, line := range lines {
		if err := meta.Index(line); err != nil {
			return nil, err
		}
	}

	return meta, nil
}

func (m *memMetadataImpl) Index(line string) error {
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
			children[part] = newLeaf(part, node, hash, size)
			//loop will break here.
		} else {
			//branch node
			if child, ok := children[part]; ok {
				node = child
			} else {
				node = newMemBranch(part, node)
				children[part] = node
			}
		}
	}
	return nil
}

func (m *memMetadataImpl) Purge() error {
	root := newMemBranch("/", nil)
	m.Node = root
	return nil
}
