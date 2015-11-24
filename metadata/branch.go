package metadata
import "path"

type branch struct {
	name string
	parent Node
	children map[string]Node
}

func NewBranch(name string, parent Node) Node {
	return &branch{
		name: name,
		parent: parent,
		children: make(map[string]Node),
	}
}

func (b *branch) IsLeaf() bool {
	return false
}

func (b *branch) Name() string {
	return b.name
}

func (b *branch) Path() string {
	if b.parent == nil {
		return b.name
	} else {
		return path.Join(b.parent.Path(), b.name)
	}
}

func (b *branch) Parent() Node {
	return b.parent
}

func (b *branch) Children() map[string]Node {
	return b.children
}