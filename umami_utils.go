package traefik_umami_feeder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func sendRequest(ctx context.Context, url string, body any, headers http.Header) (*http.Response, error) {
	var req *http.Request
	var err error

	if body != nil {
		bodyJson, err2 := json.Marshal(body)
		if err2 != nil {
			return nil, err2
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJson))
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	}

	if err != nil {
		return nil, err
	}

	if headers != nil {
		req.Header = headers
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	status := resp.StatusCode
	if status < 200 || status >= 300 {
		defer func() {
			_ = resp.Body.Close()
		}()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("request failed with status %d (failed to read body: %w)", status, err)
		}
		return nil, fmt.Errorf("request failed with status %d (%v)", status, string(respBody))
	}

	return resp, nil
}

func sendRequestAndParse(ctx context.Context, url string, body any, headers http.Header, value any) error {
	resp, err := sendRequest(ctx, url, body, headers)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(respBody, &value)
	if err != nil {
		return err
	}

	return nil
}

func parseDomainFromHost(host string) string {
	// strip the port, also of IPv6 like "[::1]:80", IndexByte avoids an error allocation
	if strings.IndexByte(host, ':') >= 0 {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		} else {
			host = strings.Trim(host, "[]")
		}
	}
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

const parseAcceptLanguagePattern = `([a-zA-Z\-]+)(?:;q=\d\.\d)?(?:,\s)?`

var parseAcceptLanguageRegexp = regexp.MustCompile(parseAcceptLanguagePattern)

func parseAcceptLanguage(acceptLanguage string) string {
	match := parseAcceptLanguageRegexp.FindStringSubmatch(acceptLanguage)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func extractRemoteIP(req *http.Request) string {
	if ip := req.Header.Get("Cf-Connecting-Ip"); ip != "" {
		return ip
	}

	if ip := req.Header.Get("X-Vercel-Ip"); ip != "" {
		return ip
	}

	// Standard proxy headers
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if xrip := req.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}

	// Direct connection
	if req.RemoteAddr != "" {
		ip, _, err := net.SplitHostPort(req.RemoteAddr)
		if err == nil {
			return ip
		}
		return req.RemoteAddr
	}

	return ""
}

// hostSet builds a set of normalized hosts, or nil if empty. Hosts with a port only match that port.
func hostSet(hosts []string) map[string]struct{} {
	return stringSet(hosts, normalizeHostPort)
}

// normalizeHostPort normalizes the hostname, keeping the port if present.
func normalizeHostPort(host string) string {
	if hostname, port, err := net.SplitHostPort(host); err == nil {
		return net.JoinHostPort(parseDomainFromHost(hostname), port)
	}
	return parseDomainFromHost(host)
}

// parseDomainFromWebsite normalizes a website domain, stripping a scheme or path
// (e.g. "https://example.com/"), as Umami doesn't validate it.
func parseDomainFromWebsite(domain string) string {
	if _, rest, found := strings.Cut(domain, "://"); found {
		domain = rest
	}
	if i := strings.IndexByte(domain, '/'); i >= 0 {
		domain = domain[:i]
	}
	return parseDomainFromHost(domain)
}

// extensionSet builds a lowercase set of extensions, or nil if empty.
func extensionSet(extensions []string) map[string]struct{} {
	return stringSet(extensions, strings.ToLower)
}

func stringSet(items []string, normalize func(string) string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[normalize(item)] = struct{}{}
	}
	return set
}
