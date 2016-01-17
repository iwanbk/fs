package rw

import (
	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
	"io"
	"os"
)

type fsFile struct {
	fsBase
	parent *fsDir
}

type fsFileHandle struct {
	file *os.File
}

func newFile(path string, parent *fsDir) *fsFile {
	return &fsFile{
		fsBase: fsBase{
			path: path,
		},
		parent: parent,
	}
}

func (n *fsFile) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	log.Debugf("Opening file '%s' (%s)", n.path, req.Flags)

	file, err := os.OpenFile(n.path, int(uint32(req.Flags)), os.ModePerm)
	if err != nil {
		return nil, err
	}

	return &fsFileHandle{
		file: file,
	}, nil
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

func (h *fsFileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	log.Debugf("Closing file descriptor")
	return h.file.Close()
}
