package ro

import (
	"fmt"
	"os"

	"github.com/Jumpscale/aysfs/metadata"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type Dir interface {
	fs.Node
	fs.HandleReadDirAller
	fs.NodeStringLookuper
	fmt.Stringer
	Parent() Dir
	FS() *FS
}

type dirImpl struct {
	fs     *FS
	parent Dir
	info   metadata.Node
}

func newDir(fs *FS, parent Dir, node metadata.Node) Dir {
	return &dirImpl{
		fs:     fs,
		parent: parent,
		info:   node,
	}
}

func (d *dirImpl) FS() *FS {
	return d.fs
}

func (d *dirImpl) Parent() Dir {
	return d.parent
}

func (d *dirImpl) String() string {
	return d.info.Path()
}

func (d *dirImpl) searchEntry(name string) (fs.Node, bool, error) {
	log.Debug("Directory '%s' search entry '%s'", d.String(), name)

	// look into the metadata for the entry
	childNode := d.info.Search(name)

	if childNode == nil {
		return nil, false, fuse.ENOENT
	}

	if childNode.IsLeaf() {
		//file
		fileNode := childNode.(metadata.Leaf)
		fsNode := d.fs.factory.GetFile(d.fs, d, fileNode)
		return fsNode, false, nil
	} else {
		return d.fs.factory.GetDir(d.fs, d, childNode), false, nil
	}
}

var _ = fs.Node(&dirImpl{})

func (d *dirImpl) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Mode = os.ModeDir | 0555
	return nil
}

var _ = fs.HandleReadDirAller(&dirImpl{})

func (d *dirImpl) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Debug("ReadDirAll '%s' entries", d)

	//block until fs allows routine to access.
	d.fs.access()

	var (
		results []fuse.Dirent
	)

	path := d.String()

	dirNode := d.fs.metadata.Search(d.String())
	if dirNode == nil {
		log.Debug("Directory '%s' not found in meta", path)
		return results, nil
	}

	log.Debug("Found (%d) child in dir '%s'", len(dirNode.Children()), dirNode.Path())

	for _, child := range dirNode.Children() {
		de := fuse.Dirent{}
		if child.IsLeaf() {
			//file
			fileNode := child.(metadata.Leaf)
			// prepare object for fuse
			de.Type = fuse.DT_File
			de.Name = fileNode.Name()
		} else {
			// prepare object for fuse
			de.Name = child.Name()
			de.Type = fuse.DT_Dir
		}

		results = append(results, de)
	}

	return results, nil
}

var _ = fs.NodeStringLookuper(&dirImpl{})

func (d *dirImpl) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Debug("Directory '%v' lookup on '%s'", d, name)

	d.fs.access()

	defer func() {
		if r := recover(); r != nil {
			log.Fatal(r)
		}
	}()
	node, _, err := d.searchEntry(name)
	if err != nil {
		return nil, err
	}

	return node, nil
}
