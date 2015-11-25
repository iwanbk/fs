package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"net/http"
	_ "net/http/pprof"

	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/filesystem"
	"github.com/op/go-logging"
	"path"
)

var (
	version = "0.1"
	log = logging.MustGetLogger("main")
)

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
	flag.IntVar(&fDebugLevel, "l", 4, "Debug leve (0 less verbose, to 5 most verbose) default to 4")
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
			log.Info("%v", http.ListenAndServe("localhost:6060", nil))

		}()
	}

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	mountPoint := path.Clean(flag.Arg(0))

	cfg := config.LoadConfig(fConfigPath)

	fs := filesystem.NewFS(mountPoint, cfg)

	log.Info("Mounting Fuse File system")
	if err := mount(fs, flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}
