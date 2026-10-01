package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// cloudflareChallengeBody is the shape of Cloudflare's managed-challenge
// interstitial: no Anubis markers, only Cloudflare's own bootstrap script.
const cloudflareChallengeBody = `<!DOCTYPE html><html><head><title>Just a moment...</title></head>
<body><span id="challenge-error-text">Enable JavaScript and cookies to continue</span>
<script>(function(){window._cf_chl_opt = {cType: 'managed'};}());</script></body></html>`

// TestFetchViaHTTPEscalatesOnCloudflareChallenge serves Cloudflare's
// challenge response (403 + cf-mitigated: challenge) from a local server and
// checks that the HTTP path hands off to the browser instead of returning the
// interstitial as page content.
func TestFetchViaHTTPEscalatesOnCloudflareChallenge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(cfMitigatedHeader, cfMitigatedChallenge)
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(cloudflareChallengeBody))
	}))
	defer srv.Close()

	html, escalate := fetchViaHTTP(options{url: srv.URL, timeout: 5 * time.Second, noCache: true})
	if !escalate {
		t.Fatalf("Cloudflare challenge not escalated; got page %q", html)
	}
}

// TestChromeUserAgent feeds `chromium --version` output into the UA builder
// and checks that the advertised Chrome major version matches the binary,
// falling back to defaultUA when the output cannot be parsed.
func TestChromeUserAgent(t *testing.T) {
	cases := map[string]string{
		"Chromium 149.0.7827.200 \n":    "Chrome/149.0.0.0 ",
		"Google Chrome 150.0.7900.12\n": "Chrome/150.0.0.0 ",
		"Chromium 151.0.7911.3 snap\n":  "Chrome/151.0.0.0 ",
		"not a version string\n":        defaultUA,
		"":                              defaultUA,
	}
	for out, want := range cases {
		got := chromeUserAgent(out)
		if want == defaultUA {
			if got != defaultUA {
				t.Errorf("chromeUserAgent(%q) = %q, want defaultUA", out, got)
			}
			continue
		}
		if !strings.Contains(got, want) || strings.Contains(got, "Headless") {
			t.Errorf("chromeUserAgent(%q) = %q, want it to contain %q", out, got, want)
		}
	}
}
