package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"net/http"
	_ "net/http/pprof"

	"sync"

	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/watcher"
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

//func watchReloadSignal(cfgPath string, fs *ro.FS) {
//	channel := make(chan os.Signal)
//	signal.Notify(channel, syscall.SIGUSR1)
//	go func(cfgPath string, fs *ro.FS) {
//		defer close(channel)
//		for {
//			<-channel
//			log.Info("Reloading ays mounts due to user signal")
//
//			func (){
//				//Put the fs down to prevent any access to filesystem
//				fs.Down()
//				defer fs.Up()
//
//				log.Debug("Puring metadata")
//				// delete Metadata
//				fs.PurgeMetadata()
//
//				var metadataDir string
//
//				if _, err := os.Stat(cfgPath); err == nil {
//					cfg := config.LoadConfig(cfgPath)
//					metadataDir = cfg.Main.Metadata
//				}
//
//				if metadataDir == "" {
//					metadataDir = "/etc/ays/local"
//				}
//
//				fs.DiscoverMetadata(metadataDir)
//			}()
//		}
//	}(cfgPath, fs)
//}

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

	cfg := config.LoadConfig(opts.ConfigPath)

	wg := sync.WaitGroup{}

	for _, mountCfg := range cfg.Mount {
		if mountCfg.Flist != "" {
			log.Infof("Mount Read only FS on %s", mountCfg.Path)

			wg.Add(1)
			go func(mountCfg config.Mount, opts Options) {
				MountROFS(mountCfg, opts)
				wg.Done()
			}(mountCfg, opts)
		} else {
			log.Infof("Mount Read write FS on %s", mountCfg.Path)

			backend, err := cfg.GetBackend(mountCfg.Backend)
			if err != nil {
				log.Fatalf("Definition of backend %s not found in config, but required for mount %s", mountCfg.Backend, mountCfg.Path)
			}
			stor, err := cfg.GetStor(backend.Stor)
			if err != nil {
				log.Fatalf("Definition of ayostor %s not found in config, but required for backend %s", backend.Stor, backend.Name)
			}

			wg.Add(1)
			os.MkdirAll(backend.Path, 0775)
			go func(mountCfg config.Mount, backend config.Backend, stor config.Aydostor, opts Options) {
				MountRWFS(mountCfg, &backend, &stor)

				watcher.SetIntervals(time.Minute, time.Hour)
				watcher.SetBackend(&backend)
				watcher.SetStore(&stor)
				watcher.Start()

				wg.Done()
			}(mountCfg, backend, stor, opts)
		}
	}

	wg.Wait()
}
