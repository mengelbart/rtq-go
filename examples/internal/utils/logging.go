package utils

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/lucas-clemente/quic-go/logging"
)

type bufferedWriteCloser struct {
	*bufio.Writer
	io.Closer
}

// NewBufferedWriteCloser creates an io.WriteCloser from a bufio.Writer and an io.Closer
func newBufferedWriteCloser(writer *bufio.Writer, closer io.Closer) io.WriteCloser {
	return &bufferedWriteCloser{
		Writer: writer,
		Closer: closer,
	}
}

func (h bufferedWriteCloser) Close() error {
	if err := h.Writer.Flush(); err != nil {
		return err
	}
	return h.Closer.Close()
}

// GetQLOGWriter creates the QLOGDIR and returns the GetLogWriter callback
func GetQLOGWriter() (func(perspective logging.Perspective, connID []byte) io.WriteCloser, error) {
	qlogDir := os.Getenv("QLOGDIR")
	if len(qlogDir) == 0 {
		return nil, nil
	}
	_, err := os.Stat(qlogDir)
	if err!=nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(qlogDir, 0o666); err != nil {
				return nil, fmt.Errorf("failed to create qlog dir %s: %s", qlogDir, err.Error())
			}
		} else {
			return nil, err
		}
	}
	return func(_ logging.Perspective, connID []byte) io.WriteCloser {
		path := fmt.Sprintf("%s/%x.qlog", strings.TrimRight(qlogDir, "/"), connID)
		f, err := os.Create(path)
		if err != nil {
			log.Printf("Failed to create qlog file %s: %s", path, err.Error())
			return nil
		}
		log.Printf("Created qlog file: %s\n", path)
		return newBufferedWriteCloser(bufio.NewWriter(f), f)
	}, nil
}
