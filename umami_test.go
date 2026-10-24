package traefik_umami_feeder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTraefikUmamiFeeder(t *testing.T) {
	cfg := CreateConfig()
	cfg.UmamiHost = "http://localhost:3000"
	cfg.UmamiUsername = "admin"
	cfg.UmamiPassword = "umami"
	cfg.UmamiTeamId = "8e39c6ad-e44a-4d3e-be98-015db2d62d40"
	cfg.CreateNewWebsites = true

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := New(ctx, next, cfg, "umami-feeder")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:80", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)
}

func TestShouldTrackDefault(t *testing.T) {
	feeder := &UmamiFeeder{}

	assertResource(t, feeder, true, "http://localhost")
	assertResource(t, feeder, true, "http://localhost/about")
	assertResource(t, feeder, true, "http://localhost/products.html")
	assertResource(t, feeder, true, "http://localhost/blog.php")
	assertResource(t, feeder, true, "http://localhost/feed.rss")
	assertResource(t, feeder, false, "http://localhost/favicon.ico")
	assertResource(t, feeder, false, "http://localhost/photo.jpg")
	assertResource(t, feeder, false, "http://localhost/background.png")
	assertResource(t, feeder, true, "http://localhost/Index.HTML")
}

func assertResource(t *testing.T, plugin *UmamiFeeder, expected bool, url string) {
	t.Helper()
	if expected != plugin.shouldTrackResource(url) {
		t.Fatalf("expected %v for %s", expected, url)
	}
}

func TestShouldTrackInvalidIp(t *testing.T) {
	feeder := UmamiFeeder{}
	err := feeder.verifyConfig(&Config{
		IgnoreIPs: []string{"127.0.0.1-127.0.0.10"},
	})

	if err == nil {
		t.Fatal("should have failed with invalid IP")
	}
}

func TestShouldTrackIps(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true}
	err := feeder.verifyConfig(&Config{
		IgnoreIPs: []string{"127.0.0.1", "10.0.0.1/24"},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertIgnoreIP(t, feeder, true, "192.168.0.1")
	assertIgnoreIP(t, feeder, false, "127.0.0.1")
	assertIgnoreIP(t, feeder, false, "10.0.0.1")
	assertIgnoreIP(t, feeder, false, "10.0.0.255")
	assertIgnoreIP(t, feeder, true, "10.0.1.1")
	assertIgnoreIP(t, feeder, true, "10.10.10.1")
	assertIgnoreIP(t, feeder, true, "1.1.1.1")
	assertIgnoreIP(t, feeder, true, "8.8.8.8")
	assertIgnoreIP(t, feeder, false, "::ffff:10.0.0.5")
	assertIgnoreIP(t, feeder, false, "[::ffff:127.0.0.1]:1234")
	assertIgnoreIP(t, feeder, true, "::ffff:8.8.8.8")
}

func TestShouldTrackIpv6(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true}
	err := feeder.verifyConfig(&Config{
		IgnoreIPs: []string{"::1", "2001:db8::/32"},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertIgnoreIP(t, feeder, false, "::1")
	assertIgnoreIP(t, feeder, true, "::2")
	assertIgnoreIP(t, feeder, false, "2001:db8::1")
	assertIgnoreIP(t, feeder, true, "2001:db9::1")
}

func TestShouldTrackRemoteAddr(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true, headerIp: "X-Real-IP"}
	err := feeder.verifyConfig(&Config{
		IgnoreIPs: []string{"127.0.0.1", "::1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for remoteAddr, expected := range map[string]bool{
		"192.168.0.1:12345": true,
		"127.0.0.1:12345":   false,
		"[2001:db8::1]:443": true,
		"[::1]:443":         false,
	} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost", nil)
		req.RemoteAddr = remoteAddr

		if expected != feeder.shouldTrackRequest(req, parseDomainFromHost(req.Host)) {
			t.Fatalf("expected %v for %s", expected, remoteAddr)
		}
	}
}

func TestParseDomainFromHost(t *testing.T) {
	for host, expected := range map[string]string{
		"example.com":      "example.com",
		"Example.COM:8080": "example.com",
		"example.com.":     "example.com",
		"127.0.0.1:80":     "127.0.0.1",
		"[::1]:80":         "::1",
		"[2001:db8::1]":    "2001:db8::1",
	} {
		if actual := parseDomainFromHost(host); actual != expected {
			t.Fatalf("expected %s for %s, got %s", expected, host, actual)
		}
	}
}

func assertIgnoreIP(t *testing.T, plugin *UmamiFeeder, expected bool, clientIP string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost", nil)
	req.Header.Set(plugin.headerIp, clientIP)

	if expected != plugin.shouldTrackRequest(req, parseDomainFromHost(req.Host)) {
		t.Fatalf("expected %v for %s", expected, clientIP)
	}
}

func TestShouldTrackHosts(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true, ignoreHosts: hostSet([]string{"localhost", "Internal.Example.com."})}

	assertIgnoreUrl(t, feeder, false, "http://localhost/about")
	assertIgnoreUrl(t, feeder, false, "http://LOCALHOST/about")
	assertIgnoreUrl(t, feeder, false, "http://localhost:8080/about")
	assertIgnoreUrl(t, feeder, false, "http://internal.example.com./")
	assertIgnoreUrl(t, feeder, true, "https://about.localhost/")
	assertIgnoreUrl(t, feeder, false, "https://internal.example.com/welcome")
	assertIgnoreUrl(t, feeder, true, "https://EXAMPLE.COM")
}

func TestShouldTrackHostsWithPort(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true, ignoreHosts: hostSet([]string{"Example.com:8080"})}

	assertIgnoreUrl(t, feeder, false, "http://example.com:8080/")
	assertIgnoreUrl(t, feeder, true, "http://example.com/")
	assertIgnoreUrl(t, feeder, true, "http://example.com:443/")
	assertIgnoreUrl(t, feeder, false, "http://example.com.:8080/")

	feeder = &UmamiFeeder{createNewWebsites: true, ignoreHosts: hostSet([]string{"example.com.:8080", "[::1]:8080"})}
	assertIgnoreUrl(t, feeder, false, "http://example.com:8080/")
	assertIgnoreUrl(t, feeder, false, "http://[::1]:8080/")
	assertIgnoreUrl(t, feeder, true, "http://[::1]:80/")
}

