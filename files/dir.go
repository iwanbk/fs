package files

import (
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"syscall"

	"github.com/g8os/fs/rw/meta"
	"github.com/g8os/fs/utils"

	"github.com/hanwen/go-fuse/fuse"
)

var (
	SkipPattern = []*regexp.Regexp{
		regexp.MustCompile(`_\d+\.aydo$`), //backup extension before fs push.
	}
)

// Mkdir creates a directory
func (fs *fileSystem) Mkdir(path string, mode uint32, context *fuse.Context) (code fuse.Status) {
	fullPath := fs.GetPath(path)

	log.Debugf("Mkdir %v", path)

	status := fuse.ToStatus(os.Mkdir(fullPath, os.FileMode(mode)))
	if status != fuse.OK {
		return status
	}

	// This line break mkdir on OL
	// fs.tracker.Touch(fullPath)
	return fuse.OK
}

// Rmdir deletes a directory
func (fs *fileSystem) Rmdir(name string, context *fuse.Context) (code fuse.Status) {
	fullPath := fs.GetPath(name)

	log.Debugf("Rmdir %v", fullPath)

	m := meta.GetMeta(fullPath)

	// make sure we touchDeleted it
	defer func() {
		if fs.overlay {
			//Set delete mark
			touchDeleted(fullPath)
		}
	}()

	status := fuse.ToStatus(syscall.Rmdir(fullPath))

	// remove meta
	if !fs.overlay {
		if merr := os.Remove(string(m)); merr == nil {
			if status == fuse.ENOENT {
				//the file itself doesn't exist but the meta does.
				return fuse.OK
			}
		}
	}

	if !fs.overlay && status != fuse.OK && status != fuse.ENOENT {
		return status
	}

	return fuse.OK
}

// OpenDir opens a directory and return all files/dir in the directory.
// If it finds .meta file, it shows the file represented by that meta
func (fs *fileSystem) OpenDir(name string, context *fuse.Context) (stream []fuse.DirEntry, status fuse.Status) {
	log.Debugf("OpenDir %v", fs.GetPath(name))
	// What other ways beyond O_RDONLY are there to open
	// directories?
	f, err := os.Open(fs.GetPath(name))
	if err != nil {
		return nil, fuse.ToStatus(err)
	}
	defer f.Close()

	want := 100
	output := make([]fuse.DirEntry, 0, want)
	for {
		infos, err := f.Readdir(want)
		for i := range infos {
			// workaround for https://code.google.com/p/go/issues/detail?id=5960
			if infos[i] == nil {
				continue
			}

			d, ok := fs.getDirent(infos[i], name)
			if !ok { // ignore certain files : .meta, .aydo
				continue
			}

			if s := fuse.ToStatT(infos[i]); s != nil {
				d.Mode = uint32(s.Mode)
			} else {
				log.Errorf("ReadDir entry %q for %q has no stat info", infos[i].Name(), name)
			}

			output = append(output, d)
		}
		if len(infos) < want || err == io.EOF {
			break
		}
		if err != nil {
			log.Errorf("Readdir() returned err:%v", err)
			break
		}
	}

	return output, fuse.OK
}

func skipDir(name string) bool {
	for _, r := range SkipPattern {
		if r.MatchString(name) {
			return true
		}
	}

	return false
}

func (fs *fileSystem) getDirent(entry os.FileInfo, dir string) (fuse.DirEntry, bool) {
	name := entry.Name()
	fullPath := fs.GetPath(path.Join(dir, entry.Name()))
	dirEntry := fuse.DirEntry{
		Name: entry.Name(),
	}

	if skipDir(name) {
		return dirEntry, false
	}

	// process meta file:
	// - if represented file exist = ignore
	// - if meta marked as deleted = ignore
	if strings.HasSuffix(fullPath, meta.MetaSuffix) { // meta file.
		filePath := strings.TrimSuffix(fullPath, meta.MetaSuffix)
		if utils.Exists(filePath) { //if the file itself is there just skip because it will get processed anyway
			return dirEntry, false
		}
		m := meta.GetMeta(filePath)
		if m.Stat().Deleted() {
			//file was deleted
			return dirEntry, false
		}
		dirEntry.Name = strings.TrimSuffix(name, meta.MetaSuffix)
	} else { //normal file.
		m := meta.GetMeta(fullPath)
		if m.Stat().Deleted() {
			return dirEntry, false
		}
	}

	return dirEntry, true
}
