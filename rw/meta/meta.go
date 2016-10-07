package meta

import (
	"fmt"
	"os"
	"time"
	"strconv"
	"syscall"
	"os/user"

	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/g8os/fs/config"
	"github.com/g8os/fs/utils"
)

var (
	PathSep    = "/"
	ignoreLine = fmt.Errorf("Ignore Line")
)

const (
	MetaSuffix = ".meta"
)

type MetaFile struct {
	Hash         string   // file hash
	Size         uint64   // file size in bytes
	Uname        string   // username (used for permissions)
	Uid          uint32
	Gname        string   // groupname (used for permissions)
	Gid          uint32
	Permissions  uint32   // perissions (octal style)
	Filetype     uint32   // golang depend os.FileMode id
	Ctime        uint64   // creation time
	Mtime        uint64   // modification time
	Extended     string   // extended attribute (see python flist doc)
	DevMajor     int64    // block/char device major id
	DevMinor     int64    // block/char device minor id
	UserKey      string
	StoreKey     string
}

type MetaState uint32

const (
	MetaStateMask MetaState = 0500
	MetaInitial   MetaState = 0400
	MetaModified  MetaState = 0200
	MetaDeleted   MetaState = 0100
)

func (s MetaState) Modified() bool {
	return s&MetaModified != 0
}

func (s MetaState) Deleted() bool {
	return s&MetaDeleted != 0
}

func (s MetaState) SetModified(m bool) MetaState {
	if m {
		return MetaState(s | MetaModified)
	} else {
		return MetaState(s & (^MetaModified))
	}
}

func (s MetaState) SetDeleted(m bool) MetaState {
	if m {
		return MetaState(s | MetaDeleted)
	} else {
		return MetaState(s & (^MetaDeleted))
	}
}

type Meta string

//MetaPath get meta path for given file name
func GetMeta(name string) Meta {
	return Meta(fmt.Sprintf("%s%s", name, MetaSuffix))
}

func (m Meta) Exists() bool {
	return utils.Exists(string(m))
}

func (m Meta) Stat() MetaState {
	stat, err := os.Stat(string(m))
	if err != nil {
		return MetaInitial
	}

	//mask out everything except the USER perm bits
	return MetaState(stat.Mode()) & MetaStateMask
}

func (m Meta) SetStat(state MetaState) {
	os.Chmod(string(m), os.FileMode(state))
}

func (m Meta) String() string {
	return string(m)
}

func (m Meta) Load() (*MetaFile, error) {
	meta := MetaFile{}
	_, err := toml.DecodeFile(string(m), &meta)
	if err != nil {
		return nil, err
	}

	return &meta, nil
}

func (m Meta) Save(meta *MetaFile) error {
	p := string(m)
	dir := path.Dir(p)
	os.MkdirAll(dir, os.ModePerm)
	file, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, os.FileMode(MetaInitial))
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := toml.NewEncoder(file)
	return encoder.Encode(meta)
}

func PopulateFromPList(backend *config.Backend, base string, plist string) error {
	iter, err := utils.IterFlistFile(plist)
	if err != nil {
		return err
	}

	for line := range iter {
		entity, err := ParseLine(base, line)
		if err != nil {
			return err
		}

		file := path.Join(backend.Path, entity.Filepath)
		m := GetMeta(file)

		fExists := utils.Exists(file)

		// user and group id
		uid := 0
		u, err := user.Lookup(entity.Uname)
		if err == nil {
			uid, _ = strconv.Atoi(u.Uid)
		}

		gid := 0
		g, err := user.LookupGroup(entity.Gname)
		if err == nil {
			gid, _ = strconv.Atoi(g.Gid)
		}

		data := &MetaFile{
			Hash: entity.Hash,
			Size: uint64(entity.Filesize),
			Uname: entity.Uname,
			Uid: uint32(uid),
			Gname: entity.Gname,
			Gid: uint32(gid),
			Permissions: uint32(entity.Permissions),
			Filetype: entity.Filetype,
			Ctime: uint64(entity.Ctime.Unix()),
			Mtime: uint64(entity.Mtime.Unix()),
			Extended: entity.Extended,
			DevMajor: entity.DevMajor,
			DevMinor: entity.DevMinor,
		}

		if fExists && !m.Stat().Modified() {
			//both meta and file exists. This file wasn't modified we can
			//just now place the meta and delete the file ONLY if file was changed.

			oldMeta, err := m.Load()
			if err != nil && !os.IsNotExist(err) {
				return err
			}

			if oldMeta.Hash != entity.Hash {
				os.Remove(file)
			}
		}

		if err := m.Save(data); err != nil {
			return err
		}
	}

	return nil
}

