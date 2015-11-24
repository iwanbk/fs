package filesystem

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"path"
	"strings"
	"github.com/Jumpscale/aysfs/database"
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
	var node fs.Node
	log.Debug("Directory search entry '%s'", name)

	err := d.fs.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("main"))
		if bucket == nil {
			return fuse.ENOENT
		}
		raw := bucket.Get([]byte(filepath.Join(d.Abs(), name)))
		if raw == nil {
			return fuse.ENOENT
		}

		if string(raw[:3]) == "dir" {
			node = &dir{
				fs:     d.fs,
				name:   name,
				parent: d,
			}
			return nil
		}

		fi, err := newFileInfo(fmt.Sprintf("%s|%s", name, string(raw)))
		if err != nil {
			return err
		}
		node = &file{
			dir:  d,
			info: fi,
		}

		return nil
	})

	// Entry found
	if err == nil {
		log.Debug("Entry '%s' found", node)
		return node, true, nil
	}

	// error which is not just a not found error
	if err != nil && err != fuse.ENOENT {
		return nil, false, err
	}

	log.Debug("Entry '%s' found", name)
	// look into the metadata for the entry
	for _, line := range d.fs.metadata {
		log.Debug("Processing metadata line '%s'", line)

		line = filepath.Clean(line)
		if !strings.HasPrefix(line, d.Abs()) {
			continue
		}

		i := strings.Index(line, d.name)
		if i <= -1 {
			continue
		}

		items := strings.Split(line[i:], string(os.PathSeparator))
		if len(items) > 1 {
			baseName := items[1]

			if strings.Contains(baseName, "|") {
				// baseName has the form : 'name|hash|size'
				fi, err := newFileInfo(baseName)
				if err != nil {
					return nil, false, err
				}
				if fi.Filename != name {
					continue
				}

				return &file{
					dir:  d,
					info: fi,
				}, false, nil
			}

			if baseName != name {
				continue
			}

			return &dir{
				fs:     d.fs,
				name:   name,
				parent: d,
			}, false, nil

		}
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
	log.Debug("Read all directory '%s' entries", d)

	var (
		results []fuse.Dirent
		err     error
	)

	//try to loadAll content of directory from db
	results, err = d.loadAll()
	if err == nil {
		return results, nil
	}

	if err != nil && err != database.NotFound {
		return nil, err
	}

	// content is not yet in db
	// walk over metadata and populate db
	set := map[string]struct{}{}
	for _, line := range d.fs.metadata {
		line = filepath.Clean(line)
		if !strings.HasPrefix(line, d.Abs()) {
			continue
		}

		i := strings.Index(line, d.name)
		if i <= -1 {
			continue
		}

		de := fuse.Dirent{}
		items := strings.Split(line[i:], string(os.PathSeparator))
		if len(items) > 1 {
			baseName := items[1]

			if strings.Contains(baseName, "|") {
				// baseName has the form : 'name|hash|size'
				fi, err := newFileInfo(baseName)
				if err != nil {
					return nil, err
				}
				file := &file{
					info: fi,
					dir:  d,
				}
				// insert file into db
				if err := storeFile(d.fs.db, file); err != nil {
					return nil, err
				}

				// prepare object for fuse
				de.Type = fuse.DT_File
				de.Name = fi.Filename

			} else {
				// don't add twice the same directory
				if _, ok := set[baseName]; ok {
					continue
				}

				dir := &dir{
					parent: d,
					name:   baseName,
				}
				// //add directory into db
				if err := storeDir(d.fs.db, dir); err != nil {
					return nil, err
				}
				set[baseName] = struct{}{}

				// prepare object for fuse
				de.Name = baseName
				de.Type = fuse.DT_Dir
			}
			results = append(results, de)
		}
	}

	d.storeAll(results)

	return results, nil
}

var _ = fs.NodeStringLookuper(&dir{})

func (d *dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Debug("Directory '%v' lookup on '%s'", d, name)
	node, cached, err := d.searchEntry(name)
	if err != nil {
		return nil, err
	}

	if !cached {
		switch n := node.(type) {
		case *dir:
			dir := node.(*dir)
			if err := storeDir(d.fs.db, dir); err != nil {
				log.Error("Putting dir '%v' ento db failed: %s", n, err)
			}
		case *file:
			f := node.(*file)
			if err := storeFile(d.fs.db, f); err != nil {
				log.Error("Putting file '%v' into db failed: %s", n, err)
			}
		}
	}

	return node, nil
}
