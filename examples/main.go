package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log"
	"math/big"

	"github.com/mengelbart/qrt"
	"github.com/mengelbart/qrt/examples/internal/gst"

	"github.com/lucas-clemente/quic-go"
	"github.com/pion/interceptor"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/x264"
	_ "github.com/pion/mediadevices/pkg/driver/camera" // This is required to register camera adapter
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/rtp"
)

const (
	addr = "localhost:4242"
	mtu  = 1000
)

func main() {
	go func() {
		err := qrtServer()
		if err != nil {
			log.Fatalf("server crashed: %v", err)
		}
	}()

	err := qrtClient()
	if err != nil {
		log.Fatalf("client crashed: %v", err)
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

	flow, err := qrtSession.AcceptFlow()
	if err != nil {
		return err
	}

	// TODO: Replace NoOp by something useful like a Receiver Report generator for congestion control
	noop := &interceptor.NoOp{}
	chain := interceptor.NewChain([]interceptor.Interceptor{noop})
	streamReader := chain.BindRemoteStream(&interceptor.StreamInfo{
		SSRC: 0,
	}, interceptor.RTPReaderFunc(func(bytes []byte, _ interceptor.Attributes) (int, interceptor.Attributes, error) {
		return len(bytes), nil, nil
	}))

	pipeline := gst.CreatePipeline()
	pipeline.Start()

	for buffer := make([]byte, mtu); ; {
		n, err := flow.Read(buffer)
		if err != nil {
			panic(err)
		}

		log.Println("received rtp")
		pipeline.Push(buffer[:n])

		if _, _, err := streamReader.Read(buffer[:n], nil); err != nil {
			panic(err)
		}
	}
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
	flow, err := qrtSession.OpenWriteFlow()
	if err != nil {
		return err
	}

	// TODO: Replace NoOp by something useful like a Congestion Controller
	noop := &interceptor.NoOp{}
	chain := interceptor.NewChain([]interceptor.Interceptor{noop})
	_ = chain.BindLocalStream(&interceptor.StreamInfo{
		SSRC: 0,
	}, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		return flow.WriteRTP(header, payload)
	}))

	x264Params, err := x264.NewParams()
	if err != nil {
		return err
	}
	x264Params.Preset = x264.PresetMedium
	x264Params.BitRate = 1_000_000 // 1mbps
	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
	)
	mediaStream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.FrameFormat = prop.FrameFormat(frame.FormatI420)
			c.Width = prop.Int(640)
			c.Height = prop.Int(480)
		},
		Codec: codecSelector,
	})
	if err != nil {
		return err
	}
	videoTrack := mediaStream.GetVideoTracks()[0]
	defer videoTrack.Close()

	rtpReader, err := videoTrack.NewRTPReader(x264Params.RTPCodec().MimeType, 0, mtu)
	if err != nil {
		return err
	}

	for {
		pkts, release, err := rtpReader.Read()
		if err != nil {
			return err
		}

		for _, pkt := range pkts {
			_, err := flow.WriteRTP(&pkt.Header, pkt.Payload)
			if err != nil {
				return err
			}
		}
		release()
	}
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
