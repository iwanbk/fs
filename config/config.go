package config

import (
	"os"

	"github.com/naoina/toml"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("config")
)

type Config struct {
	Main     Main
	Metadata string
	// Ays   []AYS
	Cache []Cache
	Debug []Debug
}

type Main struct {
	ID string
}

type AYS struct {
	ID string

	//	PrefetchCacheGrid  bool
	//	PrefetchCacheLocal bool
	//	CacheLocal         bool
	//	CacheGrid          bool
}

type Cache struct {
	URL   string
	Purge bool
}

type Debug struct {
	DebugFilter []string
	Redis       Redis
}

type Redis struct {
	Addr     string
	Port     int
	Password string
}

func LoadConfig(path string) *Config {
	cfg := &Config{}
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("can't open config file at %s: %s\n", path, err)
	}
	err = toml.NewDecoder(f).Decode(cfg)
	if err != nil {
		log.Fatalf("can't read config file at %s: %s\n", path, err)
	}

	if cfg.Metadata == "" {
		cfg.Metadata = "/etc/ays/local"
	}

	return cfg
}
