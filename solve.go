package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

// The Anubis interstitial embeds its challenge as a JSON <script> block.
var challengeRe = regexp.MustCompile(
	`(?s)<script id="anubis_challenge" type="application/json">(.*?)</script>`)

// Asset metadata lives in separate JSON script blocks on the interstitial.
var challengeMetadataRe = regexp.MustCompile(
	`(?s)<script id="anubis_(version|base_prefix)" type="application/json">(.*?)</script>`)

type challenge struct {
	method     string
	difficulty int
	randomData string
	id         string
	version    string
	basePrefix string
}

// isAnubis reports whether html is an Anubis interstitial (challenge or deny)
// rather than real content.
func isAnubis(html string) bool {
	return strings.Contains(html, `id="anubis_challenge"`) ||
		strings.Contains(html, `id="anubis_version"`)
}

// parseChallenge extracts the challenge parameters, or nil if the page has no
// solvable challenge (e.g. an outright deny page, where the JSON is null).
func parseChallenge(html string) *challenge {
	m := challengeRe.FindStringSubmatch(html)
	if m == nil {
		return nil
	}
	raw := strings.TrimSpace(m[1])
	if raw == "" || raw == "null" {
		return nil
	}
	var data struct {
		Challenge struct {
			ID         string `json:"id"`
			Method     string `json:"method"`
			RandomData string `json:"randomData"`
			Difficulty int    `json:"difficulty"`
		} `json:"challenge"`
		Rules struct {
			Algorithm  string `json:"algorithm"`
			Difficulty int    `json:"difficulty"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil
	}
	c := &challenge{
		method:     firstNonEmpty(data.Challenge.Method, data.Rules.Algorithm),
		difficulty: firstNonZero(data.Challenge.Difficulty, data.Rules.Difficulty),
		randomData: data.Challenge.RandomData,
		id:         data.Challenge.ID,
	}
	if c.method == "" || c.difficulty <= 0 || c.randomData == "" || c.id == "" {
		return nil
	}
	// Read the deployment prefix and asset version without depending on script order.
	for _, match := range challengeMetadataRe.FindAllStringSubmatch(html, -1) {
		var value string
		if err := json.Unmarshal([]byte(match[2]), &value); err != nil {
			return nil
		}
		switch match[1] {
		case "version":
			c.version = value
		case "base_prefix":
			c.basePrefix = value
		}
	}
	return c
}

// solvePoW mirrors Anubis' verifier exactly: hex(sha256(randomData ‖ nonce)),
// where nonce is its base-10 string, must begin with `difficulty` '0' hex
// characters. Returns the winning nonce and its digest.
func solvePoW(randomData string, difficulty int) (int, string) {
	prefix := strings.Repeat("0", difficulty)
	rd := []byte(randomData)
	for nonce := 0; ; nonce++ {
		sum := sha256.Sum256(append(rd, strconv.Itoa(nonce)...))
		digest := hex.EncodeToString(sum[:])
		if strings.HasPrefix(digest, prefix) {
			return nonce, digest
		}
	}
}

// fetchViaHTTP tries the browserless path. It returns the page HTML, or
// escalate=true if the caller should fall back to a browser.
func fetchViaHTTP(o options) (html string, escalate bool) {
	u, err := url.Parse(o.url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "anubis-fetch: bad url: %v\n", err)
		return "", true
	}

	jar := newJar()
	if !o.noCache {
		loadCookies(jar, u)
	}
	client := req.C().ImpersonateChrome().SetTimeout(o.timeout).SetCookieJar(jar)
	if o.ua != "" {
		client.SetCommonHeader("User-Agent", o.ua)
	}

	resp, err := client.R().Get(o.url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "anubis-fetch: http error: %v; escalating\n", err)
		return "", true
	}
	html = resp.String()

	// Not walled, or a stored cookie let us straight through.
	if !isAnubis(html) {
		if !o.noCache {
			saveCookies(jar, u)
		}
		return html, false
	}

	c := parseChallenge(html)
	switch {
	case c == nil:
		fmt.Fprintln(os.Stderr, "anubis-fetch: unparseable/deny challenge; escalating to browser")
		return "", true
	case !legacyMethods[c.method] && !isWASMMethod(c.method):
		fmt.Fprintf(os.Stderr, "anubis-fetch: challenge method %q not solvable in-process; escalating\n", c.method)
		return "", true
	case legacyMethods[c.method] && c.difficulty > maxDifficulty:
		fmt.Fprintf(os.Stderr, "anubis-fetch: difficulty %d too high for in-process solve; escalating\n", c.difficulty)
		return "", true
	}

	// Assets and submissions belong to the final challenge URL after redirects.
	origin := resp.Response.Request.URL
	start := time.Now()
	var nonce uint64
	var response string
	if isWASMMethod(c.method) {
		ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
		defer cancel()
		code, err := fetchWASM(ctx, client, origin, c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "anubis-fetch: %v; escalating\n", err)
			return "", true
		}
		n, digest, err := solveWASM(ctx, code, c.randomData, c.difficulty)
		if err != nil {
			fmt.Fprintf(os.Stderr, "anubis-fetch: %v; escalating\n", err)
			return "", true
		}
		nonce, response = uint64(n), digest
	} else {
		n, digest := solvePoW(c.randomData, c.difficulty)
		nonce, response = uint64(n), digest
	}
	elapsed := time.Since(start).Milliseconds()
	if elapsed == 0 {
		elapsed = 1
	}

	pass := challengeEndpoint(origin, c, passChallengePath)
	q := url.Values{}
	q.Set("id", c.id)
	q.Set("response", response)
	q.Set("nonce", strconv.FormatUint(nonce, 10))
	q.Set("redir", o.url)
	q.Set("elapsedTime", strconv.FormatInt(elapsed, 10))
	pass.RawQuery = q.Encode()

	resp2, err := client.R().Get(pass.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "anubis-fetch: pass-challenge error: %v; escalating\n", err)
		return "", true
	}
	html2 := resp2.String()
	if isAnubis(html2) {
		fmt.Fprintln(os.Stderr, "anubis-fetch: solution rejected; escalating to browser")
		return "", true
	}
	if !o.noCache {
		saveCookies(jar, u) // now holds the Anubis auth cookie
	}
	return html2, false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstNonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}
