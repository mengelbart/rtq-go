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

	"github.com/lucas-clemente/quic-go"
	"github.com/mengelbart/qrt"
	gstsink "github.com/mengelbart/qrt/examples/internal/gstreamer-sink"
)

const mtu = 1400

func main() {
	err := run(":4242", generateTLSConfig())
	if err != nil {
		log.Fatal(err)
	}
}

func run(addr string, tlsConf *tls.Config) error {
	listener, err := quic.ListenAddr(addr, tlsConf, &quic.Config{EnableDatagrams: true})
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

	rtpFlow, err := qrtSession.AcceptFlow(0)
	if err != nil {
		return err
	}

	pipeline, err := gstsink.NewPipeline("vp8", "autovideosink")
	if err != nil {
		return err
	}
	log.Printf("created pipeline: '%v'\n", pipeline.String())
	pipeline.Start()

	go func() {
		for buffer := make([]byte, mtu); ; {
			n, err := rtpFlow.Read(buffer)
			if err != nil {
				panic(err)
			}
			pipeline.Push(buffer[:n])
		}
	}()
	gstsink.StartMainLoop()
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
