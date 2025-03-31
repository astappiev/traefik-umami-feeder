package traefik_umami_feeder

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type UmamiEvent struct {
	Website   string `json:"website"`             // Website ID
	Hostname  string `json:"hostname"`            // Name of host
	Language  string `json:"language,omitempty"`  // Language of visitor (ex. "en-US")
	Referrer  string `json:"referrer,omitempty"`  // Referrer URL
	Url       string `json:"url"`                 // Page URL
	Ip        string `json:"ip,omitempty"`        // IP address
	UserAgent string `json:"userAgent,omitempty"` // User agent
	Timestamp int64  `json:"timestamp,omitempty"` // UNIX timestamp in seconds
	//Data     map[string]interface{} `json:"data,omitempty"`   // Additional data for the event
	//Name     string                 `json:"name,omitempty"`   // Event name (for custom events)
	//Screen   string                 `json:"screen,omitempty"` // Screen resolution (ex. "1920x1080")
	//Title    string                 `json:"title,omitempty"`  // Page title
}

type SendBody struct {
	Payload *UmamiEvent `json:"payload"`
	Type    string      `json:"type"`
}

func (h *UmamiFeeder) submitToFeed(req *http.Request, code int) {
	event := &UmamiEvent{
		Hostname:  parseDomainFromHost(req.Host),
		Language:  parseAcceptLanguage(req.Header.Get("Accept-Language")),
		Referrer:  req.Referer(),
		Url:       req.URL.String(),
		Ip:        extractRemoteIP(req),
		UserAgent: req.Header.Get("User-Agent"),
		Timestamp: time.Now().Unix(),
	}

	select {
	case h.queue <- event:
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

func (h *UmamiFeeder) reportEventToUmami(ctx context.Context, event *UmamiEvent) {
	hostname := event.Hostname
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
	event.Website = websiteId

	body := SendBody{
		Payload: event,
		Type:    "event",
	}

	h.debug("sending tracking request %v", event)
	resp, err := sendRequest(ctx, h.umamiHost+"/api/send", body, nil)
	if err != nil {
		h.error("failed to send tracking: " + err.Error())
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()
}
