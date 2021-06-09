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

var pipelines = map[int]*Pipeline{}
var pipelinesLock sync.Mutex

type Pipeline struct {
	id          int
	pipeline    *C.GstElement
	writer      io.Writer
	pipelineStr string
}

func NewPipeline(codecName, src string, w io.Writer) (*Pipeline, error) {
	pipelineStr := "appsink name=appsink"

	switch codecName {
	case "vp8":
		pipelineStr = src + " ! vp8enc ! rtpvp8pay mtu=1200 ! " + pipelineStr

	case "vp9":
		pipelineStr = src + " ! vp9enc ! rtpvp9pay ! " + pipelineStr

	case "h264":
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
		pipeline:    C.go_gst_create_src_pipeline(pipelineStrUnsafe),
		pipelineStr: pipelineStr,
		writer:      w,
	}
	pipelines[sp.id] = sp
	return sp, nil
}

func (p *Pipeline) String() string {
	return p.pipelineStr
}

//func NewPipeline(bitrate int64, w io.Writer) *Pipeline {
//	pipelinesLock.Lock()
//	defer pipelinesLock.Unlock()
//	pipelineStr := "appsink name=appsink"
//	pipelineStr = fmt.Sprintf("videotestsrc ! video/x-raw,format=I420 ! x264enc name=x264enc speed-preset=ultrafast tune=zerolatency bitrate=%v ! video/x-h264,stream-format=byte-stream ! rtph264pay name=rtph264pay mtu=1000 ! "+pipelineStr, bitrate)
//	log.Printf("creating pipeline: '%v'\n", pipelineStr)
//	sp := &Pipeline{
//		pipeline: C.go_gst_create_src_pipeline(C.CString(pipelineStr)),
//		writer:   w,
//		id:       len(pipelines),
//	}
//	pipelines[sp.id] = sp
//	return sp
//}

func (p *Pipeline) Start() {
	C.go_gst_start_src_pipeline(p.pipeline, C.int(p.id))
}

func (p *Pipeline) SSRC() uint {
	return uint(C.go_gst_get_ssrc(p.pipeline))
}

func (p *Pipeline) SetSSRC(ssrc uint) {
	C.go_gst_set_ssrc(p.pipeline, C.uint(ssrc))
}

func (p *Pipeline) SetBitRate(bitrate uint) {
	C.go_gst_set_bitrate(p.pipeline, C.uint(bitrate))
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
