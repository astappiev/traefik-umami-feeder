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
}

func assertIgnoreIP(t *testing.T, plugin *UmamiFeeder, expected bool, clientIP string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost", nil)
	req.Header.Set(plugin.headerIp, clientIP)

	if expected != plugin.shouldTrackRequest(req) {
		t.Fatalf("expected %v for %s", expected, clientIP)
	}
}

func TestShouldTrackHosts(t *testing.T) {
	feeder := &UmamiFeeder{createNewWebsites: true, ignoreHosts: []string{"localhost", "internal.example.com"}}

	assertIgnoreUrl(t, feeder, false, "http://localhost/about")
	assertIgnoreUrl(t, feeder, false, "http://LOCALHOST/about")
	assertIgnoreUrl(t, feeder, true, "https://about.localhost/")
	assertIgnoreUrl(t, feeder, false, "https://internal.example.com/welcome")
	assertIgnoreUrl(t, feeder, true, "https://EXAMPLE.COM")
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

	if expected != plugin.shouldTrackRequest(req) {
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

	if expected != plugin.shouldTrackRequest(req) {
		t.Fatalf("expected %v for %s", expected, ua)
	}
}