/*
type Entry struct {
	Path string
	Hash string
	Size int64
}
*/

type Entry struct {
	Filepath     string     // complete filepath
	Hash         string     // file hash
	Filesize     int64      // file size in bytes
	Uname        string     // username (used for permissions)
	Gname        string     // groupname (used for permissions)
	Permissions  int64      // perissions (octal style)
	Filetype     uint32     // golang depend os.FileMode id
	Ctime        time.Time  // creation time
	Mtime        time.Time  // modification time
	Extended     string     // extended attribute (see python flist doc)
	DevMajor     int64      // block/char device major id
	DevMinor     int64      // block/char device minor id
}

func ParseLine(base, line string) (*Entry, error) {
	/*
	entry := Entry{}

	lineParts := strings.Split(line, "|")
	if len(lineParts) != 3 {
		return nil, fmt.Errorf("Wrong metadata line syntax '%s'", line)
	}

	path := lineParts[0]
	if base != "" && strings.HasPrefix(path, base) {
		path = strings.TrimPrefix(path, base)
	}

	//remove prefix / if exists.
	entry.Path = strings.TrimLeft(path, PathSep)
	entry.Hash = lineParts[1]
	count, err := fmt.Sscanf(lineParts[2], "%d", &entry.Size)
	if err != nil || count != 1 {
		return nil, fmt.Errorf("Invalid metadata line '%s' (%d, %s)", line, count, err)
	}

	return &entry, nil
	*/
	if line == "" {
		err := fmt.Errorf("Cannot parse empty lines\n")
		return nil, err
	}

	// split line
	items := strings.Split(line, "|")

	if len(items) < 10 {
		err := fmt.Errorf("Flist item: malformed line, at least 10 fields expected, %d found\n", len(items))
		return nil, err
	}

	//
	// file stats
	//
	length, err := strconv.ParseInt(items[2], 10, 64)
	if err != nil {
		fmt.Errorf("Error parsing filesize: %v\n", err)
		return nil, err
	}

	perms, err := strconv.ParseInt(items[5], 8, 64)
	if err != nil {
		fmt.Errorf("Error parsing permissions: %v\n", err)
		return nil, err
	}

	//
	// file type
	//
	ftype, err := strconv.Atoi(items[6])
	fileType := os.ModeDir
	if err != nil {
		fmt.Errorf("Error parsing filetype: %v\n", err)
		return nil, err
	}

	devMajor := int64(0)
	devMinor := int64(0)

	if ftype == 3 || ftype == 5 {
		temp := strings.Split(items[9], ",")

		devMajor, err = strconv.ParseInt(temp[0], 10, 64)
		if err != nil {
			fmt.Errorf("Error parsing device major id: %v\n", err)
			return nil, err
		}

		devMinor, err = strconv.ParseInt(temp[1], 10, 64)
		if err != nil {
			fmt.Errorf("Error parsing device minor id: %v\n", err)
			return nil, err
		}
	}

	if ftype == 0 { fileType = syscall.S_IFSOCK }
	if ftype == 1 { fileType = syscall.S_IFLNK }
	if ftype == 2 { fileType = syscall.S_IFREG }
	if ftype == 3 { fileType = syscall.S_IFBLK }
	if ftype == 4 { fileType = syscall.S_IFDIR }     // not used
	if ftype == 5 { fileType = syscall.S_IFCHR }
	if ftype == 6 { fileType = syscall.S_IFIFO }

	//
	// file times
	//
	ctime, err := strconv.ParseInt(items[7], 10, 64)
	if err != nil {
		fmt.Errorf("Error parsing creation time: %v\n", err)
		return nil, err
	}

	mtime, err := strconv.ParseInt(items[8], 10, 64)
	if err != nil {
		fmt.Errorf("Error parsing modification time: %v\n", err)
		return nil, err
	}

	return &Entry{
		Filepath: items[0],
		Hash: items[1],
		Filesize: length,
		Uname: items[3],
		Gname: items[4],
		Permissions: perms,
		Filetype: uint32(fileType),
		Ctime: time.Unix(ctime, 0),
		Mtime: time.Unix(mtime, 0),
		Extended: items[9],
		DevMajor: devMajor,
		DevMinor: devMinor,
	}, nil
}
