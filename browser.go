package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// lookupChromium returns the Chromium path pinned by the Nix wrapper
// (CHROMIUM_BIN), or "" to let chromedp discover a browser on PATH.
func lookupChromium() string {
	return os.Getenv("CHROMIUM_BIN")
}

// fetchViaBrowser drives a real headless Chromium so that any JavaScript the
// site serves — Anubis' preact/metarefresh methods, a future PoW variant, or a
// Cloudflare active-JS challenge — runs to completion. Cookies obtained are
// persisted so the next run's HTTP path is let straight through.
func fetchViaBrowser(o options) (string, error) {
	ua := o.ua
	if ua == "" {
		ua = defaultUA
	}

	execOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	execOpts = append(execOpts,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.NoSandbox,
		chromedp.UserAgent(ua),
	)
	// Reuse the system Chromium wired in by the Nix wrapper; otherwise chromedp
	// discovers chrome/chromium on PATH.
	if bin := lookupChromium(); bin != "" {
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
		waitAnubisResolved(),
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

// waitAnubisResolved blocks until the Anubis interstitial's marker scripts are
// gone (challenge solved and the page reloaded to real content), then lets the
// content settle briefly. Evaluate errors are treated as "still navigating".
func waitAnubisResolved() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		const expr = `!document.getElementById('anubis_challenge') && !document.getElementById('anubis_version')`
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
