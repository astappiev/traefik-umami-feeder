package traefik_umami_feeder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type UmamiEvent struct {
	Website   string         `json:"website"`             // Website ID
	Hostname  string         `json:"hostname"`            // Name of host
	Language  string         `json:"language,omitempty"`  // Language of visitor (ex. "en-US")
	Referrer  string         `json:"referrer,omitempty"`  // Referrer URL
	Url       string         `json:"url"`                 // Page URL
	Ip        string         `json:"ip,omitempty"`        // IP address
	UserAgent string         `json:"userAgent,omitempty"` // User agent
	Timestamp int64          `json:"timestamp,omitempty"` // UNIX timestamp in seconds
	Data      map[string]any `json:"data,omitempty"`      // Additional data for the event
	// Name      string         `json:"name,omitempty"`      // Event name (for custom events)
	// Screen    string         `json:"screen,omitempty"`    // Screen resolution (ex. "1920x1080")
	// Title     string         `json:"title,omitempty"`     // Page title
}

type SendBody struct {
	Payload *UmamiEvent `json:"payload"`
	Type    string      `json:"type"`
}

// submitToFeed queues an event, sending it to the website resolver first if websiteId is empty.
func (h *UmamiFeeder) submitToFeed(req *http.Request, hostname, websiteId string, statusCode int) {
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
		event.Data = map[string]any{
			"status_code": statusCode,
		}
	}

	queue := h.queue
	if websiteId == "" {
		queue = h.unresolved
	}
	select {
	case queue <- event:
	default:
		h.error("failed to submit event: queue full")
	}
}

// supervise runs fn, restarting it after an error or panic.
func (h *UmamiFeeder) supervise(ctx context.Context, name string, fn func(ctx context.Context) error) {
	for {
		err := runRecovered(ctx, fn)
		if err == nil {
			return
		}

		h.error(name + " failed: " + err.Error())
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second): // Avoid a hot loop on repeated failures.
		}
	}
}

// runRecovered runs fn, returning a panic as an error.
func runRecovered(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	defer func() {
		if panicVal := recover(); panicVal != nil {
			err = fmt.Errorf("panic: %v", panicVal)
		}
	}()

	return fn(ctx)
}

// websiteResolver sets websiteIds of queued events and passes them to the worker.
// It runs separately, so slow API calls don't stall the worker.
func (h *UmamiFeeder) websiteResolver(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case event := <-h.unresolved:
			h.resolveEvent(ctx, event)
		}
	}
}

// resolveEvent sets the event's websiteId and passes it to the worker, or drops it if unresolved.
func (h *UmamiFeeder) resolveEvent(ctx context.Context, event *UmamiEvent) {
	event.Website = resolveWebsiteId(ctx, h, event.Hostname)
	if event.Website == "" {
		h.debugf("tracking skipped, websiteId is unknown: %s", event.Hostname)
		return
	}

	select {
	case h.queue <- event:
	default:
		h.error("failed to submit event: queue full")
	}
}

func (h *UmamiFeeder) umamiEventFeeder(ctx context.Context) error {
	batch := make([]*SendBody, 0, h.batchSize)
	timeout := time.NewTimer(h.batchMaxWait)

	for {
		// Wait for event.
		select {
		case <-ctx.Done():
			h.debugf("worker shutting down (canceled)")
			// Stop accepting events, nobody reads them anymore.
			h.isEnabled.Store(false)
			// Best effort, queued events are dropped.
			if len(batch) > 0 {
				// The worker ctx is canceled, so use a fresh one.
				sendCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				h.reportEventsToUmami(sendCtx, batch)
				cancel()
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
	defer func() {
		_ = resp.Body.Close()
	}()

	if h.isDebug {
		bodyBytes, _ := io.ReadAll(resp.Body)
		h.debugf("%v: %s", resp.Status, string(bodyBytes))
	} else {
		// Drain the body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
	}
}
