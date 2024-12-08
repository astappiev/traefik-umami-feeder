package traefik_umami_feeder

import (
	"net/http"
	"regexp"
	"strings"
)

// Copied from https://github.com/1cedsoda/traefik-umami-plugin/blob/master/umami_tracking.go
// Licensed as Apache-2.0 license

type SendPayload struct {
	Website  string                 `json:"website"`
	Hostname string                 `json:"hostname"`
	Language string                 `json:"language,omitempty"`
	Url      string                 `json:"url"`
	Referrer string                 `json:"referrer,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

type SendBody struct {
	Payload SendPayload `json:"payload"`
	Type    string      `json:"type"`
}

func buildPayload(req *http.Request, websiteId string) SendPayload {
	return SendPayload{
		Website:  websiteId,
		Hostname: parseDomainFromHost(req.Host),
		Language: parseAcceptLanguage(req.Header.Get("Accept-Language")),
		Url:      req.URL.String(),
		Referrer: req.Referer(),
		Data:     map[string]interface{}{},
	}
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

func buildSendBody(clientReq *http.Request, websiteId string) (SendBody, http.Header) {
	body := SendBody{
		Payload: buildPayload(clientReq, websiteId),
		Type:    "event",
	}

	var headers = make(http.Header)
	headers.Set("Content-Type", "application/json")
	copyHeaders(headers, clientReq.Header)
	removeHeaders(headers, hopHeaders...)
	writeXForwardedHeaders(headers, clientReq)
	return body, headers
}
