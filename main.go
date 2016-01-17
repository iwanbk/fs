package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"net/http"
	_ "net/http/pprof"
	"net/url"

	"github.com/Jumpscale/aysfs/cache"
	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/ro"
	"github.com/Jumpscale/aysfs/metadata"
	"github.com/op/go-logging"
)

const (
	MetaEngineBolt = "bolt"
	MetaEngineMem  = "memory"
)

var (
	version = "0.1"
	log     = logging.MustGetLogger("main")
	boltdb  = path.Join(os.TempDir(), "aysfs.meta.db")
)

type Options struct {
	Version    bool
	Pprof      bool
	ConfigPath string
	AutoConfig bool
	LogLevel   int
	MetaEngine string
}

var progName = filepath.Base(os.Args[0])

func getCMDOptions() Options {
	opts := Options{}

	flag.BoolVar(&opts.Version, "v", false, "show version")
	flag.BoolVar(&opts.Pprof, "pprof", false, "enable net pprof")

	flag.StringVar(&opts.ConfigPath, "config", "config.toml", "path to config file")
	flag.BoolVar(&opts.AutoConfig, "auto", false, "enable auto configuration")
	flag.StringVar(&opts.ConfigPath, "c", "config.toml", "path to config file")
	flag.IntVar(&opts.LogLevel, "l", 4, "Log level (0 less verbose, to 5 most verbose) default to 4")
	flag.StringVar(&opts.MetaEngine, "meta", MetaEngineBolt, "Specify what metadata engine to use, default to 'bolt' other option is 'memory'")

	flag.Parse()
	flag.Usage = usage

	return opts
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", progName)
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", progName)
	fmt.Fprintf(os.Stderr, "\n")
	flag.PrintDefaults()
}

func configureLogging(options *Options) {
	logging.SetLevel(logging.Level(options.LogLevel), "")
	formatter := logging.MustStringFormatter("%{color}%{module} %{level:.1s} > %{message} %{color:reset}")
	logging.SetFormatter(formatter)
}

func watchReloadSignal(cfgPath string, fs *ro.FS) {
	channel := make(chan os.Signal)
	signal.Notify(channel, syscall.SIGUSR1)
	go func(cfgPath string, fs *ro.FS) {
		defer close(channel)
		for {
			<-channel
			log.Info("Reloading ays mounts due to user signal")

			func (){
				//Put the fs down to prevent any access to filesystem
				fs.Down()
				defer fs.Up()

				log.Debug("Puring metadata")
				// delete Metadata
				fs.PurgeMetadata()

				var metadataDir string

				if _, err := os.Stat(cfgPath); err == nil {
					cfg := config.LoadConfig(cfgPath)
					metadataDir = cfg.Main.Metadata
				}

				if metadataDir == "" {
					metadataDir = "/etc/ays/local"
				}

				fs.DiscoverMetadata(metadataDir)
			}()
		}
	}(cfgPath, fs)
}

func writePidFile() error {
	pid := fmt.Sprintf("%d", os.Getpid())
	return ioutil.WriteFile("/tmp/aysfs.pid", []byte(pid), 0600)
}

func main() {
	opts := getCMDOptions()
	if opts.Version {
		fmt.Println("Version: ", version)
		os.Exit(0)
	}

	configureLogging(&opts)

	if opts.Pprof {
		go func() {
			log.Info("%v", http.ListenAndServe("localhost:6060", nil))

		}()
	}

	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	writePidFile()

	mountPoint := path.Clean(flag.Arg(0))

	cacheMgr := cache.NewCacheManager()
	var meta metadata.Metadata

	switch opts.MetaEngine {
	case MetaEngineBolt:
		os.Remove(boltdb)
		if m, err := metadata.NewBoltMetadata(mountPoint, boltdb); err != nil {
			log.Fatal("Failed to intialize metaengine", err)
		} else {
			meta = m
		}
	case MetaEngineMem:
		if m, err := metadata.NewMemMetadata(mountPoint, nil); err != nil {
			log.Fatal("Failed to intialize metaengine", err)
		} else {
			meta = m
		}
	default:
		log.Fatal("Unknown metadata engine '%s'", opts.MetaEngine)
	}

	fs := ro.NewFS(mountPoint, meta, cacheMgr)
	var metadataDir string

	if opts.AutoConfig {
		fs.AutoConfigCaches()
	}

	if _, err := os.Stat(opts.ConfigPath); err == nil {
		cfg := config.LoadConfig(opts.ConfigPath)
		metadataDir = cfg.Main.Metadata
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
			} else if u.Scheme == "ssh" {
				layer, err := cache.NewSFTPCache(c.URL, "dedupe")
				if err != nil {
					log.Fatalf("Failed to intialize cach layer '%s': %s", c.URL, err)
				}
				cacheMgr.AddLayer(layer)
			}
		}
	}

	if metadataDir == "" {
		// TODO Make portable
		metadataDir = "/etc/ays/local"
	}

	//purge all purgable cache layers.
	fs.DiscoverMetadata(metadataDir)

	fmt.Println(fs)

	watchReloadSignal(opts.ConfigPath, fs)

	//bring fileystem UP
	fs.Up()

	log.Info("Mounting Fuse File system")
	if err := mount(fs, mountPoint); err != nil {
		log.Fatal(err)
	}

}
