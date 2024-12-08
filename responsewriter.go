package traefik_umami_feeder

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

// Copied and adapted from https://github.com/safing/plausiblefeeder/blob/master/responsewriter.go
// Licensed as MIT license

// ResponseWriter is used to wrap given response writers.
type ResponseWriter struct {
	http.ResponseWriter

	request *http.Request
	feeder  *UmamiFeeder
}

// WriteHeader adds custom handling to the wrapped WriterHeader method.
func (rw *ResponseWriter) WriteHeader(code int) {
	if rw.feeder.shouldTrackStatus(code) {
		rw.feeder.submitToFeed(rw.request, code)
	}

	// Continue with the original method.
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("%T is not a http.Hijacker", rw.ResponseWriter)
	}

	return hijacker.Hijack()
}

func (rw *ResponseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
