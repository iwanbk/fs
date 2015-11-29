package cache

import (
	"io"
	"fmt"
	"github.com/Jumpscale/aysfs/utils"
)


type cacheManager struct {
	layers []Cache
}

func NewCacheManager() CacheManager {
	return &cacheManager{
		layers: make([]Cache, 0),
	}
}

func (mgr *cacheManager) getCloseCallback(index int) utils.OnClose {
	return func(path string, file io.ReadSeeker) {
		//only update higher cache layers
		for ;index >=0; index -- {
			if layer, ok := mgr.layers[index].(CacheWriter); ok {
				log.Debug("Deduping file to '%s'", layer)
				layer.DeDupe(path, file)
			}
		}
	}
}

//Open file going through all the cache layer, and return on first open success.
func (mgr *cacheManager) Open(path string) (io.ReadSeeker, error) {
	for index, layer := range mgr.layers {
		file, err := layer.Open(path)
		if err == nil {
			log.Debug("Opening file '%s' from '%s'", path, layer)
			return utils.NewCallbackCloser(file, path, mgr.getCloseCallback(index - 1)), nil
		}
	}
	return nil, fmt.Errorf("All caches couldn't open the file '%s'", path)
}

func (mgr *cacheManager) GetMetaData(id string) ([]string, error) {
	for _, layer := range mgr.layers {
		log.Debug("Getting meta data from '%s'", layer)
		meta, err := layer.GetMetaData(id)
		if err == nil {
			return meta, nil
		}
		log.Debug("Metadata loading from cache '%s' error: '%s'", layer, err)
	}
	return nil, fmt.Errorf("All caches didn't provide cache for '%s'", id)
}

func (mgr *cacheManager) Exists(path string) bool {
	for _, layer := range mgr.layers {
		exists := layer.Exists(path)
		if exists {
			return exists
		}
	}
	return false
}

func (mgr *cacheManager) Purge() error {
	for _, layer := range mgr.layers {
		if layer, ok := layer.(CachePurger); ok {
			layer.Purge()
		}
	}
	return nil
}

func (mgr *cacheManager) BasePath() string {
	return "/"
}

func (mgr *cacheManager) AddLayer(cache Cache) {
	log.Debug("Adding cache layer '%s'", cache)
	mgr.layers = append(mgr.layers, cache)
}

func (mgr *cacheManager) Layers() []Cache {
	return mgr.layers
}

