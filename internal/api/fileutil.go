package api

import (
	"io"
	"os"
)

// fileReader wraps os.File for reading from a specific position.
type fileReader struct {
	f *os.File
}

func openFile(path string) (*fileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &fileReader{f: f}, nil
}

func (fr *fileReader) Read(p []byte) (int, error) {
	return fr.f.Read(p)
}

func (fr *fileReader) Seek(offset int64, whence int) (int64, error) {
	return fr.f.Seek(offset, whence)
}

func (fr *fileReader) Close() error {
	return fr.f.Close()
}

var _ io.ReadSeeker = (*fileReader)(nil)
