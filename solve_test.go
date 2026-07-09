package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

// knownVector is Anubis' own test value: hex(sha256("hunter" + "0")).
const knownVector = "2652bdba8fb4d2ab39ef28d8534d7694c557a4ae146c1e9237bd8d950280500e"

func TestSolvePoWKnownVector(t *testing.T) {
	// Difficulty 0 accepts the first nonce, so this pins the exact hash
	// construction (randomData ‖ decimal-nonce, hex-encoded) against Anubis.
	nonce, digest := solvePoW("hunter", 0)
	if nonce != 0 {
		t.Fatalf("difficulty 0: got nonce %d, want 0", nonce)
	}
	if digest != knownVector {
		t.Fatalf("digest = %s, want %s (hash construction differs from Anubis)", digest, knownVector)
	}
}

func TestSolvePoWMeetsDifficulty(t *testing.T) {
	for _, difficulty := range []int{1, 2, 3} {
		nonce, digest := solvePoW("some-random-data", difficulty)
		prefix := strings.Repeat("0", difficulty)
		if !strings.HasPrefix(digest, prefix) {
			t.Errorf("difficulty %d: digest %q lacks prefix %q", difficulty, digest, prefix)
		}
		// The returned digest must actually be the hash of randomData‖nonce.
		want := sha256.Sum256([]byte("some-random-data" + strconv.Itoa(nonce)))
		if digest != hex.EncodeToString(want[:]) {
			t.Errorf("difficulty %d: digest does not match sha256 of randomData+nonce", difficulty)
		}
	}
}

const sampleChallenge = `<html><head>
<script id="anubis_version" type="application/json">"1.25.0"</script>
<script id="anubis_challenge" type="application/json">{"rules":{"algorithm":"fast","difficulty":4},"challenge":{"id":"abc-123","method":"fast","randomData":"deadbeef","difficulty":4}}</script>
</head><body>Making sure you're not a bot!</body></html>`

func TestParseChallenge(t *testing.T) {
	c := parseChallenge(sampleChallenge)
	if c == nil {
		t.Fatal("parseChallenge returned nil for a valid challenge")
	}
	if c.method != "fast" || c.difficulty != 4 || c.randomData != "deadbeef" || c.id != "abc-123" {
		t.Fatalf("parsed %+v, want method=fast difficulty=4 randomData=deadbeef id=abc-123", c)
	}
}

func TestParseChallengeFallsBackToRules(t *testing.T) {
	// Older/partial payloads may carry method+difficulty only on `rules`.
	html := `<script id="anubis_challenge" type="application/json">{"rules":{"algorithm":"fast","difficulty":3},"challenge":{"id":"x","randomData":"ab"}}</script>`
	c := parseChallenge(html)
	if c == nil || c.method != "fast" || c.difficulty != 3 {
		t.Fatalf("parsed %+v, want method=fast difficulty=3 from rules block", c)
	}
}

func TestParseChallengeDenyOrMissing(t *testing.T) {
	cases := map[string]string{
		"deny (null challenge)": `<script id="anubis_challenge" type="application/json">null</script>`,
		"no challenge block":    `<html><body>hello</body></html>`,
		"missing fields":        `<script id="anubis_challenge" type="application/json">{"challenge":{"method":"fast"}}</script>`,
	}
	for name, html := range cases {
		if c := parseChallenge(html); c != nil {
			t.Errorf("%s: expected nil, got %+v", name, c)
		}
	}
}

func TestIsAnubis(t *testing.T) {
	if !isAnubis(sampleChallenge) {
		t.Error("sample challenge not detected as Anubis")
	}
	if isAnubis(`<html><head><title>Real Page</title></head><body>content</body></html>`) {
		t.Error("real page misdetected as Anubis")
	}
}
