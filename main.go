package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"net/http"
	_ "net/http/pprof"

	"github.com/naoina/toml"
)

var version = "0.1"

var cfg *Config

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

var (
	fVersion    bool
	fPprof      bool
	fConfigPath string
)
var progName = filepath.Base(os.Args[0])

func init() {

	flag.BoolVar(&fVersion, "v", false, "show version")
	flag.BoolVar(&fPprof, "pprof", false, "enable net pprof")

	flag.StringVar(&fConfigPath, "config", "config.toml", "path to config file")
	flag.StringVar(&fConfigPath, "c", "config.toml", "path to config file")
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", progName)
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", progName)
	fmt.Fprintf(os.Stderr, "\n")
	flag.PrintDefaults()
}

func main() {
	flag.Parse()
	flag.Usage = usage

	if fVersion {
		fmt.Println("version :", version)
		os.Exit(0)
	}

	if fPprof {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))

		}()
	}

	log.SetPrefix(progName + ": ")

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	mountpoint := flag.Arg(0)
	if strings.HasSuffix(mountpoint, string(os.PathSeparator)) {
		mountpoint = mountpoint[:len(mountpoint)-2]
	}

	cfg = loadConfig(fConfigPath)
	if cfg.Main.Boltdb == "" {
		cfg.Main.Boltdb = "db.bolt"
	}

	fs := newFS(mountpoint, cfg)

	log.Println("mounting Fuse File system")
	if err := mount(fs, flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}
