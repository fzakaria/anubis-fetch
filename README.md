# anubis-fetch

![Built with Nix](https://img.shields.io/badge/Built_With-Nix-5277C3?logo=nixos&logoColor=white)
[![CI](https://github.com/fzakaria/anubis-fetch/actions/workflows/ci.yml/badge.svg)](https://github.com/fzakaria/anubis-fetch/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

```console
$ nix run github:fzakaria/anubis-fetch -- --text https://lore.kernel.org/
```

A Go CLI for fetching URLs behind [Anubis](https://github.com/TecharoHQ/anubis)
proof-of-work challenges. It solves legacy SHA-256 and WASM challenges
in-process, with headless Chromium as a fallback.

Sites such as `lore.kernel.org` and `kernel.org` use Anubis to require proof
of work before serving a page. A plain HTTP client can receive the challenge
page instead of the requested content. `anubis-fetch` solves the challenge,
submits the answer, and writes the resulting page to stdout.

## How it works

1. Reuse a saved cookie. Anubis issues an auth cookie after a successful
   challenge. The tool saves cookies per host and sends them on later requests.
2. Solve the proof of work in-process. The HTTP client uses
   [`req`](https://github.com/imroc/req) to impersonate Chrome's TLS and HTTP/2
   fingerprint. This can help with passive bot checks, including Cloudflare's.
   For Anubis' `fast` and `slow` methods, the tool computes a nonce and submits
   the answer directly. For the WASM methods `sha256`, `argon2id`, and `hashx`,
   it downloads the site's module and runs it in Go with
   [`wazero`](https://github.com/tetratelabs/wazero).
3. Fall back to a browser. Headless Chromium, driven by
   [`chromedp`](https://github.com/chromedp/chromedp), runs the challenge code
   served by the site.

Browser fallback handles Anubis' `preact` and `metarefresh` methods.
The tool also falls back when legacy SHA-256 difficulty exceeds its limit,
WASM execution fails or times out, or Anubis rejects a solution.
Use `--browser` to start with Chromium, or `--no-browser` to require the
HTTP path. All three WASM methods work with `--no-browser`.

### Why not use a browser for everything?

The native solver avoids starting a browser process for each fetch. Chromium
is still included in the Nix package for challenges that need the fallback.

Approximate fetch times for the original SHA-256 path:

| Path | Wall time | Needs Chromium |
| --- | --- | --- |
| In-process SHA-256 solver | ~0.6s | No |
| Browser fallback | ~2.0s | Yes |

WASM fetch times against a local Anubis server at difficulty 2:

| WASM method | Median wall time | Needs Chromium |
| --- | --- | --- |
| SHA-256 | ~0.025s | No |
| Argon2id | ~0.55s-2.33s | No |
| HashX | ~0.041s | No |

## The Anubis proof-of-work, briefly

For the `fast` and `slow` methods, Anubis embeds the challenge in the page:

```json
{"rules":{"algorithm":"fast","difficulty":4},
 "challenge":{"id":"...","method":"fast","randomData":"6214bd88...","difficulty":4}}
```

The solver looks for a nonce such that `hex(sha256(randomData || nonce))`
begins with `difficulty` zero characters. The nonce is encoded as a decimal
string. The answer is submitted to Anubis:

```text
GET /.within.website/x/cmd/anubis/api/pass-challenge?id=...&response=<hash>&nonce=<n>&redir=<url>&elapsedTime=<ms>
```

A successful response sets an auth cookie and redirects to the requested
page. Difficulty 4 requires about 65,536 hashes on average.

WASM challenges use the same submission endpoint, but the downloaded module
defines the hash and difficulty rules. Their difficulty numbers are not
directly comparable to the legacy SHA-256 method.

## Installation

Run directly:

```console
$ nix run github:fzakaria/anubis-fetch -- <url>
```

Or add the input to your flake:

```nix
{
  inputs.anubis-fetch.url = "github:fzakaria/anubis-fetch";
}
```

Add `inputs.anubis-fetch.packages.${system}.default` to
`home.packages` or `environment.systemPackages`.

## Usage

```text
anubis-fetch [flags] URL
```

| Flag | Effect |
| --- | --- |
| `--text` | Convert HTML to plain text |
| `--timeout MS` | Set the per-step timeout in milliseconds; default `30000` |
| `--ua STRING` | Set the User-Agent |
| `--browser` | Use Chromium directly |
| `--no-browser` | Exit `3` if the HTTP solver needs browser fallback |
| `--no-cache` | Disable reading and writing saved cookies |
| `--help`, `-h` | Print usage and exit |

```bash
# HTML to stdout
anubis-fetch https://lore.kernel.org/

# Plain text
anubis-fetch --text https://lore.kernel.org/

# Require the HTTP path; check for exit status 3
anubis-fetch --no-browser https://lore.kernel.org/
```

### Cookie persistence

Cookies are saved per host in
`$XDG_CACHE_HOME/anubis-fetch/cookies/<host>.json`, or
`~/.cache/anubis-fetch/cookies/<host>.json` when `XDG_CACHE_HOME` is unset.
A valid auth cookie can let subsequent requests skip the challenge.
Cookies obtained through Chromium are saved too.

Use `--no-cache` to fetch without saved cookies, or delete the host's file
to discard them. An expired or rejected cookie requires a new challenge.

## Development

```console
$ nix develop          # Go, gopls, Chromium, treefmt
$ go test ./...        # Go unit tests
$ nix build            # Packaged CLI
$ nix flake check      # Builds, tests, and formatting
$ nix fmt              # Format Go and Nix
```

## Dependencies

- [TecharoHQ/anubis](https://github.com/TecharoHQ/anubis): the challenge server.
- [imroc/req](https://github.com/imroc/req): the HTTP client with Chrome impersonation.
- [chromedp/chromedp](https://github.com/chromedp/chromedp): the Chromium driver.
- [tetratelabs/wazero](https://github.com/tetratelabs/wazero): the Go WebAssembly runtime.
