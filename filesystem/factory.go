package filesystem

import (
	"sync"
	"github.com/Jumpscale/aysfs/metadata"
	"path"
)

type FileFactory interface {
	Get(parent Dir, leaf metadata.Leaf) File
}

type fileFactoryImpl struct {
	store map[string]File
	m sync.Mutex
}

func NewFileFactory() FileFactory {
	return &fileFactoryImpl{
		store: make(map[string]File),
	}
}

func (s *fileFactoryImpl) Get(parent Dir, leaf metadata.Leaf) File {
	s.m.Lock()
	defer s.m.Unlock()

	path := path.Join(parent.String(), leaf.Name())

	if file, ok := s.store[path]; ok {
		log.Notice("File '%s' is loaded from cache", path)
		return file
	}

	log.Notice("File '%s' is loaded", path)
	file := newFile(parent, leaf)
	s.store[path] = file
	return file
}