package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/qlog"
	"github.com/mengelbart/rtq"
	gstsrc "github.com/mengelbart/rtq/examples/internal/gstreamer-src"
	"github.com/mengelbart/rtq/examples/internal/utils"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

func main() {
	logFilename := os.Getenv("LOG_FILE")
	if logFilename == "" {
		logFilename = "/logs/log.txt"
	}
	logfile, err := os.Create(logFilename)
	if err != nil {
		fmt.Printf("Could not create log file: %s\n", err.Error())
		os.Exit(1)
	}
	defer logfile.Close()
	log.SetOutput(logfile)

	qlogWriter, err := utils.GetQLOGWriter()
	if err != nil {
		log.Printf("Could not get qlog writer: %s\n", err.Error())
		os.Exit(1)
	}

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}

	quicConf := &quic.Config{
		EnableDatagrams: true,
	}
	if qlogWriter != nil {
		quicConf.Tracer = qlog.NewTracer(qlogWriter)
	}

	err = run(":4242", tlsConf, quicConf)
	if err != nil {
		log.Printf("Could not run sender: %v\n", err.Error())
		os.Exit(1)
	}
}

type gstWriter struct {
	targetBitrate int64
	rtqSession    *rtq.Session
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

func run(addr string, tlsConf *tls.Config, quicConf *quic.Config) error {
	quicSession, err := quic.DialAddr(addr, tlsConf, quicConf)
	if err != nil {
		return err
	}
	rtqSession, err := rtq.NewSession(quicSession)
	if err != nil {
		return err
	}
	rtpFlow, err := rtqSession.OpenWriteFlow(0)
	if err != nil {
		return err
	}

	chain := interceptor.NewChain([]interceptor.Interceptor{})
	streamWriter := chain.BindLocalStream(&interceptor.StreamInfo{}, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		return rtpFlow.WriteRTP(header, payload)
	}))

	writer := &gstWriter{
		rtqSession: rtqSession,
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

	go gstsrc.StartMainLoop()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	destroyed := make(chan struct{}, 1)
	gstsrc.HandleSinkEOS(func() {
		log.Println("destroy pipeline")
		pipeline.Destroy()
		destroyed <- struct{}{}
	})

	select {
	case <-signals:
		log.Printf("got interrupt signal")
	}

	pipeline.Stop()

	<-destroyed
	log.Println("destroyed pipeline, exiting")
	return err

}
