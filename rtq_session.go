// Package rtq implements QUIC RTP Tunneling
// (https://tools.ietf.org/html/draft-hurst-quic-rtp-tunnelling-01)
package rtq

import (
	"bytes"
	"sync"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/quicvarint"
	"github.com/pion/transport/packetio"
)

type datagram struct {
	flowID uint64
	data   []byte
}

type Session struct {
	sess quic.Session

	readFlowsLock sync.RWMutex
	readFlows     map[uint64]*ReadFlow
}

type SessionOption func(r *Session) error

func NewSession(sess quic.Session, opts ...SessionOption) (*Session, error) {
	s := &Session{
		sess:          sess,
		readFlowsLock: sync.RWMutex{},
		readFlows:     make(map[uint64]*ReadFlow),
	}

	for _, opt := range opts {
		err := opt(s)
		if err != nil {
			return nil, err
		}
	}

	err := s.start()
	return s, err
}

func (s *Session) OpenWriteFlow(flowID uint64) (*WriteFlow, error) {
	return &WriteFlow{session: s, flowID: flowID}, nil
}

func (s *Session) AcceptFlow(flowID uint64) (*ReadFlow, error) {
	rf := &ReadFlow{
		session: s,
		buffer:  packetio.NewBuffer(),
	}
	s.readFlowsLock.Lock()
	s.readFlows[flowID] = rf
	s.readFlowsLock.Unlock()
	return rf, nil
}

func (s *Session) Close() error {
	panic("implement me")
}

func (s *Session) sendDatagram(d *datagram) error {
	buf := bytes.Buffer{}
	quicvarint.Write(&buf, d.flowID)
	buf.Write(d.data)
	return s.sess.SendMessage(buf.Bytes())
}

func (s *Session) start() error {
	go func() {
		for {
			message, err := s.sess.ReceiveMessage()
			if err != nil {
				// TODO: Log error? Check for io.EOF?
				return
			}
			reader := bytes.NewReader(message)
			flowID, err := quicvarint.Read(reader)
			if err != nil {
				// TODO: Handle invalid datagram
				return
			}
			flow := s.getFlow(flowID)
			if flow == nil {
				// TODO: Create flow?
			}
			_, err = flow.write(message[quicvarint.Len(flowID):])
			if err != nil {
				// TODO: Handle error?
				return
			}
		}
	}()
	return nil
}

func (s *Session) getFlow(id uint64) *ReadFlow {
	s.readFlowsLock.RLock()
	defer s.readFlowsLock.RUnlock()

	return s.readFlows[id]
}