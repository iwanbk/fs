package config

import (
	"time"
	"github.com/naoina/toml"
	"os"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("config")
)

type Config struct {
	Main Main

	Ays []AYS

	Cache []Cache
	Store []Store

	Debug []Debug
}

type Main struct {
	ID     string
}

type AYS struct {
	ID                 string
	PrefetchCacheGrid  bool
	PrefetchCacheLocal bool
	CacheLocal         bool
	CacheGrid          bool
}

type Cache struct {
	Mnt         string
	Expirtation time.Duration
}

type Store struct {
	URL         string
	Expirtation time.Duration
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
	return cfg
}