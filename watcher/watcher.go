package watcher

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"

	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/tracker"
	"github.com/jeffail/tunny"
	"github.com/op/go-logging"
	"github.com/robfig/cron"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"
)

const (
	MaxWorkers = 10
)

var (
	log = logging.MustGetLogger("watcher")
)

func Start() {

}

type backenWatcher struct {
	backend *config.Backend
	stor    *config.Aydostor
	pool    *tunny.WorkPool
}

func NewWatcher(backend *config.Backend, stor *config.Aydostor) cron.Job {
	watcher := &backenWatcher{
		backend: backend,
		stor:    stor,
	}

	watcher.pool = tunny.CreatePool(MaxWorkers, watcher.process)
	return watcher
}

func (w *backenWatcher) url(hash string) (string, error) {
	u, err := url.Parse(w.stor.Addr)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, w.backend.Namespace, hash)

	return u.String(), nil
}

func (w *backenWatcher) Run() {
	log.Debugf("Watcher is awake, checking tracker file...")
	for name := range tracker.IterReady() {
		w.pool.SendWorkAsync(name, nil)
	}
}

func (w *backenWatcher) process(nameI interface{}) interface{} {
	name, _ := nameI.(string)
	log.Info("Processing file '%s'", name)
	if err := w.processFile(name); err != nil {
		log.Errorf("Failed to process file '%s'", err)
	}
	return nil
}

func (w *backenWatcher) processFile(name string) error {
	backup, err := w.backup(name)
	if err != nil {
		return err
	}

	defer os.RemoveAll(backup)

	file, err := os.Open(backup)
	if err != nil {
		return nil
	}
	defer file.Close()

	fileHash, err := w.hash(file)
	if err != nil {
		return err
	}
	file.Seek(0, os.SEEK_SET)

	//TODO: Write MetaFile
	url, err := w.url(fileHash)
	if err != nil {
		return err
	}

	return w.put(url, file)
}

// backup copy the pointed by name to name_{timestamp}.aydo
func (w *backenWatcher) backup(name string) (string, error) {
	// TODO lock write on the file

	src, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer src.Close()

	backup := fmt.Sprintf("%s_%d.aydo", name, time.Now().UnixNano())
	dst, err := os.OpenFile(backup, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return backup, nil
}

//Hash compute the md5sum of the reader r
func (w *backenWatcher) hash(r io.Reader) (string, error) {
	h := md5.New()
	_, err := io.Copy(h, r)
	if err != nil {
		log.Errorf("Hash, Error reading source: %v", err)
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (s *backenWatcher) put(url string, r io.Reader) error {
	response, err := http.Post(url, "application/octet-stream", r)
	if err != nil {
		log.Errorf("Error during uploading of file: %v", err)
		return err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		body, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("Failed to upload file. Invalid response from stor (%d): %s", response.StatusCode, body)
	}

	return nil
}
