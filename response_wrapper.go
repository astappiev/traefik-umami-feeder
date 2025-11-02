package traefik_umami_feeder

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

// ResponseWrapper wraps an http.ResponseWriter to intercept status codes and report requests to Umami.
type ResponseWrapper struct {
	http.ResponseWriter

	request *http.Request
	feeder  *UmamiFeeder
	written bool // Track if WriteHeader was called
}

// WriteHeader intercepts the status code and submits the request to the Umami feeder if needed.
// Then passes the call to the original WriteHeader method.
func (rw *ResponseWrapper) WriteHeader(statusCode int) {
	if rw.written {
		return // Prevent multiple calls
	}
	rw.written = true

	if rw.feeder.shouldTrackStatus(statusCode) {
		rw.feeder.submitToFeed(rw.request, statusCode)
	}

	// Continue with the original method.
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write intercepts the write call to ensure Flush is called after writing.
func (rw *ResponseWrapper) Write(b []byte) (int, error) {
	rw.WriteHeader(http.StatusOK)

	n, err := rw.ResponseWriter.Write(b)

	// Flush explicitly after write
	// Required due to https://github.com/astappiev/traefik-umami-feeder/issues/7
	rw.Flush()

	return n, err
}

// Hijack implements the http.Hijacker interface.
func (rw *ResponseWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}

	return nil, nil, fmt.Errorf("%T is not a http.Hijacker", rw.ResponseWriter)
}

// Flush implements the http.Flusher interface.
func (rw *ResponseWrapper) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
