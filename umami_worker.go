package traefik_umami_feeder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type UmamiEvent struct {
	Website   string                 `json:"website"`             // Website ID
	Hostname  string                 `json:"hostname"`            // Name of host
	Language  string                 `json:"language,omitempty"`  // Language of visitor (ex. "en-US")
	Referrer  string                 `json:"referrer,omitempty"`  // Referrer URL
	Url       string                 `json:"url"`                 // Page URL
	Ip        string                 `json:"ip,omitempty"`        // IP address
	UserAgent string                 `json:"userAgent,omitempty"` // User agent
	Timestamp int64                  `json:"timestamp,omitempty"` // UNIX timestamp in seconds
	Data      map[string]interface{} `json:"data,omitempty"`      // Additional data for the event
	// Name     string                 `json:"name,omitempty"`   // Event name (for custom events)
	// Screen   string                 `json:"screen,omitempty"` // Screen resolution (ex. "1920x1080")
	// Title    string                 `json:"title,omitempty"`  // Page title
}

type SendBody struct {
	Payload *UmamiEvent `json:"payload"`
	Type    string      `json:"type"`
}

func (h *UmamiFeeder) submitToFeed(req *http.Request, statusCode int) {
	hostname := parseDomainFromHost(req.Host)
	websiteId := getWebsiteId(h, hostname)

	if websiteId == "" {
		h.error("tracking skipped, websiteId is unknown: " + hostname)
		return
	}

	event := &UmamiEvent{
		Hostname:  hostname,
		Language:  parseAcceptLanguage(req.Header.Get("Accept-Language")),
		Referrer:  req.Referer(),
		Url:       req.URL.String(),
		Ip:        extractRemoteIP(req),
		UserAgent: req.Header.Get("User-Agent"),
		Timestamp: time.Now().Unix(),
		Website:   websiteId,
	}

	if statusCode >= 400 {
		event.Data = map[string]interface{}{
			"status_code": statusCode,
		}
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

func (h *UmamiFeeder) umamiEventFeeder(ctx context.Context) error {
	defer func() {
		// Recover from panic.
		panicVal := recover()
		if panicVal != nil {
			h.error("panic: " + fmt.Sprint(panicVal))
		}
	}()

	batch := make([]*SendBody, 0, h.batchSize)
	timeout := time.NewTimer(h.batchMaxWait)

	for {
		// Wait for event.
		select {
		case <-ctx.Done():
			h.debugf("worker shutting down (canceled)")
			if len(batch) > 0 {
				h.reportEventsToUmami(ctx, batch)
			}
			return nil

		case event := <-h.queue:
			batch = append(batch, &SendBody{Payload: event, Type: "event"})
			if len(batch) >= h.batchSize {
				h.reportEventsToUmami(ctx, batch)
				batch = make([]*SendBody, 0, h.batchSize)
				timeout.Reset(h.batchMaxWait)
			}

		case <-timeout.C:
			if len(batch) > 0 {
				h.reportEventsToUmami(ctx, batch)
				batch = make([]*SendBody, 0, h.batchSize)
			}
			timeout.Reset(h.batchMaxWait)
		}
	}
}

func (h *UmamiFeeder) reportEventsToUmami(ctx context.Context, events []*SendBody) {
	h.debugf("reporting %d events", len(events))
	resp, err := sendRequest(ctx, h.umamiHost+"/api/batch", events, nil)
	if err != nil {
		h.error("failed to send tracking: " + err.Error())
		return
	}
	if h.isDebug {
		bodyBytes, _ := io.ReadAll(resp.Body)
		h.debugf("%v: %s", resp.Status, string(bodyBytes))
	}
	defer func() {
		_ = resp.Body.Close()
	}()
}
