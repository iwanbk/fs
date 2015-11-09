package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"net/http"
	_ "net/http/pprof"

	"github.com/boltdb/bolt"
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
var enablePprof = flag.Bool("pprof", false, "enable net pprof")

func main() {
	if *enablePprof {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))

		}()
	}

	log.SetFlags(0)
	log.SetPrefix(progName + ": ")

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	mountpoint := flag.Arg(0)
	if strings.HasSuffix(mountpoint, string(os.PathSeparator)) {
		mountpoint = mountpoint[:len(mountpoint)-2]
	}

	cfg = loadConfig(*configPath)
	if cfg.Main.Boltdb == "" {
		cfg.Main.Boltdb = "db.bolt"
	}

	_ = os.Remove(cfg.Main.Boltdb)
	db, err := bolt.Open(cfg.Main.Boltdb, 0600, nil)
	if err != nil {
		log.Fatalln("can't open boltdb database at %s: %s\n", cfg.Main.Boltdb, err)
	}

	caches := []cacher{}
	for _, c := range cfg.Cache {
		fmt.Println("add cache", c.Mnt)
		caches = append(caches, &fsCache{
			root: c.Mnt,
			// expiration: c.Expirtation,
			dedupe: "dedupe",
		})
	}

	stores := []cacher{}
	for _, s := range cfg.Store {
		fmt.Println("add Store", s.URL)
		stores = append(stores, &httpCache{
			addr: s.URL,
			// expiration: s.Expirtation,
			dedupe: "dedupe",
		})
	}

	filesys := &FS{
		db:       db,
		metadata: []string{},
		caches:   caches,
		stores:   stores,
	}

	for _, ays := range cfg.Ays {
		log.Println("fetching md for", ays.ID)
		metadata, err := filesys.GetMetaData("dedupe", ays.ID)
		if err != nil {
			log.Fatalln("error during metadata fetching", err)
		}

		for i, line := range metadata {
			if strings.HasPrefix(line, mountpoint) {
				metadata[i] = strings.TrimPrefix(line, mountpoint)
			}
		}
		sort.StringSlice(metadata).Sort()
		filesys.metadata = append(filesys.metadata, metadata...)
	}

	log.Println("mounting Fuse File system")
	if err := mount(filesys, flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}
