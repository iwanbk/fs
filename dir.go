package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type Dir struct {
	fs     *FS
	parent string
	name   string
	node   map[string]json.RawMessage
}

var _ = fs.Node(&Dir{})

func (d *Dir) Attr(a *fuse.Attr) {
	a.Mode = os.ModeDir | 0333
}

var _ = fs.HandleReadDirAller(&Dir{})

func hash(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	res := []fuse.Dirent{}

	for k, raw := range d.node {
		de := fuse.Dirent{
			Name: k,
		}

		var dst *FileInfo
		if err := json.Unmarshal(raw, &dst); err != nil {
			return nil, fuse.EIO
		}
		if dst.Filename != "" {
			de.Type = fuse.DT_File
		} else {
			de.Type = fuse.DT_Dir
		}
		res = append(res, de)
	}
	return res, nil
}

var _ = fs.NodeStringLookuper(&Dir{})

func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	raw, ok := d.node[name]
	if !ok {
		return nil, fuse.ENOENT
	}

	var fi *FileInfo
	if err := json.Unmarshal(raw, &fi); err != nil {
		return nil, fuse.EIO
	}
	if fi.Filename != "" {
		return &File{
			dir:  d,
			info: fi,
		}, nil
	}

	var dir map[string]json.RawMessage
	if err := json.Unmarshal(d.node[name], &dir); err != nil {
		log.Fatal(err)
	}
	return &Dir{
		fs:     d.fs,
		name:   name,
		parent: d.parent,
		node:   dir,
	}, nil

}
