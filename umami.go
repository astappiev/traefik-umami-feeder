package traefik_umami_feeder

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/netip"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Config defines the plugin configuration.
type Config struct {
	// Enabled controls whether the plugin is enabled. Set to `false` to disable the plugin.
	Enabled bool `json:"enabled"`
	// Disabled disables the plugin. Deprecated: use Enabled instead.
	Disabled bool `json:"disabled"`
	// Debug enables debug logging, be prepared for flooding, use for troubleshooting.
	Debug bool `json:"debug"`
	// QueueSize defines the size of queue, i.e. the amount of events that are waiting to be submitted to Umami.
	QueueSize int `json:"queueSize"`
	// BatchSize defines the amount of events that are submitted to Umami in one request.
	BatchSize int `json:"batchSize"`
	// BatchMaxWait defines the maximum time to wait before submitting the batch.
	BatchMaxWait time.Duration `json:"batchMaxWait"`

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

	// TrackErrors defines whether errors (status codes >= 400) should be tracked.
	TrackErrors bool `json:"trackErrors"`
	// TrackAllResources defines whether all requests for any resource should be tracked.
	// By default, only requests that are believed to contain content are tracked.
	TrackAllResources bool `json:"trackAllResources"`
	// TrackExtensions defines an alternative list of file extensions that should be tracked.
	TrackExtensions []string `json:"trackExtensions"`

	// IgnoreUserAgents is a list of user agents to ignore.
	IgnoreUserAgents []string `json:"ignoreUserAgents"`
	// IgnoreURLs is a list of request urls to ignore, each string is converted to RegExp and paths matched against it.
	IgnoreURLs []string `json:"ignoreURLs"`
	// IgnoreHosts is a list of hosts to ignore.
	IgnoreHosts []string `json:"ignoreHosts"`
	// IgnoreIPs is a list of IPs or CIDRs to ignore.
	IgnoreIPs []string `json:"ignoreIPs"`
	// HeaderIp is the header name associated with the real IP address.
	HeaderIp string `json:"headerIp"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		Disabled:     false,
		Enabled:      true,
		Debug:        false,
		QueueSize:    1000,
		BatchSize:    20,
		BatchMaxWait: 5 * time.Second,
		TrackErrors:  false,

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
		IgnoreURLs:       []string{},
		IgnoreHosts:      []string{},
		IgnoreIPs:        []string{},
		HeaderIp:         "X-Real-IP",
	}
}

// UmamiFeeder a UmamiFeeder plugin.
type UmamiFeeder struct {
	next       http.Handler
	name       string
	isDebug    bool
	isEnabled  bool
	logHandler *log.Logger
	queue      chan *UmamiEvent

	batchSize    int
	batchMaxWait time.Duration

	umamiHost         string
	umamiToken        string
	umamiTeamId       string
	websites          map[string]string
	websitesMutex     sync.RWMutex
	createNewWebsites bool

	trackErrors       bool
	trackAllResources bool
	trackExtensions   []string

	ignoreHosts      []string
	ignoreUserAgents []string
	ignoreRegexps    []regexp.Regexp
	ignorePrefixes   []netip.Prefix
	headerIp         string
}

// New creates a new UmamiFeeder plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	h := &UmamiFeeder{
		next:       next,
		name:       name,
		isDebug:    config.Debug,
		isEnabled:  config.Enabled && !config.Disabled,
		logHandler: log.New(os.Stdout, "", 0),

		queue:        make(chan *UmamiEvent, config.QueueSize),
		batchSize:    config.BatchSize,
		batchMaxWait: config.BatchMaxWait,

		umamiHost:         config.UmamiHost,
		umamiToken:        config.UmamiToken,
		umamiTeamId:       config.UmamiTeamId,
		websites:          config.Websites,
		websitesMutex:     sync.RWMutex{},
		createNewWebsites: config.CreateNewWebsites,

		trackErrors:       config.TrackErrors,
		trackAllResources: config.TrackAllResources,
		trackExtensions:   config.TrackExtensions,

		ignoreHosts:      config.IgnoreHosts,
		ignoreUserAgents: config.IgnoreUserAgents,
		ignoreRegexps:    []regexp.Regexp{},
		ignorePrefixes:   []netip.Prefix{},
		headerIp:         config.HeaderIp,
	}

	if h.isEnabled {
		h.isEnabled = false // Disable until connection and config verification is done.
		go h.retryConnection(ctx, config)
	}

	return h, nil
}

func (h *UmamiFeeder) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if h.isEnabled && h.shouldTrack(req) {
		// If the resource should be reported, we wrap the response writer and check the status code before reporting
		responseWrapper := &ResponseWrapper{
			ResponseWriter: rw,
			request:        req,
			feeder:         h,
		}

		// Continue with next handler.
		h.next.ServeHTTP(responseWrapper, req)
		return
	}

	h.next.ServeHTTP(rw, req)
}

func (h *UmamiFeeder) retryConnection(ctx context.Context, config *Config) {
	const maxRetryInterval = time.Hour
	retryAttempt := 0
	for {
		currentDelay := maxRetryInterval
		if retryAttempt == 0 {
			currentDelay = 0
		} else if retryAttempt < 8 {
			currentDelay = time.Duration(15*math.Pow(2, float64(retryAttempt))) * time.Second
		}

		if retryAttempt > 0 { // Don't log for the immediate first attempt
			h.debugf("Next connection attempt in %v (attempt #%d).", currentDelay, retryAttempt+1)
		}

		select {
		case <-time.After(currentDelay):
			retryAttempt++
			h.debugf("Attempting to connect to Umami (attempt #%d)", retryAttempt)

			err := h.connect(ctx, config)
			if err == nil {
				h.debugf("Successfully connected to Umami. Verifying configuration...")

				err = h.verifyConfig(config)
				if err == nil {
					h.debugf("Configuration verified. Enabling plugin and starting worker.")
					h.isEnabled = true
					go h.startWorker(ctx)
					return // Successfully connected and configured, exit retry goroutine
				}

				h.error("Configuration error, the plugin is disabled: " + err.Error())
				h.isEnabled = false
				return // Exit retry goroutine, plugin remains disabled.
			}

			h.error("Failed to reconnect to Umami: " + err.Error())
		case <-ctx.Done():
			h.debugf("Context canceled during retryConnection, stopping connection retries.")
			return
		}
	}
}

func (h *UmamiFeeder) connect(ctx context.Context, config *Config) error {
	if h.umamiHost == "" {
		return errors.New("umamiHost is not set")
	}

	if config.UmamiUsername != "" && config.UmamiPassword != "" {
		token, err := getToken(ctx, h.umamiHost, config.UmamiUsername, config.UmamiPassword)
		if err != nil {
			return fmt.Errorf("failed to get token: %w", err)
		}
		if token == "" {
			return errors.New("retrieved token is empty")
		}
		h.debugf("token received %s", token)
		h.umamiToken = token
	}
	if h.umamiToken == "" && len(h.websites) == 0 {
		return errors.New("either umamiToken or websites must be set")
	}
	if h.umamiToken == "" && h.createNewWebsites {
		return errors.New("umamiToken is required to create new websites")
	}

	if h.umamiToken != "" {
		websites, err := fetchWebsites(ctx, h.umamiHost, h.umamiToken, h.umamiTeamId)
		if err != nil {
			return fmt.Errorf("failed to fetch websites: %w", err)
		}

		h.websitesMutex.Lock()
		for _, website := range *websites {
			if _, ok := h.websites[website.Domain]; !ok {
				h.websites[website.Domain] = website.ID
			}
		}
		h.websitesMutex.Unlock()
		h.debugf("websites fetched: %v", h.websites)
	}

	return nil
}

func (h *UmamiFeeder) verifyConfig(config *Config) error {
	if len(config.IgnoreIPs) > 0 {
		for _, ignoreIP := range config.IgnoreIPs {
			network, err := netip.ParsePrefix(ignoreIP)
			if err != nil {
				network, err = netip.ParsePrefix(ignoreIP + "/32")
			}

			if err != nil || !network.IsValid() {
				return fmt.Errorf("invalid ignoreIP given %s: %w", ignoreIP, err)
			}

			h.ignorePrefixes = append(h.ignorePrefixes, network)
		}
	}

	if len(config.IgnoreURLs) > 0 {
		for _, location := range config.IgnoreURLs {
			r, err := regexp.Compile(location)
			if err != nil {
				return fmt.Errorf("failed to compile ignoreURL %s: %w", location, err)
			}

			h.ignoreRegexps = append(h.ignoreRegexps, *r)
		}
	}

	return nil
}

func (h *UmamiFeeder) shouldTrackRequest(req *http.Request) bool {
	if len(h.ignoreHosts) > 0 {
		for _, disabledHost := range h.ignoreHosts {
			if strings.EqualFold(req.Host, disabledHost) {
				h.debugf("ignoring host %s", req.Host)
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
			h.debugf("invalid IP %s", requestIp)
			return false
		}

		for _, prefix := range h.ignorePrefixes {
			if prefix.Contains(ip) {
				h.debugf("ignoring IP %s", ip)
				return false
			}
		}
	}

	if len(h.ignoreUserAgents) > 0 {
		userAgent := req.UserAgent()
		for _, disabledUserAgent := range h.ignoreUserAgents {
			if strings.Contains(userAgent, disabledUserAgent) {
				h.debugf("ignoring user-agent %s", userAgent)
				return false
			}
		}
	}

	if len(h.ignoreRegexps) > 0 {
		requestURL := req.URL.String()
		for _, r := range h.ignoreRegexps {
			if r.MatchString(requestURL) {
				h.debugf("ignoring location %s", requestURL)
				return false
			}
		}
	}

	return true
}

func (h *UmamiFeeder) shouldTrack(req *http.Request) bool {
	if !h.shouldTrackRequest(req) {
		return false
	}

	if !h.shouldTrackResource(req.URL.Path) {
		h.debugf("ignoring resource %s", req.URL.Path)
		return false
	}

	if h.createNewWebsites {
		return true
	}

	hostname := parseDomainFromHost(req.Host)
	h.websitesMutex.RLock()
	if _, ok := h.websites[hostname]; ok {
		h.websitesMutex.RUnlock()
		return true
	}
	h.websitesMutex.RUnlock()

	h.debugf("ignoring domain %s", hostname)
	return false
}

func (h *UmamiFeeder) shouldTrackResource(url string) bool {
	if h.trackAllResources {
		return true
	}

	pathExt := path.Ext(url)

	// If a custom file extension list is defined, check if the resource matches it. If not, do not report.
	if len(h.trackExtensions) > 0 {
		return slices.Contains(h.trackExtensions, pathExt)
	}

	// Check if the suffix is regarded to be "content".
	switch pathExt {
	case "", ".htm", ".html", ".xhtml", ".jsf", ".md", ".php", ".rss", ".rtf", ".txt", ".xml", ".pdf":
		return true
	}

	return false
}

func (h *UmamiFeeder) shouldTrackStatus(statusCode int) bool {
	if statusCode >= 400 {
		if h.trackErrors {
			return true
		}

		h.debugf("not reporting %d error", statusCode)
		return false
	}
	return true
}

func (h *UmamiFeeder) error(message string) {
	if h.logHandler != nil {
		now := time.Now().Format("2006-01-02T15:04:05Z")
		h.logHandler.Printf("%s ERR middlewareName=%s error=\"%s\"", now, h.name, message)
	}
}

// Arguments are handled in the manner of [fmt.Printf].
func (h *UmamiFeeder) debugf(format string, v ...any) {
	if h.logHandler != nil && h.isDebug {
		now := time.Now().Format("2006-01-02T15:04:05Z")
		h.logHandler.Printf("%s DBG middlewareName=%s msg=\"%s\"", now, h.name, fmt.Sprintf(format, v...))
	}
}
