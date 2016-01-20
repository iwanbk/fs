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
	MetaSuffix = ".meta"
)

type MetaFile struct {
	Path     string `toml:"-"`
	Hash     string
	Size     uint64
	UserKey  string
	StoreKey string
}

func Load(name string) (*MetaFile, error) {
	meta := MetaFile{
		Path: name,
	}
	_, err := toml.DecodeFile(name, &meta)
	if err != nil {
		return nil, err
	}

	return &meta, nil
}

func Save(meta *MetaFile) error {
	if meta.Path == "" {
		return fmt.Errorf("Meta path is not set")
	}
	dir := path.Dir(meta.Path)
	os.MkdirAll(dir, os.ModePerm)
	file, err := os.OpenFile(meta.Path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
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

		mPath := path.Join(backend.Path, fmt.Sprintf("%s%s", entity.Path, MetaSuffix))
		m := MetaFile{
			Hash: entity.Hash,
			Size: uint64(entity.Size),
			Path: mPath,
		}

		if err := Save(&m); err != nil {
			return err
		}
	}

	return nil
}
