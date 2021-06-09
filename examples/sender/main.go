package main

import (
	"crypto/tls"
	"log"

	"github.com/lucas-clemente/quic-go"
	"github.com/mengelbart/qrt"
	gstsrc "github.com/mengelbart/qrt/examples/internal/gstreamer-src"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

func main() {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	err := run(":4242", tlsConf)
	if err != nil {
		log.Fatal(err)
	}
}

type gstWriter struct {
	targetBitrate int64
	qrtSession    *qrt.Session
	pipeline      *gstsrc.Pipeline
	rtcpReader    interceptor.RTCPReader
	rtpWriter     interceptor.RTPWriter
}

func (g *gstWriter) Write(p []byte) (n int, err error) {
	var pkt rtp.Packet
	err = pkt.Unmarshal(p)
	if err != nil {
		return 0, err
	}
	return g.rtpWriter.Write(&pkt.Header, p[pkt.Header.MarshalSize():], nil)
}

func run(addr string, tlsConf *tls.Config) error {
	quicSession, err := quic.DialAddr(addr, tlsConf, &quic.Config{EnableDatagrams: true})
	if err != nil {
		return err
	}
	qrtSession, err := qrt.NewSession(quicSession)
	if err != nil {
		return err
	}
	rtpFlow, err := qrtSession.OpenWriteFlow(0)
	if err != nil {
		return err
	}

	chain := interceptor.NewChain([]interceptor.Interceptor{})
	streamWriter := chain.BindLocalStream(&interceptor.StreamInfo{}, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		return rtpFlow.WriteRTP(header, payload)
	}))

	writer := &gstWriter{
		qrtSession: qrtSession,
		rtpWriter:  streamWriter,
	}

	pipeline, err := gstsrc.NewPipeline("vp8", "videotestsrc", writer)
	if err != nil {
		return err
	}
	log.Printf("created pipeline: '%v'\n", pipeline.String())
	writer.pipeline = pipeline
	pipeline.SetSSRC(0)
	pipeline.Start()

	select {}
}
