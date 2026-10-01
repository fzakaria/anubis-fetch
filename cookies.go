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

// Cookies (Anubis' `techaro.lol-anubis-auth` JWT, Cloudflare's cf_clearance)
// are persisted per host so a later run is let straight through, exactly like
// a browser revisit. Only name/value are stored — enough to resend; if a token
// has expired the server simply re-challenges and we re-solve.
//
// Cloudflare binds cf_clearance to the User-Agent of the browser that earned
// it, so the UA is stored alongside the cookies and resent with them.

type storedCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// storedSession is one host's cookie file. UserAgent is empty when the
// cookies were earned under the HTTP client's own impersonated UA.
type storedSession struct {
	UserAgent string         `json:"userAgent,omitempty"`
	Cookies   []storedCookie `json:"cookies"`
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

// loadCookies seeds the jar with any cookies previously stored for u's host
// and returns the User-Agent they were issued under ("" if none was stored).
func loadCookies(jar http.CookieJar, u *url.URL) string {
	b, err := os.ReadFile(cookieFile(u.Host))
	if err != nil {
		return ""
	}

	// Files written before the UA was stored hold a bare cookie array.
	var session storedSession
	if json.Unmarshal(b, &session) != nil {
		if json.Unmarshal(b, &session.Cookies) != nil {
			return ""
		}
	}

	cookies := make([]*http.Cookie, 0, len(session.Cookies))
	for _, c := range session.Cookies {
		cookies = append(cookies, &http.Cookie{Name: c.Name, Value: c.Value})
	}
	jar.SetCookies(u, cookies)
	return session.UserAgent
}

// saveCookies writes the jar's cookies for u's host back to disk, together
// with the User-Agent that was sent alongside them.
func saveCookies(jar http.CookieJar, u *url.URL, userAgent string) {
	writeCookies(u.Host, jar.Cookies(u), userAgent)
}

func writeCookies(host string, cookies []*http.Cookie, userAgent string) {
	if len(cookies) == 0 {
		return
	}
	session := storedSession{
		UserAgent: userAgent,
		Cookies:   make([]storedCookie, 0, len(cookies)),
	}
	for _, c := range cookies {
		session.Cookies = append(session.Cookies, storedCookie{Name: c.Name, Value: c.Value})
	}
	b, err := json.Marshal(session)
	if err != nil {
		return
	}
	path := cookieFile(host)
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o600)
}
