//+build scream

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"log"
	"math/big"

	"github.com/mengelbart/qrt"
	gstsink "github.com/mengelbart/qrt/examples/internal/gstreamer-sink"
	gstsrc "github.com/mengelbart/qrt/examples/internal/gstreamer-src"

	"github.com/lucas-clemente/quic-go"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/scream"
	_ "github.com/pion/mediadevices/pkg/driver/camera" // This is required to register camera adapter
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

const (
	mtu = 1000
)

var addr string

func main() {

	a := flag.String("addr", "localhost:4242", "address to bind/connect to")
	client := flag.Bool("client", false, "Run as client, i.e. sending side")
	server := flag.Bool("server", false, "Run as server, i.e. receiving side")
	flag.Parse()

	addr = *a

	if !*client && !*server {
		flag.Usage()
		return
	}

	if *server {
		err := qrtServer()
		if err != nil {
			log.Fatalf("server crashed: %v", err)
		}
		return
	}

	if *client {
		err := qrtClient()
		if err != nil {
			log.Fatalf("client crashed: %v", err)
		}
	}
}

func qrtServer() error {

	listener, err := quic.ListenAddr(addr, generateTLSConfig(), &quic.Config{EnableDatagrams: true})
	if err != nil {
		return err
	}
	quicSession, err := listener.Accept(context.Background())
	if err != nil {
		return err
	}

	qrtSession, err := qrt.NewSession(quicSession)
	if err != nil {
		return err
	}

	rtpFlow, err := qrtSession.AcceptFlow()
	if err != nil {
		return err
	}

	feedback, err := scream.NewReceiverInterceptor()
	if err != nil {
		return err
	}
	chain := interceptor.NewChain([]interceptor.Interceptor{feedback})
	streamReader := chain.BindRemoteStream(&interceptor.StreamInfo{
		SSRC:         0,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "ack", Parameter: "ccfb"}},
	}, interceptor.RTPReaderFunc(func(bytes []byte, _ interceptor.Attributes) (int, interceptor.Attributes, error) {
		return len(bytes), nil, nil
	}))

	pipeline := gstsink.CreatePipeline()
	pipeline.Start()

	go func() {
		for rtcpBound, buffer := false, make([]byte, mtu); ; {
			n, err := rtpFlow.Read(buffer)
			if err != nil {
				panic(err)
			}
			pipeline.Push(buffer[:n])

			if _, _, err := streamReader.Read(buffer[:n], nil); err != nil {
				panic(err)
			}

			if !rtcpBound {
				rtcpFlow, err := qrtSession.OpenWriteFlow()
				if err != nil {
					panic(err)
				}

				chain.BindRTCPWriter(interceptor.RTCPWriterFunc(func(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
					buf, err := rtcp.Marshal(pkts)
					if err != nil {
						return 0, err
					}
					return rtcpFlow.Write(buf)
				}))

				rtcpBound = true
			}
		}
	}()
	gstsink.StartMainLoop()
	return nil
}

type gstWriter struct {
	targetBitrate int64
	qrtSession    *qrt.Session
	rtcpReader    interceptor.RTCPReader
	rtpWriter     interceptor.RTPWriter
	scream        *scream.SenderInterceptor
	pipeline      *gstsrc.Pipeline
}

func (g *gstWriter) acceptFeedback() {
	rtcpFlow, err := g.qrtSession.AcceptFlow()
	if err != nil {
		panic(err)
	}

	for buffer := make([]byte, mtu); ; {
		n, err := rtcpFlow.Read(buffer)
		if err != nil {
			panic(err)
		}
		if _, _, err := g.rtcpReader.Read(buffer[:n], interceptor.Attributes{}); err != nil {
			panic(err)
		}
		if bitrate := g.scream.GetTargetBitrate(0); bitrate != g.targetBitrate {
			if bitrate < 0 {
				// TODO: Force KeyFrame here
				continue
			}
			g.targetBitrate = bitrate
			log.Printf("new target bitrate: %v\n", bitrate)
			g.pipeline.SetBitRate(uint(bitrate / 1000)) // Gstreamer expects kbit/s
		}
	}
}

func (g *gstWriter) Write(p []byte) (n int, err error) {
	var pkt rtp.Packet
	err = pkt.Unmarshal(p)
	if err != nil {
		return 0, err
	}
	return g.rtpWriter.Write(&pkt.Header, p[pkt.Header.MarshalSize():], nil)
}

func qrtClient() error {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
	quicSession, err := quic.DialAddr(addr, tlsConf, &quic.Config{EnableDatagrams: true})
	if err != nil {
		return err
	}

	qrtSession, err := qrt.NewSession(quicSession)
	if err != nil {
		return err
	}
	rtpFlow, err := qrtSession.OpenWriteFlow()
	if err != nil {
		return err
	}

	cc, err := scream.NewSenderInterceptor()
	if err != nil {
		return err
	}
	chain := interceptor.NewChain([]interceptor.Interceptor{cc})
	streamWriter := chain.BindLocalStream(&interceptor.StreamInfo{
		SSRC:         0,
		RTCPFeedback: []interceptor.RTCPFeedback{{Type: "ack", Parameter: "ccfb"}},
	}, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		return rtpFlow.WriteRTP(header, payload)
	}))
	rtcpReader := chain.BindRTCPReader(interceptor.RTCPReaderFunc(func(in []byte, attributes interceptor.Attributes) (int, interceptor.Attributes, error) {
		return len(in), nil, nil
	}))

	tb := cc.GetTargetBitrate(0)
	log.Printf("init target bitrate: %v\n", tb)
	gst := &gstWriter{
		qrtSession: qrtSession,
		rtcpReader: rtcpReader,
		rtpWriter:  streamWriter,
		scream:     cc,
	}
	pipeline := gstsrc.NewPipeline(tb, gst)
	gst.pipeline = pipeline
	pipeline.SetSSRC(0)
	pipeline.Start()

	gst.acceptFeedback()
	return nil
}

// Setup a bare-bones TLS config for the server
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}
