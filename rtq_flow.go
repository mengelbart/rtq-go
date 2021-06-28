package rtq

import (
	"errors"
	"io"

	"github.com/pion/rtp"
	"github.com/pion/transport/packetio"
)

type WriteFlow struct {
	flowID  uint64
	session *Session
}

func (w *WriteFlow) Write(b []byte) (n int, err error) {
	return len(b), w.session.sendDatagram(&datagram{
		flowID: w.flowID,
		data:   b,
	})
}

func (w *WriteFlow) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	headerBuf, err := header.Marshal()
	if err != nil {
		return 0, err
	}
	return w.Write(append(headerBuf, payload...))
}

type ReadFlow struct {
	session *Session
	buffer  io.ReadWriteCloser
}

func (r *ReadFlow) write(buf []byte) (int, error) {
	n, err := r.buffer.Write(buf)
	if errors.Is(err, packetio.ErrFull) {
		// Silently drop data when the buffer is full.
		return len(buf), nil
	}
	return n, err
}

func (r *ReadFlow) Read(buf []byte) (n int, err error) {
	return r.buffer.Read(buf)
}

func (r *ReadFlow) ReadRTP(buf []byte) (int, *rtp.Header, error) {
	n, err := r.Read(buf)
	if err != nil {
		return 0, nil, err
	}

	header := &rtp.Header{}

	err = header.Unmarshal(buf[:n])
	if err != nil {
		return 0, nil, err
	}

	return n, header, nil
}

func (r *ReadFlow) close() error {
	return r.buffer.Close()
}
