# anubis-fetch

![Built with Nix](https://img.shields.io/badge/Built_With-Nix-5277C3?logo=nixos&logoColor=white)
[![CI](https://github.com/fzakaria/anubis-fetch/actions/workflows/ci.yml/badge.svg)](https://github.com/fzakaria/anubis-fetch/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

```console
$ nix run github:fzakaria/anubis-fetch -- https://lore.kernel.org/linux-mm/some-thread/T/
```

> A fast (Go) CLI to fetch URLs from behind **[Anubis](https://github.com/TecharoHQ/anubis)** proof-of-work walls and **Cloudflare** fingerprint checks — solving the challenge in-process, and only reaching for a real browser when it has to.

A growing number of sites — `lore.kernel.org`, GNOME, `kernel.org`, and many more — sit behind [Anubis](https://github.com/TecharoHQ/anubis), a bot-wall that makes your browser solve a SHA-256 proof-of-work before it serves any content. It's great at stopping scrapers. It's also great at stopping *you* when you just want to `curl` a mailing-list thread:

```console
$ curl -s https://lore.kernel.org/linux-mm/some-thread/T/ | grep -o '<title>.*</title>'
<title>Making sure you&#39;re not a bot!</title>   # 🤢
```

`anubis-fetch` gets you the actual page:

```console
$ anubis-fetch https://lore.kernel.org/linux-mm/some-thread/T/ | grep -o '<title>.*</title>'
<title>[PATCH 0/2] ...</title>                     # 🥳
```

## How it works

Bot-walls live at two different layers, and `anubis-fetch` handles both, cheapest step first:

1. **Reuse a saved cookie.** Anubis hands out a signed auth cookie (`techaro.lol-anubis-auth`) after you pass once; a browser isn't re-challenged on the next visit, and neither are we. Cookies are persisted per host (see [Cookie persistence](#cookie-persistence)).
2. **Solve the proof-of-work in-process.** The request goes out over [`req`](https://github.com/imroc/req) impersonating a real Chrome — same TLS/JA3 + HTTP/2 fingerprint — which also clears **Cloudflare's passive fingerprinting**. If the response is an Anubis challenge, we brute-force the nonce and submit it. No browser, ~0.6s.
3. **Fall back to a real browser.** For anything the fast path can't do, we drive headless Chromium via [`chromedp`](https://github.com/chromedp/chromedp), which runs whatever JavaScript the site serves.

> [!NOTE]
> The fallback fires for: Anubis' `preact` / `metarefresh` challenge methods, an unknown/future method, a difficulty too high to brute-force, a rejected solution, **or a Cloudflare _active_ JS challenge** (Managed Challenge / Turnstile / "I'm Under Attack"). Impersonation clears *passive* Cloudflare; only a browser clears the active JS tiers. So `anubis-fetch` degrades to "slower", never "broken".

### Why not just a browser for everything?

Because it's ~4× slower and drags a ~200&nbsp;MB Chromium into every fetch. The in-process solver is the common case; the browser is the safety net.

| Path | Wall time | Needs Chromium |
| --- | --- | --- |
| Solver (Anubis proof-of-work) | ~0.6s | no |
| Browser fallback | ~2.0s | yes |

## The Anubis proof-of-work, briefly

Anubis embeds a challenge as JSON in the page:

```json
{"rules":{"algorithm":"fast","difficulty":4},
 "challenge":{"id":"…","method":"fast","randomData":"6214bd88…","difficulty":4}}
```

Solving it means finding a nonce such that `hex(sha256(randomData ‖ nonce))` begins with `difficulty` zero characters (the nonce is its base-10 string). We then hand the answer back:

```
GET /.within.website/x/cmd/anubis/api/pass-challenge?id=…&response=<hash>&nonce=<n>&redir=<url>&elapsedTime=<ms>
```

…which sets the auth cookie and redirects to the real page. Difficulty 4 (the default on `lore`/`kernel.org`/GNOME) is ~65k hashes — sub-millisecond in Go.

## Installation

Run it directly:

```console
$ nix run github:fzakaria/anubis-fetch -- <url>
```

Install into your profile:

```console
$ nix profile install github:fzakaria/anubis-fetch
```

Or add it to your own flake:

```nix
{
  inputs.anubis-fetch.url = "github:fzakaria/anubis-fetch";
  # then, e.g. in home.packages / environment.systemPackages:
  #   inputs.anubis-fetch.packages.${system}.default
}
```

## Usage

```console
$ anubis-fetch [flags] URL
```

| Flag | Meaning |
| --- | --- |
| `--text` | render readable plain text instead of HTML |
| `--timeout MS` | per-step timeout in milliseconds (default `30000`) |
| `--ua STRING` | override the User-Agent |
| `--browser` | skip the solver; go straight to the headless browser |
| `--no-browser` | never use the browser; exit `3` if the solve can't apply |
| `--no-cache` | don't read or write the persistent cookie jar |

```console
# HTML to stdout
$ anubis-fetch https://lore.kernel.org/linux-mm/some-thread/T/

# readable plain text
$ anubis-fetch --text https://lore.kernel.org/linux-mm/some-thread/T/

# lean/fast only — useful in scripts; exits 3 if it would need a browser
$ anubis-fetch --no-browser https://example.com/ && echo "got it"
```

### Cookie persistence

After a successful fetch, the auth cookie is written to
`$XDG_CACHE_HOME/anubis-fetch/cookies/<host>.json` (falling back to
`~/.cache/…`). The next run for that host is let straight through — no
proof-of-work, no browser — exactly like a browser revisit. Cookies obtained
via the browser fallback are saved too, so a subsequent run can take the fast
HTTP path. Use `--no-cache` to disable, or just delete the file to force a
re-solve.

## Development

Everything is wired through the flake:

```console
$ nix develop          # dev shell: go, gopls, chromium, treefmt
$ go test ./...        # unit tests (hermetic — no network)
$ nix build            # build the wrapped binary
$ nix flake check      # build + tests + formatting
$ nix fmt              # format Go + Nix via treefmt (gofmt + alejandra)
```

The proof-of-work implementation is pinned against Anubis' own published test
vector (`sha256("hunter" + "0")`) so a drift in the hash construction fails a
unit test rather than silently returning garbage.

## Prior art & thanks

- [TecharoHQ/anubis](https://github.com/TecharoHQ/anubis) — the bot-wall this
  politely negotiates with.
- [imroc/req](https://github.com/imroc/req) — the HTTP client whose Chrome
  impersonation clears passive Cloudflare fingerprinting.
- [chromedp/chromedp](https://github.com/chromedp/chromedp) — drives the
  headless-browser fallback.
