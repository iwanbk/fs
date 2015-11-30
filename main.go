package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"net/http"
	_ "net/http/pprof"
	"net/url"

	"path"

	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/filesystem"
	"github.com/op/go-logging"

	"os/signal"
	"syscall"
)

var (
	version = "0.1"
	log     = logging.MustGetLogger("main")
)

var (
	fVersion    bool
	fPprof      bool
	fConfigPath string
	fAutoConfig bool
	fDebugLevel int
)

var progName = filepath.Base(os.Args[0])

func init() {
	flag.BoolVar(&fVersion, "v", false, "show version")
	flag.BoolVar(&fPprof, "pprof", false, "enable net pprof")

	flag.StringVar(&fConfigPath, "config", "config.toml", "path to config file")
	flag.BoolVar(&fAutoConfig, "auto", false, "enable auto configuration")
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

func watchReloadSignal(path string, auto bool, fs *filesystem.FS) {
	channel := make(chan os.Signal)
	signal.Notify(channel, syscall.SIGUSR1)
	go func(path string, fs *filesystem.FS) {
		defer close(channel)
		for {
			<-channel
			log.Info("Reloading ays mounts due to user signal")

			if auto {
				fs.DiscoverMetadata("/etc/ays/local")
			} else if !auto && path != "" {
				cfg := config.LoadConfig(path)
				for _, a := range cfg.Ays {
					fs.AttachFList(a.ID)
				}
			}
		}
	}(path, fs)
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

	cacheMgr := cache.NewCacheManager()
	fs := filesystem.NewFS(mountPoint, cacheMgr)
	var cfg *config.Config

	if fAutoConfig {
		fs.AutoConfigCaches()
		fs.DiscoverMetadata("/etc/ays/local")
	} else {
		cfg = config.LoadConfig(fConfigPath)
		// attaching cache layers to the fs
		for _, c := range cfg.Cache {
			u, err := url.Parse(c.URL)
			if err != nil {
				log.Fatalf("Invalid URL for cache '%s'", c.URL)
			}
			if u.Scheme == "" || u.Scheme == "file" {
				//add FS layer
				cacheMgr.AddLayer(cache.NewFSCache(u.Path, "dedupe", c.Purge))
			} else if u.Scheme == "http" || u.Scheme == "https" {
				if c.Purge {
					log.Warning("HTTP cache '%s' doesn't support purging", c.URL)
				}
				cacheMgr.AddLayer(cache.NewHTTPCache(c.URL, "dedupe"))
			}

		}
	}

	fmt.Println(fs)

	//purge all purgable cache layers.
	cacheMgr.Purge()

	if !fAutoConfig && cfg != nil {
		//now adding the AYS lists.
		for _, a := range cfg.Ays {
			fs.AttachFList(a.ID)
		}
	}

	watchReloadSignal(fConfigPath, fAutoConfig, fs)

	log.Info("Mounting Fuse File system")
	if err := mount(fs, flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
}
