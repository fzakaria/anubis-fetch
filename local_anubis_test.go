package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	localBackendMarker    = "anubis-fetch local backend"
	localStartupTimeout   = 30 * time.Second
	localPollInterval     = 100 * time.Millisecond
	localHTTPTimeout      = 2 * time.Second
	localShutdownTimeout  = 5 * time.Second
	localFetchTimeout     = 60 * time.Second
	wasmAssetPrefix       = "/.within.website/x/cmd/anubis/static/wasm"
	wasmMagic             = "\x00asm"
	wasmTestBitDifficulty = 8
)

func TestLocalAnubis(t *testing.T) {
	// Exercise the packaged server and CLI on loopback for every supported challenge.
	serverBinary := os.Getenv("ANUBIS_TEST_SERVER")
	fetchBinary := os.Getenv("ANUBIS_FETCH")
	if serverBinary == "" || fetchBinary == "" {
		t.Skip("run through nix flake check to supply the local server and CLI")
	}

	for _, method := range []string{"fast", "sha256", "argon2id", "hashx"} {
		t.Run(method, func(t *testing.T) {
			// Inspect the challenge and assets before solving and reusing the auth cookie.
			var serverArgs []string
			if method == wasmSHA256 {
				// A bit difficulty above the legacy cap must still use the WASM runner.
				serverArgs = []string{"--difficulty", fmt.Sprint(wasmTestBitDifficulty)}
			}
			url := startLocalAnubis(t, serverBinary, method, serverArgs...)
			page := readLocalURL(t, url)
			challenge := parseChallenge(page)
			if challenge == nil || challenge.method != method {
				t.Fatalf("expected %s challenge, got %s", method, page)
			}

			// Check that both feature levels contain WASM rather than an HTML error page.
			if method != "fast" {
				for _, feature := range []string{"baseline", "simd128"} {
					asset := readLocalURL(t, url+wasmAssetPrefix+"/"+feature+"/"+method+".wasm")
					if !strings.HasPrefix(asset, wasmMagic) {
						t.Fatalf("%s/%s is not a WASM module", feature, method)
					}
				}
			}

			// Each method gets a fresh cache so the first fetch must solve the challenge.
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			assertLocalFetch(t, fetchBinary, 0, "--no-browser", url)

			// Force another solve so cached cookies cannot hide a missing WASM runner.
			assertLocalFetch(t, fetchBinary, 0, "--no-browser", "--no-cache", url)

			// Retain the browser path as a separately exercised fallback.
			if method != "fast" {
				assertLocalFetch(t, fetchBinary, 0, "--browser", "--no-cache", url)
			}

			// A saved auth cookie must make the next browserless fetch succeed.
			assertLocalFetch(t, fetchBinary, 0, "--no-browser", url)
		})
	}
}

func startLocalAnubis(t *testing.T, binary, method string, extraArgs ...string) string {
	// Allocate a loopback port and keep server logs for any failing subtest.
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	logPath := filepath.Join(t.TempDir(), "server.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	args := append([]string{method, "--port", fmt.Sprint(port)}, extraArgs...)
	server := exec.CommandContext(ctx, binary, args...)
	server.Stdout, server.Stderr = log, log
	server.Cancel = func() error { return server.Process.Signal(syscall.SIGTERM) }
	server.WaitDelay = localShutdownTimeout
	if err := server.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		server.Wait()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Logf("server log:\n%s", data)
		}
	})

	// Wait for the proxy listener with a deadline instead of a fixed startup sleep.
	client := &http.Client{Timeout: localHTTPTimeout}
	deadline := time.Now().Add(localStartupTimeout)
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			response.Body.Close()
			return url
		}
		time.Sleep(localPollInterval)
	}
	t.Fatal("local Anubis did not start")
	return ""
}

func readLocalURL(t *testing.T, url string) string {
	// Read challenge pages and assets with a bounded request timeout.
	t.Helper()
	client := &http.Client{Timeout: localHTTPTimeout}
	response, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertLocalFetch(t *testing.T, binary string, wantExit int, args ...string) {
	// Require the expected exit status and the backend marker for successful fetches.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), localFetchTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	gotExit := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatal(err)
		}
		gotExit = exitError.ExitCode()
	}
	if gotExit != wantExit {
		t.Fatalf("fetch %v exited %d, want %d:\n%s", args, gotExit, wantExit, output)
	}
	if wantExit == 0 && !strings.Contains(string(output), localBackendMarker) {
		t.Fatalf("fetch returned no backend content:\n%s", output)
	}
}
