module github.com/mengelbart/rtq-go

go 1.16

require (
	github.com/lucas-clemente/quic-go v0.20.1
	github.com/pion/rtp v1.7.2
	github.com/pion/transport v0.12.3
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // indirect
)

replace github.com/lucas-clemente/quic-go v0.20.1 => github.com/mengelbart/quic-go v0.7.1-0.20211215120218-ca20b5dae716
