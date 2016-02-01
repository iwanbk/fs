package watcher

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"

	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/Jumpscale/aysfs/config"
	"github.com/Jumpscale/aysfs/crypto"
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/Jumpscale/aysfs/tracker"
	"github.com/Jumpscale/aysfs/utils"
	"github.com/jeffail/tunny"
	"github.com/op/go-logging"
	"github.com/robfig/cron"
	bro "gopkg.in/kothar/brotli-go.v0/enc"
)

const (
	MaxWorkers = 10
)

var (
	log = logging.MustGetLogger("watcher")
)

type backenWatcher struct {
	backend *config.Backend
	stor    *config.Aydostor
	pool    *tunny.WorkPool
	tracker tracker.Tracker

	url string

	logger TLogger
}

type encrypted struct {
	file      io.Reader
	userKey   string
	globalKey string
}

func NewWatcher(backend *config.Backend, stor *config.Aydostor, tracker tracker.Tracker) (cron.Job, error) {
	logFile := backend.Log
	if logFile == "" {
		logFile = path.Join(os.TempDir(), fmt.Sprintf("aydofs.%s.log", backend.Name))
	}

	watcher := &backenWatcher{
		backend: backend,
		stor:    stor,
		logger:  NewTLogger(logFile),
		tracker: tracker,
	}

	url, err := watcher.getUrl()
	if err != nil {
		return nil, err
	}
	watcher.url = url

	pool, err := tunny.CreatePool(MaxWorkers, watcher.process).Open()
	if err != nil {
		return nil, err
	}
	watcher.pool = pool

	return watcher, nil
}

func (w *backenWatcher) getUrl() (string, error) {
	u, err := url.Parse(w.stor.Addr)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "store", w.backend.Namespace)

	return u.String(), nil
}

func (w *backenWatcher) Run() {
	log.Debugf("Watcher is awake, checking tracker file...")
	for name := range w.tracker.IterReady() {
		w.pool.SendWorkAsync(name, nil)
	}
}

func (w *backenWatcher) process(nameI interface{}) interface{} {
	name, _ := nameI.(string)
	log.Info("Processing file '%s'", name)
	if err := w.processFile(name); err == nil || os.IsNotExist(err) {
		log.Info("File '%s' processing completed successfully", name)
		w.tracker.Forget(name)
	} else {
		log.Errorf("Failed to process file '%s'", err)
	}

	return nil
}

func (w *backenWatcher) encrypt(fileHash string, file io.Reader) (*encrypted, error) {
	// encrypt file
	enc := &encrypted{}

	sessionKey := crypto.CreateSessionKey(fileHash)

	out, err := crypto.EncryptSymStream(sessionKey, file)
	if err != nil {
		return nil, err
	}

	encryptedKey, err := crypto.EncryptAsym(&w.backend.ClientKey.PublicKey, sessionKey)
	if err != nil {
		log.Errorf("Error encrypted session with client key:%v", err)
		return nil, err
	}

	enc.userKey = fmt.Sprintf("%x", encryptedKey)

	encryptedKey, err = crypto.EncryptAsym(&w.backend.GlobalKey.PublicKey, sessionKey)
	if err != nil {
		log.Errorf("Error encrypted session with store key:%v", err)
		return nil, err
	}

	enc.globalKey = fmt.Sprintf("%x", encryptedKey)
	enc.file = out

	return enc, nil
}

func (w *backenWatcher) compress(in io.Reader) (io.Reader, error) {
	reader, writer := io.Pipe()

	brotliWriter := bro.NewBrotliWriter(nil, writer)

	go func() {
		defer brotliWriter.Close()
		io.Copy(brotliWriter, in)
	}()

	return reader, nil
}

func (w *backenWatcher) processFile(name string) error {
	backup, err := w.backup(name)
	if err != nil {
		return err
	}

	defer os.RemoveAll(backup)

	var reader io.Reader
	file, err := os.Open(backup)
	if err != nil {
		return nil
	}
	defer file.Close()

	//initially the reader IS the file.
	reader = file
	stat, err := file.Stat()
	if err != nil {
		return err
	}

	m := &meta.MetaFile{
		Size: uint64(stat.Size()),
	}

	//Building the stream pipes
	//1- Encrypt if enabled.
	if w.backend.Encrypted {
		plainHash, err := w.hash(file)
		if err != nil {
			return err
		}

		file.Seek(0, os.SEEK_SET)

		enc, err := w.encrypt(plainHash, file)
		if err != nil {
			return err
		}

		m.UserKey = enc.userKey
		m.StoreKey = enc.globalKey

		reader = enc.file
	}

	//2- Compression
	reader, err = w.compress(reader)
	if err != nil {
		return err
	}

	var hasher *utils.Hasher
	//recalculate the hash of the encrypted file
	hasher, reader = utils.NewHasher(reader)

	//3- Upload
	if err := w.put(reader); err != nil {
		return err
	}

	m.Hash = hasher.Hash()
	mf := meta.GetMeta(name)
	err = mf.Save(m)
	if err != nil {
		return err
	}

	w.logger.Log(name, m.Hash)
	return nil
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

func (w *backenWatcher) put(r io.Reader) error {
	response, err := http.Post(w.url, "application/octet-stream", r)
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
