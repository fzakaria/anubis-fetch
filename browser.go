package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// chromeUATemplate is Chrome's reduced desktop User-Agent; only the major
// version is real, the rest is frozen at zero.
const chromeUATemplate = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36"

// chromiumVersionRe pulls the major version out of `chromium --version`,
// e.g. "Chromium 149.0.7827.200".
var chromiumVersionRe = regexp.MustCompile(`\b(\d+)\.\d+\.\d+\.\d+\b`)

// lookupChromium returns the Chromium path pinned by the Nix wrapper
// (CHROMIUM_BIN), or "" to let chromedp discover a browser on PATH.
func lookupChromium() string {
	return os.Getenv("CHROMIUM_BIN")
}

// chromeUserAgent builds a Chrome User-Agent whose major version matches the
// `--version` output of the browser about to be launched. Cloudflare compares
// the UA string against navigator.userAgentData, which always reports the
// real binary, and refuses to clear its challenge when the two disagree.
// Falls back to defaultUA when the version cannot be parsed.
func chromeUserAgent(versionOutput string) string {
	m := chromiumVersionRe.FindStringSubmatch(versionOutput)
	if m == nil {
		return defaultUA
	}
	return fmt.Sprintf(chromeUATemplate, m[1])
}

// browserUserAgent asks the Chromium at bin for its version and returns a
// matching UA, or defaultUA when bin is unknown or cannot be run.
func browserUserAgent(ctx context.Context, bin string) string {
	if bin == "" {
		return defaultUA
	}
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return defaultUA
	}
	return chromeUserAgent(string(out))
}

// fetchViaBrowser drives a real headless Chromium so that any JavaScript the
// site serves — Anubis' preact/metarefresh methods, a future PoW variant, or a
// Cloudflare active-JS challenge — runs to completion. Cookies obtained are
// persisted so the next run's HTTP path is let straight through.
func fetchViaBrowser(o options) (string, error) {
	// The UA must go in as a launch flag: a CDP override does not reach the
	// cross-origin iframe Cloudflare's Turnstile widget runs in.
	bin := lookupChromium()
	ua := o.ua
	if ua == "" {
		ua = browserUserAgent(context.Background(), bin)
	}

	execOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	execOpts = append(execOpts,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.NoSandbox,
		chromedp.UserAgent(ua),
		// Keep navigator.webdriver false; Cloudflare's managed challenge
		// never clears for a browser that reports itself as automated.
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	// Reuse the system Chromium wired in by the Nix wrapper; otherwise chromedp
	// discovers chrome/chromium on PATH.
	if bin != "" {
		execOpts = append(execOpts, chromedp.ExecPath(bin))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), execOpts...)
	defer cancelAlloc()
	// Dial the browser's websocket under the caller's timeout rather than
	// chromedp's ten-second default, which a loaded machine can blow past
	// while Chromium is still starting up.
	ctx, cancelCtx := chromedp.NewContext(allocCtx,
		chromedp.WithBrowserOption(chromedp.WithDialTimeout(o.timeout)),
	)
	defer cancelCtx()
	ctx, cancelTimeout := context.WithTimeout(ctx, o.timeout)
	defer cancelTimeout()

	var html string
	var cookies []*network.Cookie
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(o.url),
		waitChallengeResolved(),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			c, err := network.GetCookies().Do(ctx)
			if err == nil {
				cookies = c
			}
			return nil
		}),
	)

	if !o.noCache && len(cookies) > 0 {
		if u, perr := url.Parse(o.url); perr == nil {
			writeCookies(u.Host, toHTTPCookies(cookies))
		}
	}
	return html, err
}

// waitChallengeResolved blocks until both the Anubis interstitial's marker
// scripts and Cloudflare's challenge bootstrap (window._cf_chl_opt) are gone,
// meaning the challenge was solved and the page reloaded to real content, then
// lets the content settle briefly. Evaluate errors are treated as "still
// navigating".
func waitChallengeResolved() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		const expr = `!document.getElementById('anubis_challenge') && ` +
			`!document.getElementById('anubis_version') && ` +
			`!window._cf_chl_opt`
		for {
			var gone bool
			if err := chromedp.Evaluate(expr, &gone).Do(ctx); err == nil && gone {
				// Give the reloaded content a moment to finish rendering.
				return chromedp.Sleep(400 * time.Millisecond).Do(ctx)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	})
}

func toHTTPCookies(cookies []*network.Cookie) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		out = append(out, &http.Cookie{Name: c.Name, Value: c.Value})
	}
	return out
}
