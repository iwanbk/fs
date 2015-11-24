package metadata

import (
	"path"
)

type leaf struct {
	parent Node
	name string
	hash string
	size int
}

func NewLeaf(name string, parent Node, hash string, size int) Node {
	return &leaf{
		name: name,
		parent: parent,
		hash: hash,
		size: size,
	}
}

func (l *leaf) IsLeaf() bool {
	return true
}

func (l *leaf) Name() string {
	return l.name
}

func (l *leaf) Path() string {
	if l.parent == nil {
		return l.name
	} else {
		return path.Join(l.parent.Path(), l.name)
	}
}

func (l *leaf) Parent() Node {
	return l.parent
}

func (l *leaf) Children() map[string]Node {
	return nil
}

func (l *leaf) Hash() string {
	return l.hash
}

func (l *leaf) Size() int {
	return l.size
}