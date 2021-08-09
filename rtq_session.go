// Package rtq implements QUIC RTP Tunneling
// (https://tools.ietf.org/html/draft-hurst-quic-rtp-tunnelling-01)
package rtq

import (
	"bytes"
	"fmt"
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
	return s.sess.CloseWithError(0, "eos")
}

func (s *Session) sendDatagram(d *datagram) error {
	buf := bytes.Buffer{}
	quicvarint.Write(&buf, d.flowID)
	buf.Write(d.data)
	return s.sess.SendMessage(buf.Bytes())
}

func (s *Session) sendDatagramNotify(d *datagram, cb func(bool)) error {
	buf := bytes.Buffer{}
	quicvarint.Write(&buf, d.flowID)
	buf.Write(d.data)
	return s.sess.SendMessageNotify(buf.Bytes(), cb)
}

func (s *Session) start() error {
	go func() {
		for {
			message, err := s.sess.ReceiveMessage()
			if err != nil {
				if err.Error() == "Application error 0x0: eos" {
					s.readFlowsLock.Lock()
					for _, flow := range s.readFlows {
						err = flow.close()
						if err != nil {
							fmt.Printf("failed to close flow: %s\n", err)
						}
					}
					s.readFlowsLock.Unlock()
					return
				}
				fmt.Printf("failed to receive message: %v\n", err)
				return
			}
			reader := bytes.NewReader(message)
			flowID, err := quicvarint.Read(reader)
			if err != nil {
				fmt.Printf("failed to parse flow identifier from message of length: %v: %s\n", len(message), err)
				return
			}
			flow := s.getFlow(flowID)
			if flow == nil {
				// TODO: Create flow?
				fmt.Printf("dropping message for unknown flow, forgot to create flow with ID %v?", flowID)
				continue
			}
			_, err = flow.write(message[quicvarint.Len(flowID):])
			if err != nil {
				// TODO: Handle error?
				fmt.Printf("// TODO: handler flow.write error: %s\n", err)
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
