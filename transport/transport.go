package transport

import (
	"os"
	"time"
)

type ClientTransport interface {
	Get(path string, dst *os.File) (int64, error)
}

type Transport interface {
	List(path string) ([]DirEntry, error)
	Fetch(path string, size int64, dst *os.File) (int64, error)
}

type DirEntry struct {
	IsDir   bool
	Name    string
	Mode    os.FileMode
	Size    int64
	ModTime time.Time
}
