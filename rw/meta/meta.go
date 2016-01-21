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
	Path     string `toml:"-"`
	Hash     string
	Size     uint64
	UserKey  string
	StoreKey string
}

func Load(name string) (*MetaFile, error) {
	meta := &MetaFile{
		Path: name,
	}
	_, err := toml.DecodeFile(name, &meta)
	if err != nil {
		return meta, err
	}

	return meta, nil
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

		fPath := path.Join(backend.Path, entity.Path)
		mPath := fmt.Sprintf("%s%s", fPath, MetaSuffix)

		if utils.Exists(fmt.Sprintf("%s%s", fPath, OverlayDeletedSuffix)) {
			//file was deleted locally, completely ignore
			continue
		}

		if !utils.IsWritable(mPath) {
			//ReadOnly meta means file has changed. We don't override meta of content
			continue
		}

		m := &MetaFile{
			Hash: entity.Hash,
			Size: uint64(entity.Size),
			Path: mPath,
		}

		oldMeta, err := Load(mPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		if err := Save(m); err != nil {
			return err
		}

		if utils.Exists(fPath) {
			//both meta and file exists. This file wasn't modified we can
			//just now place the meta and delete the file ONLY if file was changed.
			if oldMeta.Hash != entity.Hash {
				os.Remove(fPath)
			}
		}
	}

	return nil
}
