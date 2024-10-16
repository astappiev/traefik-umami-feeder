package traefik_umami_feeder

import (
	"net/http"
	"time"
)

type WebsitesResponse struct {
	Data     []Website `json:"data"`
	Count    int       `json:"count"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	OrderBy  string    `json:"orderBy"`
}

type Website struct {
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

func createWebsite(umamiHost string, umamiToken string, websiteDomain string) (*Website, error) {
	var headers = make(http.Header)
	headers.Set("Authorization", "Bearer "+umamiToken)

	var result Website
	err := sendRequestAndParse(umamiHost+"/api/websites", Website{
		Name:   websiteDomain,
		Domain: websiteDomain,
	}, headers, &result)

	if err != nil {
		return nil, err
	}

	return &result, nil
}

func fetchWebsites(umamiHost string, umamiToken string) (*[]Website, error) {
	var headers = make(http.Header)
	headers.Set("Authorization", "Bearer "+umamiToken)

	var result WebsitesResponse
	err := sendRequestAndParse(umamiHost+"/api/websites?pageSize=200", nil, headers, &result)

	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}
