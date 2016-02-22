package config

import (
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/Jumpscale/aysfs/crypto"
	"github.com/naoina/toml"
	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("config")
)

const (
	RO = "RO"
	RW = "RW"
	OL = "OL"
)

type Config struct {
	Mount    []Mount
	Backend  map[string]Backend
	Aydostor map[string]Aydostor
}

type Mount struct {
	Path     string
	Flist    string
	Backend  string
	Mode     string
	Stor     string `toml:",omitempty"`
	TrimBase bool
}

type Backend struct {
	Name string `toml:"-"`
	Path string
	Stor string

	Upload           bool `toml:",omitempty"`
	Namespace        string
	AydostorPushCron string `toml:",omitempty"`
	CleanupCron      string `toml:",omitempty"`
	CleanupOlderThan int    `toml:",omitempty"`

	Log string

	Encrypted bool   `toml:",omitempty"`
	UserRsa   string `toml:",omitempty"`
	StoreRsa  string `toml:",omitempty"`

	ClientKey *rsa.PrivateKey `toml:"-"`
	GlobalKey *rsa.PrivateKey `toml:"-"`
}

type Aydostor struct {
	Name string `toml:"-"`

	Addr   string
	Login  string
	Passwd string
}

func (c *Config) GetBackend(name string) (*Backend, error) {
	if backend, ok := c.Backend[name]; ok {
		backend.Name = name
		return &backend, nil
	} else {
		return nil, fmt.Errorf("Backend '%s' not found", name)
	}
}

func (c *Config) GetStor(name string) (*Aydostor, error) {
	if stor, ok := c.Aydostor[name]; ok {
		stor.Name = name
		return &stor, nil
	} else {
		return nil, fmt.Errorf("Stor '%s' not found", name)
	}
}

func (b *Backend) LoadRSAKeys() error {
	if b.Encrypted {
		if _, err := os.Stat(b.UserRsa); err == nil {
			content, err := ioutil.ReadFile(b.UserRsa)
			if err != nil {
				err := fmt.Errorf("Error reading rsa key at %v: %v", b.UserRsa, err)
				log.Errorf(err.Error())
				return err
			}
			b.ClientKey, err = crypto.ReadPrivateKey(content)
			if err != nil {
				err := fmt.Errorf("Error reading rsa key at %v: %v", b.UserRsa, err)
				log.Errorf(err.Error())
				return err
			}
		}

		if _, err := os.Stat(b.StoreRsa); err == nil {
			content, err := ioutil.ReadFile(b.StoreRsa)
			if err != nil {
				err := fmt.Errorf("Error reading rsa key at %v: %v", b.StoreRsa, err)
				log.Errorf(err.Error())
				return err
			}
			b.GlobalKey, err = crypto.ReadPrivateKey(content)
			if err != nil {
				err := fmt.Errorf("Error reading rsa key at %v: %v", b.StoreRsa, err)
				log.Errorf(err.Error())
				return err
			}
		}
	}
	return nil
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

	for name, backend := range cfg.Backend {
		if err := backend.LoadRSAKeys(); err != nil {
			log.Fatal(err)
		}

		cfg.Backend[name] = backend
	}

	return cfg
}
