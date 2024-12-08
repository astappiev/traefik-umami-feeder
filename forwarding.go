package traefik_umami_feeder

// Copied source from
// https://github.com/kzmake/traefik-plugin-forward-request/blob/master/util.go

import (
	"net"
	"net/http"
	"strings"
)

const (
	xForwardedProto  = "x-forwarded-proto"
	xForwardedFor    = "x-forwarded-for"
	xForwardedHost   = "x-forwarded-host"
	xForwardedPort   = "x-forwarded-port"
	xForwardedURI    = "x-forwarded-uri"
	xForwardedMethod = "x-forwarded-method"
)

func copyHeaders(dst, src http.Header, headersToCopy []string) {
	for _, key := range headersToCopy {
		if values := src.Values(key); len(values) > 0 {
			dst[key] = values
		}
	}
}

func writeXForwardedHeaders(dst http.Header, req *http.Request) {
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if values := req.Header.Values(xForwardedFor); len(values) > 0 {
			clientIP = strings.Join(values, ", ") + ", " + clientIP
		}
		dst.Set(xForwardedFor, clientIP)
	}

	xfm := req.Header.Get(xForwardedMethod)
	switch {
	case xfm != "":
		dst.Set(xForwardedMethod, xfm)
	case req.Method != "":
		dst.Set(xForwardedMethod, req.Method)
	default:
		dst.Del(xForwardedMethod)
	}

	xfp := req.Header.Get(xForwardedProto)
	switch {
	case xfp != "":
		dst.Set(xForwardedProto, xfp)
	case req.TLS != nil:
		dst.Set(xForwardedProto, "https")
	default:
		dst.Set(xForwardedProto, "http")
	}

	if xfp := req.Header.Get(xForwardedPort); xfp != "" {
		dst.Set(xForwardedPort, xfp)
	}

	xfh := req.Header.Get(xForwardedHost)
	switch {
	case xfh != "":
		dst.Set(xForwardedHost, xfh)
	case req.Host != "":
		dst.Set(xForwardedHost, req.Host)
	default:
		dst.Del(xForwardedHost)
	}

	xfu := req.Header.Get(xForwardedURI)
	switch {
	case xfu != "":
		dst.Set(xForwardedURI, xfu)
	case req.URL.RequestURI() != "":
		dst.Set(xForwardedURI, req.URL.RequestURI())
	default:
		dst.Del(xForwardedURI)
	}
}