func TestParseDomainFromWebsite(t *testing.T) {
	for domain, expected := range map[string]string{
		"example.com":               "example.com",
		"https://Example.com/":      "example.com",
		"http://example.com:8080/x": "example.com",
		"example.com/blog":          "example.com",
	} {
		if actual := parseDomainFromWebsite(domain); actual != expected {
			t.Fatalf("expected %s for %s, got %s", expected, domain, actual)
		}
	}
}

func TestVerifyConfigIgnoreIPs(t *testing.T) {
	feeder := &UmamiFeeder{headerIp: "X-Real-IP"}
	if err := feeder.verifyConfig(&Config{IgnoreIPs: []string{"::ffff:10.0.0.0/104", "::ffff:192.168.0.7"}}); err != nil {
		t.Fatal(err)
	}

	for _, ip := range []string{"10.0.0.5", "::ffff:10.0.0.5", "192.168.0.7", "::ffff:192.168.0.7"} {
		assertIgnoreIP(t, feeder, false, ip)
	}

	for _, ignoreIP := range []string{"192.168.1.0:24", "10.0.0.1:80", "[::1]:80"} {
		if err := (&UmamiFeeder{}).verifyConfig(&Config{IgnoreIPs: []string{ignoreIP}}); err == nil {
			t.Fatalf("expected %q to be rejected", ignoreIP)
		}
	}
}

func TestShouldTrackUrls(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true}
	err := feeder.verifyConfig(&Config{
		IgnoreURLs: []string{"/about", "^/admin", "world$"},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertIgnoreUrl(t, feeder, true, "/")
	assertIgnoreUrl(t, feeder, false, "/about")
	assertIgnoreUrl(t, feeder, false, "/aboutus")
	assertIgnoreUrl(t, feeder, false, "/category/about")
	assertIgnoreUrl(t, feeder, true, "/world/news")
	assertIgnoreUrl(t, feeder, false, "/hello-world")
	assertIgnoreUrl(t, feeder, false, "/admin/secret")
}

func assertIgnoreUrl(t *testing.T, plugin *UmamiFeeder, expected bool, url string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)

	if expected != plugin.shouldTrackRequest(req, parseDomainFromHost(req.Host)) {
		t.Fatalf("expected %v for %s", expected, url)
	}
}

func TestShouldTrackUserAgents(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true, ignoreUserAgents: []string{"Googlebot", "Uptime-Kuma"}}

	assertIgnoreUa(t, feeder, true, "Mozilla/5.0 (Windows; Windows NT 6.0; WOW64) Gecko/20100101 Firefox/60.7")
	assertIgnoreUa(t, feeder, true, "Mozilla/5.0 (compatible; MSIE 10.0; Windows NT 10.0; Win64; x64 Trident/6.0)")
	assertIgnoreUa(t, feeder, true, "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	assertIgnoreUa(t, feeder, false, "Uptime-Kuma/1.18.5")
	assertIgnoreUa(t, feeder, false, "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/90.0.4430.212 Safari/537.36 Uptime-Kuma/1.23.1")
	assertIgnoreUa(t, feeder, true, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	assertIgnoreUa(t, feeder, false, "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	assertIgnoreUa(t, feeder, false, "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/W.X.Y.Z Mobile Safari/537.36 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
}

func assertIgnoreUa(t *testing.T, plugin *UmamiFeeder, expected bool, ua string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost/", nil)
	req.Header.Set("User-Agent", ua)

	if expected != plugin.shouldTrackRequest(req, parseDomainFromHost(req.Host)) {
		t.Fatalf("expected %v for %s", expected, ua)
	}
}

func TestShouldTrackCustomExtensions(t *testing.T) {
	feeder := &UmamiFeeder{trackExtensions: extensionSet([]string{".JS", ""})}

	assertResource(t, feeder, true, "http://localhost/app.js")
	assertResource(t, feeder, true, "http://localhost/about")
	assertResource(t, feeder, false, "http://localhost/index.html")
}

func TestNewNormalizesWebsites(t *testing.T) {
	cfg := CreateConfig()
	cfg.Enabled = false
	cfg.Websites = map[string]string{"Example.com.": "id"}

	handler, err := New(context.Background(), http.NotFoundHandler(), cfg, "umami-feeder")
	if err != nil {
		t.Fatal(err)
	}

	if websiteId := handler.(*UmamiFeeder).websites["example.com"]; websiteId != "id" {
		t.Fatalf("expected normalized website, got %q", websiteId)
	}
}
