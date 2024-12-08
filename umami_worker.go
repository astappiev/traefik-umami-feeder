package traefik_umami_feeder

import (
	"context"
	"fmt"
	"net/http"
)

type SendPayload struct {
	Website  string `json:"website"`
	Hostname string `json:"hostname"`
	Language string `json:"language,omitempty"`
	Referrer string `json:"referrer,omitempty"`
	Url      string `json:"url"`
	//Ip       string                 `json:"ip,omitempty"`
	//Data     map[string]interface{} `json:"data,omitempty"` // Additional data for the event
	//Name     string                 `json:"name,omitempty"` // Event name (for custom events)
	//Screen   string                 `json:"screen,omitempty"` // Screen resolution (ex. "1920x1080")
	//Tag      string                 `json:"tag,omitempty"`
	//Title    string                 `json:"title,omitempty"` // Page title
}

type SendBody struct {
	Payload SendPayload `json:"payload"`
	Type    string      `json:"type"`
}

type UmamiPayload struct {
	body    SendBody
	headers http.Header
}

var headersToCopy = []string{
	"User-Agent",
	"X-Real-Op",
	"X-Forwarded-For",
	"cf-ipcountry",
	"cf-region-code",
	"cf-ipcity",
	"cf-connecting-ip",
	"x-vercel-ip-country",
	"x-vercel-ip-country-region",
	"x-vercel-ip-city",
}

// Copied and adapted from https://github.com/safing/plausiblefeeder/blob/master/event.go
// Licensed as MIT license

func (h *UmamiFeeder) submitToFeed(req *http.Request) {
	body := SendBody{
		Payload: SendPayload{
			Hostname: parseDomainFromHost(req.Host),
			Language: parseAcceptLanguage(req.Header.Get("Accept-Language")),
			Referrer: req.Referer(),
			Url:      req.URL.String(),
		},
		Type: "event",
	}

	var headers = make(http.Header)
	copyHeaders(headers, req.Header, headersToCopy)
	writeXForwardedHeaders(headers, req)

	payload := &UmamiPayload{body: body, headers: headers}

	select {
	case h.queue <- payload:
	default:
		h.error("failed to submit event: queue full")
	}
}

func (h *UmamiFeeder) startWorker(ctx context.Context) {
	for {
		err := h.umamiEventFeeder(ctx)
		if err != nil {
			h.error("worker failed: " + err.Error())
		} else {
			return
		}
	}
}

func (h *UmamiFeeder) umamiEventFeeder(ctx context.Context) (err error) {
	defer func() {
		// Recover from panic.
		panicVal := recover()
		if panicVal != nil {
			h.error("panic: " + fmt.Sprint(panicVal))
		}
	}()

	for {
		// Wait for event.
		select {
		case <-ctx.Done():
			h.debug("worker shutting down (canceled)")
			return nil

		case event := <-h.queue:
			h.reportEventToUmami(ctx, event)
		}
	}
}

func (h *UmamiFeeder) reportEventToUmami(ctx context.Context, event *UmamiPayload) {
	hostname := event.body.Payload.Hostname
	websiteId, ok := h.websites[hostname]
	if !ok {
		website, err := createWebsite(ctx, h.umamiHost, h.umamiToken, h.umamiTeamId, hostname)
		if err != nil {
			h.error("failed to create website: " + err.Error())
			return
		}

		h.websites[website.Domain] = website.ID
		websiteId = website.ID
		h.debug("created website for: %s", website.Domain)
	}
	if websiteId == "" {
		h.error("skip tracking, websiteId is unknown: " + hostname)
		return
	}
	event.body.Payload.Website = websiteId

	h.debug("sending tracking request %v %v", event.body, event.headers)
	resp, err := sendRequest(ctx, h.umamiHost+"/api/send", event.body, event.headers)
	if err != nil {
		h.error("failed to send tracking: " + err.Error())
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()
}
