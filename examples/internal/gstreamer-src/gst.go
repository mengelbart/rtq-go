package gst

/*
#cgo pkg-config: gstreamer-1.0

#include "gst.h"

*/
import "C"
import (
	"bytes"
	"errors"
	"io"
	"log"
	"sync"
	"unsafe"
)

var UnknownCodecError = errors.New("unknown codec")

// StartMainLoop starts GLib's main loop
// It needs to be called from the process' main thread
// Because many gstreamer plugins require access to the main thread
// See: https://golang.org/pkg/runtime/#LockOSThread
func StartMainLoop() {
	C.gstreamer_send_start_mainloop()
}

var pipelines = map[int]*Pipeline{}
var pipelinesLock sync.Mutex

type Pipeline struct {
	id          int
	pipeline    *C.GstElement
	writer      io.Writer
	pipelineStr string
	payloder    string
}

func NewPipeline(codecName, src string, w io.Writer) (*Pipeline, error) {
	pipelineStr := "appsink name=appsink"
	var payloader string

	switch codecName {
	case "vp8":
		payloader = "rtpvp8pay"
		pipelineStr = src + " ! vp8enc ! rtpvp8pay mtu=1200 ! " + pipelineStr

	case "vp9":
		payloader = "rtpvp9pay"
		pipelineStr = src + " ! vp9enc ! rtpvp9pay ! " + pipelineStr

	case "h264":
		payloader = "rtph264pay"
		pipelineStr = src + " ! x264enc speed-preset=utrafast tune=zerolatency ! rtph264pay ! " + pipelineStr

	default:
		return nil, UnknownCodecError
	}

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	sp := &Pipeline{
		id:          len(pipelines),
		pipeline:    C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		pipelineStr: pipelineStr,
		payloder:    payloader,
		writer:      w,
	}
	pipelines[sp.id] = sp
	return sp, nil
}

func (p *Pipeline) String() string {
	return p.pipelineStr
}

func (p *Pipeline) Start() {
	C.gstreamer_send_start_pipeline(p.pipeline, C.int(p.id))
}

func (p *Pipeline) Stop() {
	C.gstreamer_send_stop_pipeline(p.pipeline)
}

func (p *Pipeline) Destroy() {
	C.gstreamer_send_destroy_pipeline(p.pipeline)
}

var eosHandler func()

func HandleSinkEOS(handler func()) {
	eosHandler = handler
}

//export goHandleEOS
func goHandleEOS() {
	eosHandler()
}

func (p *Pipeline) SSRC() uint {
	payloderStrUnsafe := C.CString(p.payloder)
	defer C.free(unsafe.Pointer(payloderStrUnsafe))
	return uint(C.gstreamer_send_get_ssrc(p.pipeline, payloderStrUnsafe))
}

func (p *Pipeline) SetSSRC(ssrc uint) {
	payloaderStrUnsafe := C.CString(p.payloder)
	defer C.free(unsafe.Pointer(payloaderStrUnsafe))
	C.gstreamer_send_set_ssrc(p.pipeline, payloaderStrUnsafe, C.uint(ssrc))
}

func (p *Pipeline) SetBitRate(bitrate uint) {
	C.gstreamer_send_set_bitrate(p.pipeline, C.uint(bitrate))
}

//export goHandlePipelineBuffer
func goHandlePipelineBuffer(buffer unsafe.Pointer, bufferLen C.int, pipelineID C.int) {
	pipelinesLock.Lock()
	pipeline, ok := pipelines[int(pipelineID)]
	pipelinesLock.Unlock()
	defer C.free(buffer)
	if !ok {
		log.Printf("no pipeline with ID %v, discarding buffer", int(pipelineID))
		return
	}

	bs := C.GoBytes(buffer, bufferLen)
	n, err := io.Copy(pipeline.writer, bytes.NewReader(bs))
	if err != nil {
		log.Printf("failed to write %v bytes to writer: %v", n, err)
	}
	if n != int64(bufferLen) {
		log.Printf("different buffer size written: %v vs. %v", n, bufferLen)
	}
}
