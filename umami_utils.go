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

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
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
	// check if the host has a port
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

const parseAcceptLanguagePattern = `([a-zA-Z\-]+)(?:;q=\d\.\d)?(?:,\s)?`

var parseAcceptLanguageRegexp = regexp.MustCompile(parseAcceptLanguagePattern)

func parseAcceptLanguage(acceptLanguage string) string {
	matches := parseAcceptLanguageRegexp.FindAllStringSubmatch(acceptLanguage, -1)
	if len(matches) == 0 {
		return ""
	}
	return matches[0][1]
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
