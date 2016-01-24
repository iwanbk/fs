package meta

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/metadata"
	"github.com/Jumpscale/aysfs/utils"
	"path"
)

const (
	MetaSuffix           = ".meta"
	OverlayDeletedSuffix = "_###"
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
	MetaInitial   MetaState = 0000
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
	file, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		return err
	}
	encoder := toml.NewEncoder(file)
	return encoder.Encode(meta)
}

func PopulateFromPList(backend *config.Backend, base string, plist string) error {
	iter, err := utils.IterFlistFile(plist)
	if err != nil {
		return err
	}

	for line := range iter {
		entity, err := metadata.ParseLine(base, line)
		if err != nil {
			return err
		}

		file := path.Join(backend.Path, entity.Path)
		m := GetMeta(file)

		state := m.Stat()
		if state.Modified() || state.Deleted() {
			continue
		}

		fExists := utils.Exists(file)

		data := &MetaFile{
			Hash: entity.Hash,
			Size: uint64(entity.Size),
		}

		if fExists {
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
