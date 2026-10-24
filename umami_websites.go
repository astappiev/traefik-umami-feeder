package traefik_umami_feeder

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type websitesResponse struct {
	Data     []Website `json:"data"`
	Count    int       `json:"count"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	OrderBy  string    `json:"orderBy"`
}

type Website struct {
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	TeamId    string    `json:"teamId,omitempty"`
	Domain    string    `json:"domain,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

func createWebsite(ctx context.Context, umamiHost, umamiToken, teamId, websiteDomain string) (*Website, error) {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+umamiToken)

	var result Website
	err := sendRequestAndParse(ctx, umamiHost+"/api/websites", Website{
		Name:   websiteDomain,
		Domain: websiteDomain,
		TeamId: teamId,
	}, headers, &result)
	if err != nil {
		return nil, err
	}
	if result.ID == "" {
		// lookupWebsiteId never finds an empty id, so a website would be created per event.
		return nil, errors.New("response has no website id")
	}

	return &result, nil
}

func fetchWebsites(ctx context.Context, umamiHost, umamiToken, teamId string) (*[]Website, error) {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+umamiToken)

	url := umamiHost + "/api/websites?pageSize=200"
	if len(teamId) != 0 {
		url = umamiHost + "/api/teams/" + teamId + "/websites?pageSize=200"
	}

	var result websitesResponse
	err := sendRequestAndParse(ctx, url, nil, headers, &result)
	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// lookupWebsiteId returns the websiteId for the hostname, or "" if unknown.
func lookupWebsiteId(h *UmamiFeeder, hostname string) string {
	h.websitesMutex.RLock()
	websiteId := h.websites[hostname]
	h.websitesMutex.RUnlock()
	return websiteId
}

// failedWebsiteRetryInterval is how long a failed website creation isn't retried.
const failedWebsiteRetryInterval = 10 * time.Minute

// resolveWebsiteId returns the websiteId for the hostname, creating the website if needed.
// Creation is serialized by createMutex, failures aren't retried for failedWebsiteRetryInterval.
func resolveWebsiteId(ctx context.Context, h *UmamiFeeder, hostname string) string {
	h.createMutex.Lock()
	defer h.createMutex.Unlock()

	// It may have been created since the event was queued.
	if websiteId := lookupWebsiteId(h, hostname); websiteId != "" {
		return websiteId
	}

	if failedAt, ok := h.failedWebsites[hostname]; ok && time.Since(failedAt) < failedWebsiteRetryInterval {
		return ""
	}

	// websitesMutex isn't held during the API call, so requests aren't blocked.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	website, err := createWebsite(ctx, h.umamiHost, h.umamiToken, h.umamiTeamId, hostname)
	if err != nil {
		h.failedWebsites[hostname] = time.Now()
		h.error("failed to create website " + hostname + ": " + err.Error())
		return ""
	}
	delete(h.failedWebsites, hostname)

	h.websitesMutex.Lock()
	h.websites[hostname] = website.ID
	h.websitesMutex.Unlock()

	h.debugf("website created '%s': %s", website.Domain, website.ID)
	return website.ID
}
