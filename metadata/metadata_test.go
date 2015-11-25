package metadata

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

var lines = []string{
	"/opt/mongodb/bin/mongod|d7ca41fbf8cb8a03fc70d773c32ec8d2|23605576",
	"/opt/mongodb/bin/mongos|8e7100afca707b38c1d438a4be48d0b2|18354848",
	"/opt/mongodb/bin/mongo|71ae6457a07eb4cc69bead58e497cb07|11875136",
}

func TestNewMetadata(t *testing.T) {
	meta, err := NewMetadata("", lines)
	assert.NoError(t, err)
	assert.Implements(t, (*Node)(nil), meta)
}

func TestRootNode(t *testing.T) {
	meta, err := NewMetadata("", lines)

	assert.NoError(t, err)
	assert.Equal(t, "/", meta.Name())
	assert.Equal(t, "/", meta.Path())
	assert.Equal(t, nil, meta.Parent())
}

func TestRootNodeChildren(t *testing.T) {
	meta, err := NewMetadata("", lines)

	assert.NoError(t, err)
	children := meta.Children()
	assert.Len(t, children, 1)
	child := children["opt"]

	assert.NotNil(t, child)
	assert.Equal(t, "opt", child.Name())
	assert.Equal(t, "/opt", child.Path())
}

func TestRootNodeGrandChildren(t *testing.T) {
	meta, err := NewMetadata("", lines)

	assert.NoError(t, err)
	children := meta.Children()
	assert.Len(t, children, 1)
	child := children["opt"]

	grandchildren := child.Children()
	assert.Len(t, grandchildren, 1)
	grandchild := grandchildren["mongodb"]
	assert.NotNil(t, grandchild)
}

func TestSearch(t *testing.T) {
	meta, err := NewMetadata("", lines)
	assert.NoError(t, err)

	node := meta.Search("/opt/mongodb/bin")
	assert.NotNil(t, node)

	assert.False(t, node.IsLeaf())
}

func TestSearchChild(t *testing.T) {
	meta, err := NewMetadata("", lines)
	assert.NoError(t, err)

	node := meta.Search("/opt/mongodb")
	assert.NotNil(t, node)

	node = node.Search("/bin/mongod")
	assert.NotNil(t, node)

	assert.True(t, node.IsLeaf())
}

func TestSearchSelf(t *testing.T) {
	meta, err := NewMetadata("", lines)
	assert.NoError(t, err)

	node := meta.Search("/")
	assert.NotNil(t, node)

	assert.Equal(t, meta, node)
}

func TestLeaf(t *testing.T) {
	meta, err := NewMetadata("", lines)
	assert.NoError(t, err)

	node := meta.Search("/opt/mongodb/bin/mongod")
	assert.NotNil(t, node)

	assert.True(t, node.IsLeaf())

	leaf := node.(Leaf)
	assert.Equal(t, int64(23605576), leaf.Size())
	assert.Equal(t, "d7ca41fbf8cb8a03fc70d773c32ec8d2", leaf.Hash())
}

func TestPrefixing(t *testing.T) {
	meta, err := NewMetadata("/opt/mongodb", lines)
	assert.NoError(t, err)

	children := meta.Children()
	assert.Len(t, children, 1)

	node := meta.Search("/bin/mongod")
	assert.NotNil(t, node)

	assert.True(t, node.IsLeaf())

	leaf := node.(Leaf)
	assert.Equal(t, int64(23605576), leaf.Size())
	assert.Equal(t, "d7ca41fbf8cb8a03fc70d773c32ec8d2", leaf.Hash())
}