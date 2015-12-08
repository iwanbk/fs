package metadata

import (
	"encoding/json"
	"fmt"
	"github.com/Jumpscale/aysfs/utils"
	"path"
	"strings"
)

const (
	kvNodeTypeDir  = "DIR"
	kvNodeTypeFile = "FILE"
)

type KeyValueStore interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
}

type kvNodeDescriptorWrapper struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type kvNodeDirDescriptor struct {
	Name     string   `json:"name"`
	Children []string `json:"children"`
}

type kvNodeFileDescriptor struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Hash string `json:"hash"`
}

type kvBranch struct {
	name     string
	path     string
	parent   Node
	store    KeyValueStore
	children []string
}

func newKVBranch(store KeyValueStore, name string, path string, parent Node, children []string) Node {
	return &kvBranch{
		name:     name,
		path:     path,
		parent:   parent,
		store:    store,
		children: children,
	}
}

func (b *kvBranch) IsLeaf() bool {
	return false
}

func (b *kvBranch) Name() string {
	return b.name
}

func (b *kvBranch) Path() string {
	return b.path
}

func (b *kvBranch) Parent() Node {
	return b.parent
}

func (b *kvBranch) Children() map[string]Node {
	children := make(map[string]Node)
	root := b.Path()

	if b.children == nil {
		//load children if is not set.
		node, _ := b.get(root)
		if cnode, ok := node.(*kvBranch); ok {
			b.children = cnode.children
		}
	}

	for _, key := range b.children {
		_path := path.Join(root, key)
		child, err := b.get(_path)
		if err != nil {
			log.Error("Failed to load child: %s", err)
			continue
		}

		if child == nil {
			log.Error("No node found at '%s'", _path)
			continue
		}

		children[key] = child
	}

	return children
}

func (b *kvBranch) Search(_path string) Node {
	fullPath := path.Join(b.Path(), strings.TrimLeft(_path, PathSep))

	node, err := b.get(fullPath)
	if err != nil {
		return nil
	}

	return node
}

func (m *kvBranch) getDirNode(path string, raw json.RawMessage) (Node, error) {
	descriptor := kvNodeDirDescriptor{}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return nil, err
	}

	return newKVBranch(m.store, descriptor.Name, path, m, descriptor.Children), nil
}

func (m *kvBranch) getFileNode(raw json.RawMessage) (Node, error) {
	descriptor := kvNodeFileDescriptor{}
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return nil, err
	}

	return newLeaf(descriptor.Name, m, descriptor.Hash, descriptor.Size), nil
}

func (m *kvBranch) get(key string) (Node, error) {
	data, err := m.store.Get(key)
	if err != nil {
		return nil, err
	}

	if data == nil || len(data) == 0 {
		return nil, nil
	}

	wrapper := kvNodeDescriptorWrapper{}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	log.Debug("Found Node of type '%s' at '%s'", wrapper.Type, key)
	switch wrapper.Type {
	case kvNodeTypeDir:
		return m.getDirNode(key, wrapper.Data)
	case kvNodeTypeFile:
		return m.getFileNode(wrapper.Data)
	}

	return nil, fmt.Errorf("Unknown node type '%s'", wrapper.Type)
}

type kvMetadataImpl struct {
	*kvBranch
	store KeyValueStore
	base  string
}

func NewKVMetadata(base string, store KeyValueStore) Metadata {
	branch := &kvBranch{
		name:   "/",
		path:   "/",
		parent: nil,
		store:  store,
	}

	return &kvMetadataImpl{
		kvBranch: branch,
		store:    store,
		base:     base,
	}
}

func (m *kvMetadataImpl) set(k string, t string, o interface{}) error {
	data := map[string]interface{}{
		"type": t,
		"data": o,
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	log.Debug("Storing '%s'", bytes)

	return m.store.Set(k, bytes)
}

func (m *kvMetadataImpl) Purge() error {
	return nil
}

func (m *kvMetadataImpl) Index(line string) error {
	entry, err := ParseLine(m.base, line)
	if err == ignoreLine {
		return nil
	} else if err != nil {
		return err
	}
	parts := []string{"/"}
	parts = append(parts, strings.Split(entry.Path, PathSep)...)
	//_path := ""
	log.Debug("Indexing file '%s'", entry.Path)
	for i, part := range parts {
		_path := path.Join(parts[0 : i+1]...)
		_parentPath := path.Join(parts[0:i]...)

		log.Debug("Processing node '%s' parent '%s'", _path, _parentPath)
		if i == len(parts)-1 {
			//set the leaf node
			descriptor := kvNodeFileDescriptor{
				Name: part,
				Hash: entry.Hash,
				Size: entry.Size,
			}

			log.Debug("Setting leaf node '%s'", _path)
			if err := m.set(_path, kvNodeTypeFile, &descriptor); err != nil {
				return err
			}
		} else {
			node, err := m.get(_path)
			if err != nil {
				return err
			}
			//set current node itself.
			if node == nil {
				//just create the branch node.
				descriptor := kvNodeDirDescriptor{
					Name:     part,
					Children: []string{},
				}

				log.Debug("Setting intermediate node '%s'", _path)
				m.set(_path, kvNodeTypeDir, &descriptor)
			}
		}

		//update parent node.
		parent, err := m.get(_parentPath)
		if err != nil {
			return err
		}

		if parent == nil {
			continue
		}

		if pnode, ok := parent.(*kvBranch); ok {
			if utils.In(pnode.children, part) {
				continue
			}

			descriptor := kvNodeDirDescriptor{
				Name:     pnode.name,
				Children: append(pnode.children, part),
			}
			log.Debug("Updating parent node '%s'", _parentPath)
			m.set(_parentPath, kvNodeTypeDir, descriptor)
		} else {
			return fmt.Errorf("Invalid type found, expecting dir: %v", parent)
		}
	}

	return nil
}
