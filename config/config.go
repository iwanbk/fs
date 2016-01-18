package config

import (
	"fmt"
	"os"

	"github.com/naoina/toml"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("config")
)

type Config struct {
	Mount    []Mount
	Backend  []Backend
	Aydostor []Aydostor
}

type Mount struct {
	Path    string
	Flist   string
	Backend string
	ACL     string
}

type Backend struct {
	Name             string
	Path             string
	Stor             string
	Namespace        string
	AydostorPushCron string
	CleanupCron      string
}

type Aydostor struct {
	Name   string
	Addr   string
	Login  string
	Passwd string
}

func (c *Config) GetBackend(name string) (Backend, error) {
	for _, b := range c.Backend {
		if b.Name == name {
			return b, nil
		}
	}
	return Backend{}, fmt.Errorf("backend not found")
}

func (c *Config) GetStor(name string) (Aydostor, error) {
	for _, s := range c.Aydostor {
		if s.Name == name {
			return s, nil
		}
	}
	return Aydostor{}, fmt.Errorf("backend not found")
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
