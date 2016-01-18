package watcher

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/op/go-logging"
)

var (
	log = logging.MustGetLogger("watcher")
)

func Start() {
	log.Info("Start watcher")
	go startWatching()
}

func startWatching() {
	ticker := time.NewTicker(cfg.StoreInterval)

	for range ticker.C {
		dir, err := os.Open(cfg.Backend.Path)
		if err != nil {
			log.Errorf("Error opening backend root: %v", err)
			dir.Close()
			continue
		}

		names, err := dir.Readdirnames(-1)
		if err != nil {
			log.Errorf("Error reading files in backend root: %v", err)
			dir.Close()
			continue
		}

		for _, name := range names {
			err = processFile(name)
			if err != nil {
				log.Errorf("Error processing backend: %v", err)
				dir.Close()
				continue
			}
		}

		dir.Close()
	}
}

func processFile(name string) error {

	if err := copy(name); err != nil {
		log.Errorf("Error copying %s to %s_ :%v", name, name, err)
		return err
	}

	f, err := os.Open(name + "_")
	defer f.Close()
	if err != nil {
		log.Errorf("Error opening file %s_ :%v", name, err)
		return err
	}

	// h, err := hash(f)
	// if err != nil {
	// 	log.Errorf("Error hashing file %s_ :%v", name, err)
	// 	return err
	// }

	// tlog := &TLog{
	// 	Hash:  h,
	// 	Path:  name,
	// 	Epoch: time.Now(),
	// }
	// TODO append to Tlog

	if err := storeClient.PutFile(cfg.Backend.Namespace, f); err != nil {
		log.Errorf("Error uploading %s to store %s: %v", name, storeClient.Addr, err)
		return err
	}

	// TODO write Meta

	if err := os.Remove(name + "_"); err != nil {
		log.Errorf("Error deleting %s_ :%v", name, err)
		return err
	}

	return nil
}

// Copy copy the pointed by name to name_
func copy(name string) error {
	// TODO lock write on the file
	src, err := os.Open(name)
	defer src.Close()
	if err != nil {
		return err
	}

	dst, err := os.OpenFile(name+"_", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	defer dst.Close()
	if err != nil {
		return err
	}

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}

//Hash compute the md5sum of the reader r
func hash(r io.Reader) (string, error) {
	s, ok := r.(io.Seeker)
	if ok {
		if _, err := s.Seek(0, os.SEEK_SET); err != nil {
			return "", err
		}
	}

	h := md5.New()
	_, err := io.Copy(h, r)
	if err != nil {
		log.Errorf("Hash, Error reading source: %v", err)
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
