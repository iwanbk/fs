package meta

import (
	"fmt"
	"os"

	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/utils"
)

var (
	PathSep    = "/"
	ignoreLine = fmt.Errorf("Ignore Line")
)

const (
	MetaSuffix = ".meta"
)

type MetaFile struct {
	Hash     string
	Size     uint64
	UserKey  string
	StoreKey string
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

		file := path.Join(backend.Path, entity.Path)
		m := GetMeta(file)

		fExists := utils.Exists(file)

		data := &MetaFile{
			Hash: entity.Hash,
			Size: uint64(entity.Size),
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

type Entry struct {
	Path string
	Hash string
	Size int64
}

func ParseLine(base, line string) (*Entry, error) {
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
}
