package filesystem

import (
	"github.com/Jumpscale/aysfs/metadata"
	"path"
	"sync"
)

type NodeFactory interface {
	GetFile(fs *FS, parent Dir, leaf metadata.Leaf) File
	GetDir(fs *FS, parent Dir, branch metadata.Node) Dir
}

type nodeFactoryImpl struct {
	fileStore map[string]File
	dirStore  map[string]Dir
	m         sync.Mutex
}

func NewNodeFactory() NodeFactory {
	return &nodeFactoryImpl{
		fileStore: make(map[string]File),
		dirStore:  make(map[string]Dir),
	}
}

func (s *nodeFactoryImpl) getPath(parent Dir, node metadata.Node) string {
	if parent == nil {
		return node.Name()
	} else {
		return path.Join(parent.String(), node.Name())
	}
}

func (s *nodeFactoryImpl) GetFile(fs *FS, parent Dir, leaf metadata.Leaf) File {
	s.m.Lock()
	defer s.m.Unlock()

	path := s.getPath(parent, leaf)

	if file, ok := s.fileStore[path]; ok {
		log.Notice("File '%s' is loaded from cache", path)
		return file
	}

	file := newFile(parent, leaf)
	s.fileStore[path] = file
	return file
}

func (s *nodeFactoryImpl) GetDir(fs *FS, parent Dir, branch metadata.Node) Dir {
	s.m.Lock()
	defer s.m.Unlock()

	path := s.getPath(parent, branch)

	if dir, ok := s.dirStore[path]; ok {
		log.Notice("Dir '%s' is loaded from cache", path)
		return dir
	}

	dir := newDir(fs, parent, branch)
	s.dirStore[path] = dir

	return dir
}
