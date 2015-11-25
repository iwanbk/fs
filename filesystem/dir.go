package filesystem

import (
	"encoding/gob"
	"fmt"
	"os"
	//"path/filepath"
	"path"
	"strings"
	"github.com/Jumpscale/aysfs/database"
	"github.com/Jumpscale/aysfs/metadata"
	"github.com/boltdb/bolt"

	"golang.org/x/net/context"

	"bytes"

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
//  var node fs.Node
	log.Debug("Directory search entry '%s'", name)

//	err := d.fs.db.View(func(tx *bolt.Tx) error {
//		bucket := tx.Bucket([]byte("main"))
//		if bucket == nil {
//			return fuse.ENOENT
//		}
//		raw := bucket.Get([]byte(filepath.Join(d.Abs(), name)))
//		if raw == nil {
//			return fuse.ENOENT
//		}
//
//		if string(raw[:3]) == "dir" {
//			node = &dir{
//				fs:     d.fs,
//				name:   name,
//				parent: d,
//			}
//			return nil
//		}
//
//		fi, err := newFileInfo(fmt.Sprintf("%s|%s", name, string(raw)))
//		if err != nil {
//			return err
//		}
//		node = &file{
//			dir:  d,
//			info: fi,
//		}
//
//		return nil
//	})
//
//	// Entry found
//	if err == nil {
//		log.Debug("Entry '%s' found", node)
//		return node, true, nil
//	}
//
//	// error which is not just a not found error
//	if err != nil && err != fuse.ENOENT {
//		return nil, false, err
//	}

	//log.Debug("Entry '%s' NOT found", name)
	// look into the metadata for the entry
	dirnode := d.fs.metadata.Search(d.Abs())
	metanode := dirnode.Children()[name]
	if metanode == nil {
		return nil, false, fuse.ENOENT
	}

	if metanode.IsLeaf() {
		//file
		fnode := metanode.(metadata.Leaf)
		return &file{
			dir:  d,
			info: &fileInfo{
				Size: fnode.Size(),
				Hash: fnode.Hash(),
				Filename: fnode.Path(),
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

func (d *dir) loadAll() ([]fuse.Dirent, error) {
	results := []fuse.Dirent{}

	raw, err := database.LoadFromBolt(d.fs.db, d.dbKey())
	if err != nil {
		return nil, err
	}

	gob.NewDecoder(bytes.NewReader(raw)).Decode(&results)

	return results, err
}

func (d *dir) storeAll(content []fuse.Dirent) error {
	buff := bytes.Buffer{}
	if err := gob.NewEncoder(&buff).Encode(content); err != nil {
		return err
	}
	return database.StoreInBolt(d.fs.db, []byte(d.dbKey()), buff.Bytes())
}

func storeFile(db *bolt.DB, f *file) error {
	content := []byte(fmt.Sprintf("%s|%d", f.info.Hash, f.info.Size))
	return database.StoreInBolt(db, []byte(f.path()), content)
}

func storeDir(db *bolt.DB, dir *dir) error {
	return database.StoreInBolt(db, []byte(dir.Abs()), []byte("dir"))
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
		err     error
	)

	//try to loadAll content of directory from db

	results, err = d.loadAll()
	if err == nil {
		log.Debug("ReadDirAll got results from DB")
		return results, nil
	}

	if err != nil && err != database.NotFound {
		return nil, err
	}

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
//			file := &file{
//				info: &fileInfo{
//					Size: fileNode.Size(),
//					Hash: fileNode.Hash(),
//					Filename: fileNode.Path(),
//				},
//				dir:  d,
//			}
//			//insert file into db
//			if err := storeFile(d.fs.db, file); err != nil {
//				return nil, err
//			}

			// prepare object for fuse
			de.Type = fuse.DT_File
			de.Name = fileNode.Name()
		} else {
//			//dir
//			dir := &dir{
//				parent: d,
//				name:   child.Name(),
//			}
//			//add directory into db
//			if err := storeDir(d.fs.db, dir); err != nil {
//				return nil, err
//			}

			// prepare object for fuse
			de.Name = child.Name()
			de.Type = fuse.DT_Dir
		}

		results = append(results, de)
	}

	//d.storeAll(results)

	return results, nil
}

var _ = fs.NodeStringLookuper(&dir{})

func (d *dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Debug("Directory '%v' lookup on '%s'", d, name)
	node, _, err := d.searchEntry(name)
	if err != nil {
		return nil, err
	}
//
//	if !cached {
//		switch n := node.(type) {
//		case *dir:
//			dir := node.(*dir)
//			if err := storeDir(d.fs.db, dir); err != nil {
//				log.Error("Putting dir '%v' ento db failed: %s", n, err)
//			}
//		case *file:
//			f := node.(*file)
//			if err := storeFile(d.fs.db, f); err != nil {
//				log.Error("Putting file '%v' into db failed: %s", n, err)
//			}
//		}
//	}

	return node, nil
}
