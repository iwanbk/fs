package metadata

import (
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"path"
	"strings"
)

type boltMetadataImpl struct {
	Node
	db   *bolt.DB
	base string
}

type boltBranch struct {
	name   string
	parent Node
	db     *bolt.DB
}

func newBoltBrach(name string, db *bolt.DB, parent Node) Node {
	return &boltBranch{
		name:   name,
		parent: parent,
		db:     db,
	}
}

func (b *boltBranch) Name() string {
	return b.name
}

func (b *boltBranch) Path() string {
	return path.Join(b.getPathParts()...)
}

func (b *boltBranch) Parent() Node {
	return b.parent
}

func (b *boltBranch) getPathParts() []string {
	var node Node = b
	parts := make([]string, 0)
	for node != nil {
		parts = append(parts, node.Name())
		node = node.Parent()
	}
	reversed := make([]string, len(parts))
	for i := 0; i < len(parts); i++ {
		reversed[i] = parts[len(parts)-i-1]
	}

	return reversed
}

func (b *boltBranch) getCurrentBucket(t *bolt.Tx) *bolt.Bucket {
	var bucket *bolt.Bucket
	for i, part := range b.getPathParts() {
		if i == 0 {
			bucket = t.Bucket([]byte(part))
		} else {
			bucket = bucket.Bucket([]byte(part))
		}

		if bucket == nil {
			return nil
		}
	}

	return bucket
}

func (b *boltBranch) Children() map[string]Node {
	nodes := make(map[string]Node)
	b.db.View(func(t *bolt.Tx) error {
		bucket := b.getCurrentBucket(t)
		if bucket == nil {
			return fmt.Errorf("Invalid path")
		}

		cursor := bucket.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			name := string(key)
			if len(value) == 0 {
				//that's is a sub - bucket
				node := newBoltBrach(name, b.db, b)
				nodes[name] = node
			} else {
				//try loading this into map
				var leafData map[string]interface{}
				err := json.Unmarshal(value, &leafData)
				if err != nil {
					log.Error("Failed to load leaf data '%s/%s': %s", b.Path(), name, err)
					return err
				}
				node := newLeaf(name, b, leafData["hash"].(string), int64(leafData["size"].(float64)))
				nodes[name] = node
			}
		}

		return nil
	})

	return nodes
}

func (b *boltBranch) IsLeaf() bool {
	return false
}

func (b *boltBranch) Search(path string) Node {
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

func NewBoltMetadata(base string, dbpath string) (Metadata, error) {
	db, err := bolt.Open(dbpath, 0600, nil)
	if err != nil {
		return nil, err
	}

	root := &boltBranch{
		name: "/",
		db:   db,
	}

	meta := &boltMetadataImpl{
		db:   db,
		Node: root,
		base: base,
	}

	return meta, nil
}

func (m *boltMetadataImpl) Index(line string) error {
	entry, err := ParseLine(m.base, line)
	if err == ignoreLine {
		return nil
	} else if err != nil {
		return err
	}

	parts := strings.Split(entry.Path, PathSep)
	go m.db.Batch(func(t *bolt.Tx) error {
		bucket, err := t.CreateBucketIfNotExists([]byte(m.Name()))
		if err != nil {
			return err
		}

		for i, part := range parts {
			if i == len(parts)-1 {
				//add the leaf node.
				data := map[string]interface{}{
					"hash": entry.Hash,
					"size": entry.Size,
				}
				bytes, err := json.Marshal(data)
				if err != nil {
					return err
				}
				log.Debug("Bolt meta: creating leaf on '%s' '%s'", parts, data)
				return bucket.Put([]byte(part), bytes)
				//loop will break here.
			} else {
				//branch node
				log.Debug("Bolt meta: creating branch on '%s'", parts[:i])
				bucket, err = bucket.CreateBucketIfNotExists([]byte(part))
				if err != nil {
					return err
				}
			}
		}

		return nil
	})

	return nil
}
