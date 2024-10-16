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
	Name      string    `json:"name,omitempty"`
	TeamId    string    `json:"teamId,omitempty"`
	Domain    string    `json:"domain,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

func createWebsite(umamiHost string, umamiToken string, teamId string, websiteDomain string) (*Website, error) {
	var headers = make(http.Header)
	headers.Set("Authorization", "Bearer "+umamiToken)

	var result Website
	err := sendRequestAndParse(umamiHost+"/api/websites", Website{
		Name:   websiteDomain,
		Domain: websiteDomain,
		TeamId: teamId,
	}, headers, &result)

	if err != nil {
		return nil, err
	}

	return &result, nil
}

func fetchWebsites(umamiHost string, umamiToken string, teamId string) (*[]Website, error) {
	var headers = make(http.Header)
	headers.Set("Authorization", "Bearer "+umamiToken)

	url := umamiHost + "/api/websites?pageSize=200"
	if len(teamId) != 0 {
		url = umamiHost + "/api/teams/" + teamId + "/websites?pageSize=200"
	}

	var result WebsitesResponse
	err := sendRequestAndParse(url, nil, headers, &result)

	if err != nil {
		return nil, err
	}

	return &result.Data, nil
}
