package meta

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
)

const (
	MetaSuffix = ".meta"
)

type MetaFile struct {
	Path string `toml:"-"`
	Hash string
	Size uint64
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
	file, err := os.OpenFile(meta.Path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	encoder := toml.NewEncoder(file)
	return encoder.Encode(meta)
}
