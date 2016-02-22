package rw

import (
	"bazil.org/fuse/fs"
	"sync"
	"time"
)

const (
	FactoryVacuum         = 1 * time.Minute
	FactoryInvalidateTime = 5 * time.Minute
)

type Factory interface {
	File(fs *FS, path string, parent *fsDir) fs.Node
	Link(fs *FS, path string, parent *fsDir) fs.Node
	Dir(fs *FS, path string, parent *fsDir) fs.Node
	Get(path string) (fs.Node, bool)
	Forget(path string)
}

type factory struct {
	cache  map[string]fs.Node
	access map[string]time.Time
	m      sync.Mutex
}

func NewFactory() Factory {
	f := &factory{
		cache:  make(map[string]fs.Node),
		access: make(map[string]time.Time),
	}

	go f.monitor()

	return f
}

func (f *factory) monitor() {
	log.Debugf("Started factory vacuum routine")
	for {
		time.Sleep(FactoryVacuum)
		log.Debugf("Facotory vacuuming")
		now := time.Now()
		for name, t := range f.access {
			if now.Sub(t) > FactoryInvalidateTime {
				log.Debugf("Factory auto invalidation of '%s'", name)
				f.Forget(name)
			}
		}
	}
}

func (f *factory) File(fs *FS, path string, parent *fsDir) fs.Node {
	log.Debugf("Creating a file instance for: %s", path)
	f.m.Lock()
	defer f.m.Unlock()
	node, ok := f.Get(path)
	if ok {
		return node
	}

	node = newFile(fs, path, parent)
	f.cache[path] = node
	return node
}

func (f *factory) Link(fs *FS, path string, parent *fsDir) fs.Node {
	log.Debugf("Createing a link instance for: %s", path)
	f.m.Lock()
	defer f.m.Unlock()
	node, ok := f.Get(path)
	if ok {
		return node
	}

	node = newLink(fs, path, parent)
	f.cache[path] = node
	return node
}

func (f *factory) Dir(fs *FS, path string, parent *fsDir) fs.Node {
	log.Debugf("Creating a dir instance for: %s", path)
	f.m.Lock()
	defer f.m.Unlock()
	node, ok := f.Get(path)
	if ok {
		return node
	}

	node = newDir(fs, path, parent)
	f.cache[path] = node
	return node
}

func (f *factory) Get(path string) (fs.Node, bool) {
	node, ok := f.cache[path]
	//record access attempts
	f.access[path] = time.Now()
	return node, ok
}

func (f *factory) Forget(path string) {
	f.m.Lock()
	defer f.m.Unlock()
	delete(f.cache, path)
	delete(f.access, path)
}
