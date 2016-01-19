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
	CleanupOlderThan int

	Encrypted bool
	UserRsa   string
	StoreRsa  string

	ClientKey *rsa.PrivateKey `toml:"-"`
	GlobalKey *rsa.PrivateKey `toml:"-"`
}

type Aydostor struct {
	Name   string
	Addr   string
	Login  string
	Passwd string
}

func (c *Config) GetBackend(name string) (*Backend, error) {
	for i, b := range c.Backend {
		if b.Name == name {
			return &c.Backend[i], nil
		}
	}
	return nil, fmt.Errorf("backend not found")
}

func (c *Config) GetStor(name string) (*Aydostor, error) {
	for i, s := range c.Aydostor {
		if s.Name == name {
			return &c.Aydostor[i], nil
		}
	}
	return nil, fmt.Errorf("backend not found")
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

	for i := range cfg.Backend {
		if err := cfg.Backend[i].LoadRSAKeys(); err != nil {
			log.Fatal(err)
		}
	}

	return cfg
}
