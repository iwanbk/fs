package watcher

import (
	"time"

	"github.com/Jumpscale/aysfs/config"
)

var cfg *watcherConfig

type watcherConfig struct {
	CleaningInterval time.Duration
	StoreInterval    time.Duration

	Backend *config.Backend
	Store   *config.Aydostor
}

func init() {
	cfg = &watcherConfig{
		CleaningInterval: time.Hour * 60 * 24, //once a day
		StoreInterval:    time.Hour,           //once an hour
	}
}

// SetIntervals define how often the watcher shoudl upload file to the store and clean the backend.
func SetIntervals(toStore time.Duration, clean time.Duration) {
	cfg.StoreInterval = toStore
	cfg.CleaningInterval = clean
}

func SetBackend(backend *config.Backend) {
	cfg.Backend = backend
}

func SetStore(store *config.Aydostor) {
	cfg.Store = store
	storeClient = NewStoreClient(store.Addr)
}
