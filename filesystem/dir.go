package filesystem

import (
	"fmt"
	"os"
	"path"
	"strings"
	"github.com/Jumpscale/aysfs/metadata"

	"golang.org/x/net/context"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type dir struct {
	fs     *FS
	parent *dir
	name   string
}

func (d *dir) String() string {
	if d.parent == nil {
		return d.name
	} else {
		return path.Join(d.parent.String(), d.name)
	}

}
//Abs return the absolute path of the directory pointed by d
func (d *dir) Abs() string {
	var path string
	if d.name == "/" {
		return "/"
	}

	var name string
	for d != nil && d.name != "/" {
		name = d.name
		if strings.Contains(d.name, "|") {
			ss := strings.Split(d.name, "|")
			name = ss[0]
		}
		path = fmt.Sprintf("/%s%s", name, path)
		d = d.parent
	}
	return path
}

func (d *dir) dbKey() []byte {
	return []byte(fmt.Sprintf("dir:%s", d.Abs()))
}

func (d *dir) searchEntry(name string) (fs.Node, bool, error) {
	log.Debug("Directory search entry '%s'", name)

	// look into the metadata for the entry
	dirNode := d.fs.metadata.Search(d.Abs())
	childNode := dirNode.Children()[name]
	if childNode == nil {
		return nil, false, fuse.ENOENT
	}

	if childNode.IsLeaf() {
		//file
		fileNode := childNode.(metadata.Leaf)
		return &file{
			dir:  d,
			info: &fileInfo{
				Size: fileNode.Size(),
				Hash: fileNode.Hash(),
				Filename: fileNode.Name(),
			},
		}, false, nil
	} else {
		return &dir{
			fs:     d.fs,
			name:   name,
			parent: d,
		}, false, nil
	}

	return nil, false, fuse.ENOENT
}

var _ = fs.Node(&dir{})

func (d *dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Mode = os.ModeDir | 0555
	return nil
}

var _ = fs.HandleReadDirAller(&dir{})

func (d *dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Debug("ReadDirAll '%s' entries", d)

	var (
		results []fuse.Dirent
	)

	log.Debug("ReadDirAll found no results in DB, load from meta (%s)", d.Abs())
	dirNode := d.fs.metadata.Search(d.Abs())
	if dirNode == nil {
		log.Debug("Directory '%s' not found in meta", d.Abs())
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

var _ = fs.NodeStringLookuper(&dir{})

func (d *dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Debug("Directory '%v' lookup on '%s'", d, name)
	node, _, err := d.searchEntry(name)
	if err != nil {
		return nil, err
	}

	return node, nil
}
