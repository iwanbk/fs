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

	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/filesystem"
	"github.com/op/go-logging"
)

var version = "0.1"

var (
	fVersion    bool
	fPprof      bool
	fConfigPath string
	fDebugLevel int
)

var progName = filepath.Base(os.Args[0])

func init() {
	flag.BoolVar(&fVersion, "v", false, "show version")
	flag.BoolVar(&fPprof, "pprof", false, "enable net pprof")

	flag.StringVar(&fConfigPath, "config", "config.toml", "path to config file")
	flag.StringVar(&fConfigPath, "c", "config.toml", "path to config file")
	flag.IntVar(&fDebugLevel, "l", 5, "Debug leve (0 less verbose, to 5 most verbose [default])")
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", progName)
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", progName)
	fmt.Fprintf(os.Stderr, "\n")
	flag.PrintDefaults()
}

func configureLogging() {
	logging.SetLevel(logging.Level(fDebugLevel), "")
	formatter := logging.MustStringFormatter("%{color}%{module} %{level:.1s} > %{message} %{color:reset}")
	logging.SetFormatter(formatter)
}

func main() {
	flag.Parse()
	flag.Usage = usage

	if fVersion {
		fmt.Println("version :", version)
		os.Exit(0)
	}

	configureLogging()

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

	cfg := config.LoadConfig(fConfigPath)
	if cfg.Main.Boltdb == "" {
		cfg.Main.Boltdb = "db.bolt"
	}

	fs := filesystem.NewFS(mountpoint, cfg)

	log.Println("mounting Fuse File system")
	if err := mount(fs, flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}
