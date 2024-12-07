package traefik_umami_feeder

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"path"
	"strings"
	"time"
)

// Config the plugin configuration.
type Config struct {
	// Disabled disables the plugin.
	Disabled bool `json:"disabled"`
	// Debug enables debug logging, be prepared for flooding.
	Debug bool `json:"debug"`

	// UmamiHost is the URL of the Umami instance.
	UmamiHost string `json:"umamiHost"`
	// UmamiToken is an API KEY, which is optional, but either UmamiToken or Websites should be set.
	UmamiToken string `json:"umamiToken"`
	// UmamiUsername could be provided as an alternative to UmamiToken, used to retrieve the token.
	UmamiUsername string `json:"umamiUsername"`
	// UmamiPassword is required if UmamiUsername is set.
	UmamiPassword string `json:"umamiPassword"`
	// UmamiTeamId defines a team, which will be used to retrieve the websites.
	UmamiTeamId string `json:"umamiTeamId"`

	// Websites is a map of domain to websiteId, which is required if UmamiToken is not set.
	// If both UmamiToken and Websites are set, Websites will override/extend domains retrieved from the API.
	Websites map[string]string `json:"websites"`
	// CreateNewWebsites when set to true, the plugin will create new websites using API, UmamiToken is required.
	CreateNewWebsites bool `json:"createNewWebsites"`

	// TrackAllResources defines whether all requests for any resource should be tracked.
	// By default, only requests that are believed to contain content are tracked.
	TrackAllResources bool `json:"trackAllResources"`
	// TrackExtensions defines an alternative list of file extensions that should be tracked.
	TrackExtensions []string `json:"trackExtensions"`

	// IgnoreUserAgents is a list of user agents that should be ignored.
	IgnoreUserAgents []string `json:"ignoreUserAgents"`
	// IgnoreIPs is a list of IPs or CIDRs that should be ignored.
	IgnoreIPs []string `json:"ignoreIPs"`
	// headerIp Header associated to real IP
	HeaderIp string `json:"headerIp"`
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

		TrackAllResources: false,
		TrackExtensions:   []string{},

		IgnoreUserAgents: []string{},
		IgnoreIPs:        []string{},
		HeaderIp:         "X-Real-Ip",
	}
}

// UmamiFeeder a UmamiFeeder plugin.
type UmamiFeeder struct {
	next       http.Handler
	name       string
	isDebug    bool
	isDisabled bool
	logHandler *log.Logger

	umamiHost         string
	umamiToken        string
	umamiTeamId       string
	websites          map[string]string
	createNewWebsites bool

	trackAllResources bool
	trackExtensions   []string

	ignoreUserAgents []string
	ignorePrefixes   []netip.Prefix
	headerIp         string
}

// New created a new Demo plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// construct
	h := &UmamiFeeder{
		next:       next,
		name:       name,
		isDebug:    config.Debug,
		isDisabled: config.Disabled,
		logHandler: log.New(os.Stdout, "", 0),

		umamiHost:         config.UmamiHost,
		umamiToken:        config.UmamiToken,
		umamiTeamId:       config.UmamiTeamId,
		websites:          config.Websites,
		createNewWebsites: config.CreateNewWebsites,

		trackAllResources: config.TrackAllResources,
		trackExtensions:   config.TrackExtensions,

		ignoreUserAgents: config.IgnoreUserAgents,
		ignorePrefixes:   []netip.Prefix{},
		headerIp:         config.HeaderIp,
	}

	if !h.isDisabled {
		err := h.verifyConfig(config)
		if err != nil {
			h.error(err.Error())
			h.error("due to the error, the Umami plugin is disabled")
			h.isDisabled = true
		}
	}

	if len(config.IgnoreIPs) > 0 {
		for _, ignoreIp := range config.IgnoreIPs {
			network, err := netip.ParsePrefix(ignoreIp)
			if err != nil {
				network, err = netip.ParsePrefix(ignoreIp + "/32")
			}

			if err != nil || !network.IsValid() {
				if err != nil {
					h.error(err.Error())
				}
				h.error(fmt.Sprintf("invalid ignoreIp given %s, this param accepts only IP addresses or CIRD in a format 10.0.0.1/16", ignoreIp))
				h.isDisabled = true
			} else {
				h.ignorePrefixes = append(h.ignorePrefixes, network)
			}
		}
	}

	return h, nil
}

