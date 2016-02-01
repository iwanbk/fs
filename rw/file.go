package rw

import (
	"bytes"
	"fmt"

	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/Jumpscale/aysfs/crypto"
	"github.com/Jumpscale/aysfs/rw/meta"
	"github.com/Jumpscale/aysfs/tracker"
	"github.com/Jumpscale/aysfs/utils"
	"github.com/dsnet/compress/brotli"
	"golang.org/x/net/context"
)

type fsFile struct {
	fsBase
	fs     *FS
	parent *fsDir
}

type fsFileHandle struct {
	file    *os.File
	tracker tracker.Tracker
}

func newFile(fs *FS, path string, parent *fsDir) *fsFile {
	return &fsFile{
		fsBase: fsBase{
			path: path,
		},
		fs:     fs,
		parent: parent,
	}
}

func (n *fsFile) MetaPath() string {
	return fmt.Sprintf("%s%s", n.path, meta.MetaSuffix)
}

func (n *fsFile) Meta() (*meta.MetaFile, error) {
	m := meta.GetMeta(n.path)
	return m.Load()
}

func (n *fsFile) url(hash string) (string, error) {
	u, err := url.Parse(n.fs.Stor().Addr)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "store", n.fs.Backend().Namespace, hash)

	return u.String(), nil
}

func (n *fsFile) download() error {
	log.Infof("Downloading file '%s'", n.path)
	meta, err := n.Meta()
	if err != nil {
		log.Errorf("Failed to download due to metadata loading failed: %s", err)
		return err
	}

	url, err := n.url(meta.Hash)
	if err != nil {
		log.Errorf("Failed to build file url: %s", err)
		return err
	}

	log.Info("Downloading: %s", url)

	response, err := http.Get(url)
	if err != nil {
		log.Errorf("Failed to download file from stor: %s", err)
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(response.Body)
		log.Errorf("Invalid response from stor(%d): %s", response.StatusCode, body)
		return fuse.ENOENT
	}

	broReader := brotli.NewReader(response.Body)

	file, err := os.OpenFile(n.path, os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	if n.fs.Backend().Encrypted {
		if meta.UserKey == "" {
			err := fmt.Errorf("encryption key is empty, can't decrypt file %v", n.path)
			log.Error(err.Error())
			return err
		}

		r := bytes.NewBuffer([]byte(meta.UserKey))
		bKey := []byte{}
		fmt.Fscanf(r, "%x", &bKey)

		sessionKey, err := crypto.DecryptAsym(n.fs.backend.ClientKey, bKey)
		if err != nil {
			log.Errorf("Error decrypting session key: %v", err)
			return err
		}

		if err := crypto.DecryptSym(sessionKey, broReader, file); err != nil {
			log.Errorf("Error decrypting data: %v", err)
			return err
		}
	} else {
		if _, err = io.Copy(file, broReader); err != nil {
			log.Errorf("Error downloading data: %v", err)
			return err
		}
	}

	return err
}

func (n *fsFile) open(flags fuse.OpenFlags) (fs.Handle, error) {
	log.Debugf("Opening file '%s' (%s)", n.path, flags)

	file, err := os.OpenFile(n.path, int(uint32(flags)), os.ModePerm)
	if os.IsNotExist(err) {
		//probably ReadOnly mode. if meta exist, get the file from stor.
		if err := n.download(); err != nil {
			return nil, utils.ErrnoFromPathError(err)
		} else {
			return n.open(flags)
		}
	} else if err != nil {
		return nil, utils.ErrnoFromPathError(err)
	}

	return &fsFileHandle{
		file:    file,
		tracker: n.fs.tracker,
	}, nil
}

func (n *fsFile) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	return n.open(req.Flags)
}

func (n *fsFile) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	return nil
}

func (n *fsFile) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	if req.Valid.Size() {
		os.Truncate(n.path, int64(req.Size))
	}
	return nil
}

func (h *fsFileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	buffer := make([]byte, req.Size)
	n, err := h.file.ReadAt(buffer, req.Offset)
	if err != nil && err != io.EOF {
		log.Errorf("Reading file failed: %s", err)
		return err
	}

	resp.Data = buffer[:n]
	return nil
}

func (h *fsFileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	defer h.tracker.Touch(h.file.Name())

	n, err := h.file.WriteAt(req.Data, req.Offset)
	if err != nil {
		return err
	}

	resp.Size = n
	return nil
}

func (h *fsFileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	log.Debugf("Closing file descriptor")
	defer h.tracker.Close(h.file.Name())
	return h.file.Close()
}
