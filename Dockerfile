FROM golang:1.16.5-buster AS build

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -yqq \
        wget \
        tar \
        git \
        pkg-config \
        build-essential \
        libgstreamer1.0-dev \
        libgstreamer1.0-0 \
        libgstreamer-plugins-base1.0-dev \
        gstreamer1.0-plugins-base \
        gstreamer1.0-plugins-good \
        gstreamer1.0-plugins-bad \
        gstreamer1.0-plugins-ugly

ENV GO111MODULE=on

WORKDIR /src

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN go build -o /out/sender /src/examples/sender/main.go
RUN go build -o /out/receiver /src/examples/receiver/main.go

FROM martenseemann/quic-network-simulator-endpoint:latest

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -yqq \
        libgstreamer1.0-dev \
        libgstreamer1.0-0 \
        libgstreamer-plugins-base1.0-dev \
        gstreamer1.0-plugins-base \
        gstreamer1.0-plugins-good \
        gstreamer1.0-plugins-bad \
        gstreamer1.0-plugins-ugly

COPY --from=build \
        /out/sender \
        /out/receiver \
        /src/tools/run_endpoint.sh \
        ./

RUN chmod +x run_endpoint.sh

ENTRYPOINT [ "./run_endpoint.sh" ]