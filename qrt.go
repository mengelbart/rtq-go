// Package qrt implements QUIC RTP Tunneling
// (https://tools.ietf.org/html/draft-hurst-quic-rtp-tunnelling-01)
package qrt

import (
	"github.com/lucas-clemente/quic-go"
	"github.com/pion/rtp"
)

type Session struct {
	sess quic.Session
}

func NewSession(sess quic.Session) (*Session, error) {
	return &Session{
		sess: sess,
	}, nil
}

func (s *Session) OpenWriteFlow() (*WriteFlow, error) {
	return &WriteFlow{session: s}, nil
}

func (s *Session) AcceptFlow() (*ReadFlow, error) {
	return &ReadFlow{session: s}, nil
}

func (s *Session) Close() error {
	panic("implement me")
}

type WriteFlow struct {
	session *Session
}

func (w *WriteFlow) Write(b []byte) (n int, err error) {
	return len(b), w.session.sess.SendMessage(b)
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
}

func (r *ReadFlow) Read(buf []byte) (n int, err error) {
	message, err := r.session.sess.ReceiveMessage()
	if err != nil {
		return 0, err
	}
	// TODO: Check any length constraints?
	n = copy(buf, message)
	return n, nil
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
