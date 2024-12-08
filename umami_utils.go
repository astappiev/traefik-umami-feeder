package traefik_umami_feeder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func sendRequest(ctx context.Context, url string, body interface{}, headers http.Header) (*http.Response, error) {
	var req *http.Request
	var err error

	if body != nil {
		bodyJson, err := json.Marshal(body)
		if err != nil {
			return nil, err
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
		return nil, fmt.Errorf("request failed with status %d", status)
	}

	return resp, nil
}

func sendRequestAndParse(ctx context.Context, url string, body interface{}, headers http.Header, value interface{}) error {
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

// opts the port from the host.
func parseDomainFromHost(host string) string {
	// check if the host has a port
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
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

func copyHeaders(dst, src http.Header, headersToCopy []string) {
	for _, key := range headersToCopy {
		if values := src.Values(key); len(values) > 0 {
			dst[key] = values
		}
	}
}
