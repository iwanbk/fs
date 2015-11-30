package metadata

import (
	"path"
	"sync"

	"bazil.org/fuse/fs"
)

type Leaf interface {
	Node
	sync.Locker
	Hash() string
	Size() int64
	FuseNode() fs.Node
	SetFuseNode(node fs.Node)
}

type leaf struct {
	parent Node
	name   string
	hash   string
	size   int64

	lock     sync.Mutex
	fuseNode fs.Node
}

func NewLeaf(name string, parent Node, hash string, size int64) Node {
	return &leaf{
		name:   name,
		parent: parent,
		hash:   hash,
		size:   size,
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

func (l *leaf) Size() int64 {
	return l.size
}

func (l *leaf) Search(path string) Node {
	return nil
}

func (l *leaf) FuseNode() fs.Node {
	return l.fuseNode
}

func (l *leaf) SetFuseNode(node fs.Node) {
	l.fuseNode = node
}

func (l *leaf) Lock() {
	l.lock.Lock()
}

func (l *leaf) Unlock() {
	l.lock.Unlock()
}
