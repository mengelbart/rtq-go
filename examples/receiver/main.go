package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"os/signal"

	"github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/qlog"
	"github.com/mengelbart/rtq"
	gstsink "github.com/mengelbart/rtq/examples/internal/gstreamer-sink"
	"github.com/mengelbart/rtq/examples/internal/utils"
)

const mtu = 1400

func main() {
	logFilename := os.Getenv("LOG_FILE")
	if logFilename != "" {
		logfile, err := os.Create(logFilename)
		if err != nil {
			fmt.Printf("Could not create log file: %s\n", err.Error())
			os.Exit(1)
		}
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	qlogWriter, err := utils.GetQLOGWriter()
	if err != nil {
		log.Printf("Could not get qlog writer: %s\n", err.Error())
		os.Exit(1)
	}

	quicConf := &quic.Config{
		EnableDatagrams: true,
	}
	if qlogWriter != nil {
		quicConf.Tracer = qlog.NewTracer(qlogWriter)
	}

	dstStr := "autovideosink"
	dst := os.Getenv("DESTINATION")
	if len(dst) > 0 {
		dstStr = fmt.Sprintf("matroskamux ! filesink location=%v", dst)
	}

	err = run(":4242", generateTLSConfig(), quicConf, dstStr)
	if err != nil {
		log.Fatal(err)
	}
}

func run(addr string, tlsConf *tls.Config, quicConf *quic.Config, dst string) error {
	listener, err := quic.ListenAddr(addr, tlsConf, quicConf)
	if err != nil {
		return err
	}
	quicSession, err := listener.Accept(context.Background())
	if err != nil {
		return err
	}

	rtqSession, err := rtq.NewSession(quicSession)
	if err != nil {
		return err
	}

	rtqFlow, err := rtqSession.AcceptFlow(0)
	if err != nil {
		return err
	}

	pipeline, err := gstsink.NewPipeline("vp8", dst)
	if err != nil {
		return err
	}
	log.Printf("created pipeline: '%v'\n", pipeline.String())

	destroyed := make(chan struct{}, 1)
	gstsink.HandleSinkEOS(func() {
		pipeline.Destroy()
		destroyed <- struct{}{}
	})
	pipeline.Start()

	done := make(chan struct{}, 1)
	errChan := make(chan error, 1)
	go func() {
		for buffer := make([]byte, mtu); ; {
			n, err := rtqFlow.Read(buffer)
			if err != nil {
				if err == io.EOF {
					close(done)
					break
				}
				errChan <- err
			}
			pipeline.Push(buffer[:n])
		}
	}()
	go gstsink.StartMainLoop()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	select {
	case err1 := <-errChan:
		err = err1
	case <-done:
	case <-signals:
		log.Printf("got interrupt signal")
		err := rtqSession.Close()
		if err != nil {
			log.Printf("failed to close rtq session: %v\n", err.Error())
		}
	}

	log.Println("stopping pipeline")
	pipeline.Stop()
	<-destroyed
	log.Println("destroyed pipeline, exiting")
	return err
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
