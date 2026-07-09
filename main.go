// Command anubis-fetch fetches a URL from behind a JavaScript bot-wall.
//
// It handles two very different walls with one tool:
//
//   - Anubis (github.com/TecharoHQ/anubis) — a SHA-256 proof-of-work
//     interstitial fronting lore.kernel.org, GNOME, kernel.org, and friends.
//     anubis-fetch solves the proof-of-work in-process, no browser needed.
//   - Cloudflare-style passive fingerprinting (TLS/JA3 + HTTP2) — cleared by
//     impersonating a real Chrome at the transport layer via curl-impersonate
//     (through the req library).
//
// Strategy, cheapest first:
//
//  1. Reuse a persisted cookie (a prior Anubis auth token) — like a browser,
//     a revisit isn't re-challenged.
//  2. Solve the proof-of-work in-process over the impersonating HTTP client.
//  3. Fall back to a real headless Chromium (chromedp) for anything the fast
//     path can't do: the preact/metarefresh challenge methods, an unknown or
//     future method, a difficulty too high to brute-force, a rejected solution,
//     or a Cloudflare *active* JS challenge.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"jaytaylor.com/html2text"
)

const (
	passChallengePath = "/.within.website/x/cmd/anubis/api/pass-challenge"
	// escalateExit is returned by --no-browser when the in-process solve fails.
	escalateExit = 3
	// maxDifficulty caps the in-process brute force. Go's SHA-256 is fast
	// (~16M hashes ≈ a few seconds at difficulty 6); above that a browser's
	// parallel workers win, so we escalate instead.
	maxDifficulty = 6
	defaultUA     = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

// solvableMethods are the Anubis challenge methods we can solve without a
// browser. "fast" is the default; "slow" is a deprecated, identical variant.
var solvableMethods = map[string]bool{"fast": true, "slow": true}

type options struct {
	url       string
	timeout   time.Duration
	ua        string
	text      bool
	browser   bool // skip the solver, go straight to the browser
	noBrowser bool // never use the browser; exit escalateExit if solving fails
	noCache   bool // don't read/write the persistent cookie jar
}

func usage() {
	fmt.Fprint(os.Stderr, `anubis-fetch — fetch a URL from behind Anubis / Cloudflare bot-walls

Usage:
  anubis-fetch [flags] URL

Flags:
  --text            render readable plain text instead of HTML
  --timeout MS      per-step timeout in milliseconds (default 30000)
  --ua STRING       override the User-Agent
  --browser         skip the in-process solver; use the headless browser
  --no-browser      never use the browser; exit 3 if the solve can't apply
  --no-cache        don't read or write the persistent cookie jar
`)
}

func main() {
	os.Exit(run())
}

func run() int {
	o := parseFlags()

	if !o.browser {
		html, escalate := fetchViaHTTP(o)
		if !escalate {
			output(html, o)
			return 0
		}
		if o.noBrowser {
			fmt.Fprintln(os.Stderr, "anubis-fetch: cannot solve in-process and --no-browser is set")
			return escalateExit
		}
	}

	html, err := fetchViaBrowser(o)
	if err != nil {
		fmt.Fprintf(os.Stderr, "anubis-fetch: browser error: %v\n", err)
		if html == "" {
			return 1
		}
	}
	output(html, o)
	return 0
}

func parseFlags() options {
	var o options
	var timeoutMs int
	flag.IntVar(&timeoutMs, "timeout", 30000, "per-step timeout in milliseconds")
	flag.StringVar(&o.ua, "ua", "", "override User-Agent")
	flag.BoolVar(&o.text, "text", false, "render readable plain text instead of HTML")
	flag.BoolVar(&o.browser, "browser", false, "skip the solver; use the browser")
	flag.BoolVar(&o.noBrowser, "no-browser", false, "never use the browser; exit 3 on failure")
	flag.BoolVar(&o.noCache, "no-cache", false, "don't read or write the cookie jar")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
		os.Exit(2)
	}
	o.url = flag.Arg(0)
	o.timeout = time.Duration(timeoutMs) * time.Millisecond
	return o
}

func output(html string, o options) {
	if o.text {
		if text, err := html2text.FromString(html, html2text.Options{PrettyTables: true}); err == nil {
			fmt.Print(text)
			return
		}
		// Fall through to raw HTML if rendering fails.
	}
	fmt.Print(html)
}