func (h *UmamiFeeder) verifyConfig(config *Config) error {
	if h.umamiHost == "" {
		return fmt.Errorf("`umamiHost` is not set")
	}

	if config.UmamiUsername != "" && config.UmamiPassword != "" {
		token, err := getToken(h.umamiHost, config.UmamiUsername, config.UmamiPassword)
		if err != nil {
			return fmt.Errorf("failed to get token: %w", err)
		}
		if token == "" {
			return fmt.Errorf("retrieved token is empty")
		}
		h.debug("token received %s", token)
		h.umamiToken = token
	}
	if h.umamiToken == "" && len(h.websites) == 0 {
		return fmt.Errorf("either `umamiToken` or `websites` should be set")
	}
	if h.umamiToken == "" && h.createNewWebsites {
		return fmt.Errorf("`umamiToken` is required to create new websites")
	}

	if h.umamiToken != "" {
		websites, err := fetchWebsites(h.umamiHost, h.umamiToken, h.umamiTeamId)
		if err != nil {
			return fmt.Errorf("failed to fetch websites: %w", err)
		}

		for _, website := range *websites {
			if _, ok := h.websites[website.Domain]; ok {
				continue
			}

			h.websites[website.Domain] = website.ID
			h.debug("fetched websiteId for: %s", website.Domain)
		}
	}

	return nil
}

func (h *UmamiFeeder) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !h.isDisabled {
		if h.shouldTrack(req) {
			go h.trackRequest(req)
		} else {
			h.debug("ignoring request to %s%s", req.Host, req.URL)
		}
	}

	h.next.ServeHTTP(rw, req)
}

func (h *UmamiFeeder) shouldTrack(req *http.Request) bool {
	if len(h.ignoreUserAgents) > 0 {
		userAgent := req.UserAgent()
		for _, disabledUserAgent := range h.ignoreUserAgents {
			if strings.Contains(userAgent, disabledUserAgent) {
				return false
			}
		}
	}

	if len(h.ignorePrefixes) > 0 {
		requestIp := req.Header.Get(h.headerIp)
		if requestIp == "" {
			requestIp = req.RemoteAddr
		}

		ip, err := netip.ParseAddr(requestIp)
		if err != nil {
			h.debug("invalid IP %s", requestIp)
			return false
		}

		for _, prefix := range h.ignorePrefixes {
			if prefix.Contains(ip) {
				return false
			}
		}
	}

	if !h.shouldTrackResource(req.URL.Path) {
		return false
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

func (h *UmamiFeeder) shouldTrackResource(url string) bool {
	if h.trackAllResources {
		return true
	}

	pathExt := path.Ext(url)

	// If a custom file extension list is defined, check if the resource matches it. If not, do not report.
	if len(h.trackExtensions) > 0 {
		for _, suffix := range h.trackExtensions {
			if suffix == pathExt {
				return true
			}
		}
		return false
	}

	// Check if the suffix is regarded to be "content".
	switch pathExt {
	case ".htm":
	case ".html":
	case ".xhtml":
	case ".jsf":
	case ".md":
	case ".php":
	case ".rss":
	case ".rtf":
	case ".txt":
	case ".xml":
	case ".pdf":
	case "":
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
			h.error("failed to create website: " + err.Error())
			return
		}

		h.websites[website.Domain] = website.ID
		websiteId = website.ID
		h.debug("created website for: %s", website.Domain)
	}

	sendBody, sendHeaders := buildSendBody(req, websiteId)
	h.debug("sending tracking request %s with body %v %v", req.URL, sendBody, sendHeaders)

	_, err := sendRequest(h.umamiHost+"/api/send", sendBody, sendHeaders)
	if err != nil {
		h.error("failed to send tracking: " + err.Error())
		return
	}
}

func (h *UmamiFeeder) error(message string) {
	if h.logHandler != nil {
		time := time.Now().Format("2006-01-02T15:04:05Z")
		h.logHandler.Printf("%s ERR middlewareName=%s error=\"%s\"", time, h.name, message)
	}
}

// Arguments are handled in the manner of [fmt.Printf].
func (h *UmamiFeeder) debug(format string, v ...any) {
	if h.logHandler != nil && h.isDebug {
		time := time.Now().Format("2006-01-02T15:04:05Z")
		h.logHandler.Printf("%s DBG middlewareName=%s msg=\"%s\"", time, h.name, fmt.Sprintf(format, v...))
	}
}
