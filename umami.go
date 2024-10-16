package traefik_umami_feeder

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config the plugin configuration.
type Config struct {
	// basic plugin configuration
	Disabled bool `json:"disabled"`
	Debug    bool `json:"debug"`

	// Umami configuration
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

	// filters to ignore requests and do not send view events
	IgnoreUserAgents []string `json:"ignoreUserAgents"`
	IgnoreIPs        []string `json:"ignoreIPs"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		Disabled: false,
		Debug:    false,

		UmamiHost:     "",
		UmamiToken:    "",
		UmamiUsername: "",
		UmamiPassword: "",
		UmamiTeamId:   "",

		Websites:          map[string]string{},
		CreateNewWebsites: false,

		IgnoreUserAgents: []string{},
		IgnoreIPs:        []string{},
	}
}

// UmamiFeeder a UmamiFeeder plugin.
type UmamiFeeder struct {
	next       http.Handler
	name       string
	debug      bool
	disabled   bool
	logHandler *log.Logger

	umamiHost         string
	umamiToken        string
	umamiTeamId       string
	websites          map[string]string
	createNewWebsites bool

	ignoreUserAgents []string
	ignoreIPs        []string
}

// New created a new Demo plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// construct
	h := &UmamiFeeder{
		next:       next,
		name:       name,
		debug:      config.Debug,
		disabled:   config.Disabled,
		logHandler: log.New(os.Stdout, "", 0),

		umamiHost:         config.UmamiHost,
		umamiToken:        config.UmamiToken,
		umamiTeamId:       config.UmamiTeamId,
		websites:          config.Websites,
		createNewWebsites: config.CreateNewWebsites,

		ignoreUserAgents: config.IgnoreUserAgents,
		ignoreIPs:        config.IgnoreIPs,
	}

	if !h.disabled {
		if config.UmamiUsername != "" && config.UmamiPassword != "" {
			token, err := getToken(h.umamiHost, config.UmamiUsername, config.UmamiPassword)
			if err != nil {
				return nil, fmt.Errorf("failed to get token: %w", err)
			}
			if token == "" {
				return nil, fmt.Errorf("retrieved token is empty")
			}
			h.trace("token received %s", token)
			h.umamiToken = token
		}

		if h.umamiHost == "" {
			return nil, fmt.Errorf("`umamiHost` is not set")
		}
		if h.umamiToken == "" && len(h.websites) == 0 {
			return nil, fmt.Errorf("either `umamiToken` or `websites` should be set")
		}
		if h.umamiToken == "" && h.createNewWebsites {
			return nil, fmt.Errorf("`umamiToken` is required to create new websites")
		}

		if h.umamiToken != "" {
			websites, err := fetchWebsites(h.umamiHost, h.umamiToken, h.umamiTeamId)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch websites: %w", err)
			}

			for _, website := range *websites {
				if _, ok := h.websites[website.Domain]; ok {
					continue
				}

				h.websites[website.Domain] = website.ID
				h.trace("fetched websiteId for: %s", website.Domain)
			}
			h.log("websites fetched")
		}
	}

	return h, nil
}

func (h *UmamiFeeder) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !h.disabled {
		if h.shouldBeTracked(req) {
			go h.trackRequest(req)
		} else {
			h.trace("Tracking skipped %v", req.URL)
		}
	}

	h.next.ServeHTTP(rw, req)
}

func (h *UmamiFeeder) shouldBeTracked(req *http.Request) bool {
	if len(h.ignoreUserAgents) > 0 {
		userAgent := req.UserAgent()
		for _, disabledUserAgent := range h.ignoreUserAgents {
			if strings.Contains(userAgent, disabledUserAgent) {
				return false
			}
		}
	}

	if len(h.ignoreIPs) > 0 {
		requestIp := req.RemoteAddr
		for _, disabledIp := range h.ignoreIPs {
			if requestIp == disabledIp {
				return false
			}
		}
	}

	if h.createNewWebsites {
		return true
	}

	hostname := parseDomainFromHost(req.Host)
	if _, ok := h.websites[hostname]; ok {
		return true
	}

	return false
}

func (h *UmamiFeeder) trackRequest(req *http.Request) {
	hostname := parseDomainFromHost(req.Host)
	websiteId, ok := h.websites[hostname]
	if !ok {
		website, err := createWebsite(h.umamiHost, h.umamiToken, h.umamiTeamId, hostname)
		if err != nil {
			h.log("failed to create website: " + err.Error())
			return
		}

		h.websites[website.Domain] = website.ID
		websiteId = website.ID
		h.trace("created website for: %s", website.Domain)
	}

	sendBody, sendHeaders := buildSendBody(req, websiteId)
	h.trace("sending tracking request %s with body %v %v", req.URL, sendBody, sendHeaders)

	_, err := sendRequest(h.umamiHost+"/api/send", sendBody, sendHeaders)
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
