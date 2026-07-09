package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"

	"golang.org/x/net/publicsuffix"
)

// Cookies (chiefly Anubis' `techaro.lol-anubis-auth` JWT) are persisted per
// host so a later run is let straight through, exactly like a browser revisit.
// Only name/value are stored — enough to resend; if a token has expired the
// server simply re-challenges and we re-solve.

type storedCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func cacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "anubis-fetch")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "anubis-fetch")
	}
	return filepath.Join(home, ".cache", "anubis-fetch")
}

func cookieFile(host string) string {
	return filepath.Join(cacheDir(), "cookies", host+".json")
}

func newJar() http.CookieJar {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	return jar
}

// loadCookies seeds the jar with any cookies previously stored for u's host.
func loadCookies(jar http.CookieJar, u *url.URL) {
	b, err := os.ReadFile(cookieFile(u.Host))
	if err != nil {
		return
	}
	var stored []storedCookie
	if json.Unmarshal(b, &stored) != nil {
		return
	}
	cookies := make([]*http.Cookie, 0, len(stored))
	for _, c := range stored {
		cookies = append(cookies, &http.Cookie{Name: c.Name, Value: c.Value})
	}
	jar.SetCookies(u, cookies)
}

// saveCookies writes the jar's cookies for u's host back to disk.
func saveCookies(jar http.CookieJar, u *url.URL) {
	writeCookies(u.Host, jar.Cookies(u))
}

func writeCookies(host string, cookies []*http.Cookie) {
	if len(cookies) == 0 {
		return
	}
	stored := make([]storedCookie, 0, len(cookies))
	for _, c := range cookies {
		stored = append(stored, storedCookie{Name: c.Name, Value: c.Value})
	}
	b, err := json.Marshal(stored)
	if err != nil {
		return
	}
	path := cookieFile(host)
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}
