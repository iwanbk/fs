package main

import (
	"encoding/json"
	"log"

	"bazil.org/fuse/fs"
	"github.com/boltdb/bolt"
)

type FS struct {
	db       *bolt.DB
	root     map[string]json.RawMessage
	binStore string
}

var _ = fs.FS(&FS{})

func (f *FS) Root() (fs.Node, error) {
	var dst map[string]json.RawMessage
	if err := json.Unmarshal(f.root["/"], &dst); err != nil {
		log.Fatal(err)
	}

	n := &Dir{
		fs:     f,
		node:   dst,
		parent: "/",
	}
	return n, nil
}
