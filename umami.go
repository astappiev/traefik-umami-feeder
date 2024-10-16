package traefik_umami_feeder

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Config the plugin configuration.
type Config struct {
	UmamiHost string `json:"umamiHost"`
	// it is optional, but either UmamiToken or Websites should be set
	UmamiToken string `json:"umamiToken"`
	// as an alternative to UmamiToken, you can set UmamiUsername and UmamiPassword to authenticate
	UmamiUsername string `json:"umamiUsername"`
	UmamiPassword string `json:"umamiPassword"`
	UmamiTeamId   string `json:"umamiTeamId"`
	// if both UmamiToken and Websites are set, Websites will be used to override the websites in the API
	Websites map[string]string `json:"websites"`
	// if createNewWebsites is set to true, the plugin will create new websites in the API, UmamiToken is required
	CreateNewWebsites bool `json:"createNewWebsites"`
	Debug             bool `json:"debug"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		UmamiHost:         "",
		UmamiToken:        "",
		UmamiUsername:     "",
		UmamiPassword:     "",
		UmamiTeamId:       "",
		Websites:          map[string]string{},
		CreateNewWebsites: false,
		Debug:             false,
	}
}

// UmamiFeeder a UmamiFeeder plugin.
type UmamiFeeder struct {
	next       http.Handler
	name       string
	debug      bool
	logHandler *log.Logger

	UmamiHost         string
	UmamiToken        string
	UmamiTeamId       string
	Websites          map[string]string
	CreateNewWebsites bool
}

// New created a new Demo plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// construct
	h := &UmamiFeeder{
		next:       next,
		name:       name,
		debug:      config.Debug,
		logHandler: log.New(os.Stdout, "", 0),

		UmamiHost:         config.UmamiHost,
		UmamiToken:        config.UmamiToken,
		UmamiTeamId:       config.UmamiTeamId,
		Websites:          config.Websites,
		CreateNewWebsites: config.CreateNewWebsites,
	}

	if config.UmamiUsername != "" && config.UmamiPassword != "" {
		token, err := getToken(h.UmamiHost, config.UmamiUsername, config.UmamiPassword)
		if err != nil {
			return nil, fmt.Errorf("failed to get token: %w", err)
		}
		if token == "" {
			return nil, fmt.Errorf("retrieved token is empty")
		}
		h.trace("token received %s", token)
		h.UmamiToken = token
	}

	if h.UmamiHost == "" {
		return nil, fmt.Errorf("`umamiHost` is not set")
	}
	if h.UmamiToken == "" && len(h.Websites) == 0 {
		return nil, fmt.Errorf("either `umamiToken` or `websites` should be set")
	}
	if h.UmamiToken == "" && h.CreateNewWebsites {
		return nil, fmt.Errorf("`umamiToken` is required to create new websites")
	}

	if h.UmamiToken != "" {
		websites, err := fetchWebsites(h.UmamiHost, h.UmamiToken, h.UmamiTeamId)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch websites: %w", err)
		}

		for _, website := range *websites {
			if _, ok := h.Websites[website.Domain]; ok {
				continue
			}

			h.Websites[website.Domain] = website.ID
			h.trace("fetched websiteId for: %s", website.Domain)
		}
		h.log("websites fetched")
	}

	return h, nil
}

func (h *UmamiFeeder) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if h.shouldBeTracked(req) {
		go h.trackRequest(req)
	} else {
		h.trace("Tracking skipped %v", req.URL)
	}

	h.next.ServeHTTP(rw, req)
}

func (h *UmamiFeeder) shouldBeTracked(req *http.Request) bool {
	if h.CreateNewWebsites {
		return true
	}

	hostname := parseDomainFromHost(req.Host)
	if _, ok := h.Websites[hostname]; ok {
		return true
	}

	return false
}

func (h *UmamiFeeder) trackRequest(req *http.Request) {
	hostname := parseDomainFromHost(req.Host)
	websiteId, ok := h.Websites[hostname]
	if !ok {
		website, err := createWebsite(h.UmamiHost, h.UmamiToken, h.UmamiTeamId, hostname)
		if err != nil {
			h.log("failed to create website: " + err.Error())
			return
		}

		h.Websites[website.Domain] = website.ID
		websiteId = website.ID
		h.trace("created website for: %s", website.Domain)
	}

	sendBody, sendHeaders := buildSendBody(req, websiteId)
	h.trace("sending tracking request %s with body %v %v", req.URL, sendBody, sendHeaders)

	_, err := sendRequest(h.UmamiHost+"/api/send", sendBody, sendHeaders)
	if err != nil {
		h.trace("failed to send tracking: " + err.Error())
		return
	}
}

func (h *UmamiFeeder) log(message string) {
	if h.logHandler != nil {
		time := time.Now().Format("2006-01-02T15:04:05Z")
		h.logHandler.Printf("time=\"%s\" level=info msg=\"[traefik-umami-feeder] %s\"", time, message)
	}
}

// Arguments are handled in the manner of [fmt.Printf].
func (h *UmamiFeeder) trace(format string, v ...any) {
	if h.logHandler != nil && h.debug {
		time := time.Now().Format("2006-01-02T15:04:05Z")
		h.logHandler.Printf("time=\"%s\" level=trace msg=\"[traefik-umami-feeder] %s\"", time, fmt.Sprintf(format, v...))
	}
}
