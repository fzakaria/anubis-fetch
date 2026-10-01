package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestCookieSessionRoundTrip writes cookies plus the User-Agent they were
// issued under into a temporary cache dir, then loads them back into a fresh
// jar and checks both survive.
func TestCookieSessionRoundTrip(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	u, _ := url.Parse("https://example.org/page")
	const ua = "Mozilla/5.0 Chrome/149.0.0.0"

	writeCookies(u.Host, []*http.Cookie{{Name: "cf_clearance", Value: "tok"}}, ua)

	jar := newJar()
	if got := loadCookies(jar, u); got != ua {
		t.Errorf("loaded User-Agent %q, want %q", got, ua)
	}
	cookies := jar.Cookies(u)
	if len(cookies) != 1 || cookies[0].Name != "cf_clearance" || cookies[0].Value != "tok" {
		t.Errorf("loaded cookies %v, want cf_clearance=tok", cookies)
	}
}

// TestLoadLegacyCookieFile seeds the cache with the original bare-array file
// format and checks the cookies still load, with no User-Agent attached.
func TestLoadLegacyCookieFile(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	u, _ := url.Parse("https://example.org/page")
	path := cookieFile(u.Host)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `[{"name":"techaro.lol-anubis-auth","value":"jwt"}]`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	jar := newJar()
	if got := loadCookies(jar, u); got != "" {
		t.Errorf("legacy file yielded User-Agent %q, want none", got)
	}
	cookies := jar.Cookies(u)
	if len(cookies) != 1 || cookies[0].Value != "jwt" {
		t.Errorf("loaded cookies %v, want techaro.lol-anubis-auth=jwt", cookies)
	}
}
