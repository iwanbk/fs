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
	entry, err := ParseLine(m.base, line)
	if err == ignoreLine {
		return nil
	} else if err != nil {
		return err
	}

	parts := strings.Split(entry.Path, PathSep)
	node := m.Node
	for i, part := range parts {
		if node.IsLeaf() {
			return fmt.Errorf("Invalid line '%s' found nested leafs", line)
		}

		children := node.Children()
		if i == len(parts)-1 {
			//add the leaf node.
			children[part] = newLeaf(part, node, entry.Hash, entry.Size)
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
