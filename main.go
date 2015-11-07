package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"bazil.org/fuse"

	"github.com/naoina/toml"
)

var configPath = flag.String("config", "config.toml", "path to config file")
var progName = filepath.Base(os.Args[0])

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", progName)
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", progName)
	fmt.Fprintf(os.Stderr, "\n")
	flag.PrintDefaults()
}

func loadConfig(path string) *Config {
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

var cfg *Config

func main() {
	log.SetFlags(0)
	log.SetPrefix(progName + ": ")

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	cfg = loadConfig(*configPath)
	if cfg.Main.boltdb == "" {
		cfg.Main.boltdb = "db.bolt"
	}

	// db, err := bolt.Open(cfg.Main.boltdb, 0600, nil)
	// if err != nil {
	// 	log.Fatalln("can't open boltdb database at %s: %s\n", cfg.Main.boltdb, err)
	// }
	f, err := os.Open("metadata.json")
	if err != nil {
		log.Fatalln(err)
	}
	node := map[string]json.RawMessage{}
	if err := json.NewDecoder(f).Decode(&node); err != nil {
		log.Fatalln(err)
	}

	filesys := &FS{
		// db: db,
		root:     node,
		binStore: "bin_store",
	}

	fuse.Debug(func(msg interface{}) {
		log.SetPrefix("debug ")
		log.Printf("%+v\n", msg)
	})

	if err := mount(filesys, flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}
