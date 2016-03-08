package files

import (
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/g8os/fs/rw/meta"
	"github.com/g8os/fs/utils"

	"github.com/hanwen/go-fuse/fuse"
)

var (
	SkipPattern = []*regexp.Regexp{
		regexp.MustCompile(`_\d+\.aydo$`), //backup extension before fs push.
	}
)

func skipDir(name string) bool {
	for _, r := range SkipPattern {
		if r.MatchString(name) {
			return true
		}
	}

	return false
}

func (fs *fileSystem) getDirent(entry os.FileInfo, fullPath string) (fuse.DirEntry, bool) {
	name := entry.Name()

	dirEntry := fuse.DirEntry{
		Name: name,
	}

	if skipDir(name) {
		return dirEntry, false
	}

	/*
		TODO : ask azmy & chris about these code
		if entry.IsDir() {
			dirEntry.Type = fuse.DT_Dir
		} else if entry.Mode()&os.ModeSymlink > 0 {
			dirEntry.Type = fuse.DT_Link
			return dirEntry, true
		} else {
			dirEntry.Type = fuse.DT_File
		}
	*/

	if strings.HasSuffix(fullPath, meta.MetaSuffix) {
		//We are processing a meta file.
		fullPath = strings.TrimSuffix(fullPath, meta.MetaSuffix)
		if utils.Exists(fullPath) {
			//if the file itself is there just skip because it will get processed anyway
			return dirEntry, false
		}
		m := meta.GetMeta(fullPath)
		if m.Stat().Deleted() {
			//file was deleted
			return dirEntry, false
		}
		dirEntry.Name = strings.TrimSuffix(name, meta.MetaSuffix)
	} else {
		//normal file.
		m := meta.GetMeta(fullPath)
		if m.Stat().Deleted() {
			return dirEntry, false
		}
	}

	return dirEntry, true
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
	want := 500
	output := make([]fuse.DirEntry, 0, want)
	for {
		infos, err := f.Readdir(want)
		for i := range infos {
			// workaround forhttps://code.google.com/p/go/issues/detail?id=5960
			if infos[i] == nil {
				continue
			}
			n := infos[i].Name()
			/*d := fuse.DirEntry{
				Name: n,
			}*/
			d, ok := fs.getDirent(infos[i], n)
			if !ok {
				continue
			}
			if s := fuse.ToStatT(infos[i]); s != nil {
				d.Mode = uint32(s.Mode)
			} else {
				log.Errorf("ReadDir entry %q for %q has no stat info", n, name)
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
