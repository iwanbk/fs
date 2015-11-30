package filesystem

import (
	"fmt"
	"io"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/net/context"
)

const (
	FileReadBuffer = 512 * 1024 //bytes [512K]
)

type FileBuffer interface {
	fs.Handle
	fs.HandleReleaser
	fs.HandleReader
}

type fileBufferImpl struct {
	file   File
	buffer []byte
	offset int64
	size   int64
}

func NewFileBuffer(file File) FileBuffer {
	return &fileBufferImpl{
		file:   file,
		buffer: make([]byte, FileReadBuffer),
	}
}

func (f *fileBufferImpl) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	f.file.Release()
	return nil
}

func (f *fileBufferImpl) available(offset int64, size int64) bool {
	diff := offset - f.offset
	return offset >= f.offset && diff+size <= f.size
}

func (f *fileBufferImpl) readFromOffset(offset int64, min int) error {
	if len(f.buffer) < min {
		return fmt.Errorf("ReadAhead buffer to small for the read requst %d/%d", min, len(f.buffer))
	}

	size, err := f.file.Read(offset, f.buffer)
	f.offset = offset
	f.size = int64(size)

	if err != nil && err != io.EOF {
		return err
	}

	return nil
}

func (f *fileBufferImpl) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	if !f.available(req.Offset, int64(req.Size)) {
		err := f.readFromOffset(req.Offset, req.Size)
		if err != nil {
			return err
		}
	}

	//buffer is ready. Just copy data.
	offset := req.Offset - f.offset
	resp.Data = f.buffer[offset : offset+int64(req.Size)]

	return nil
}
