package traefik_umami_feeder

import (
	"context"
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

func getWebsiteId(h *UmamiFeeder, hostname string) string {
	h.websitesMutex.RLock()
	websiteId, ok := h.websites[hostname]
	h.websitesMutex.RUnlock()

	if ok {
		return websiteId
	}

	h.websitesMutex.Lock()
	defer h.websitesMutex.Unlock()

	// Double-check after acquiring write lock to prevent race condition
	if websiteId, ok := h.websites[hostname]; ok {
		return websiteId
	}

	// Create a background context for the API call
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	website, err := createWebsite(ctx, h.umamiHost, h.umamiToken, h.umamiTeamId, hostname)
	if err != nil {
		h.error("failed to create website: " + err.Error())
		return ""
	}

	h.websites[website.Domain] = website.ID
	h.debugf("website created '%s': %s", website.Domain, website.ID)
	return website.ID
}
