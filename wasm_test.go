package main

import (
	"bytes"
	"context"
	"github.com/imroc/req/v3"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWASMCancellation(t *testing.T) {
	// Run a module with an infinite work loop and require the deadline to stop it.
	module, err := os.ReadFile("testdata/wasm/loop.wasm")
	if err != nil {
		t.Fatal(err)
	}
	const timeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	_, _, err = solveWASM(ctx, module, "aa", 1)
	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("expected deadline failure, got %v (context: %v)", err, ctx.Err())
	}
}

func TestWASMRejectsInvalidInputs(t *testing.T) {
	// Reject bad modules and input lengths before entering the work loop.
	module, err := os.ReadFile("testdata/wasm/loop.wasm")
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"", "zz", "a", strings.Repeat("aa", maxWASMDataBytes+1)} {
		if _, _, err := solveWASM(t.Context(), module, data, 1); err == nil {
			t.Errorf("accepted invalid input of length %d", len(data))
		}
	}
	if _, _, err := solveWASM(t.Context(), []byte("not wasm"), "aa", 1); err == nil {
		t.Error("accepted invalid module")
	}
}

func TestWASMRejectsInvalidPointer(t *testing.T) {
	// An exported input pointer outside linear memory must return an error.
	module, err := os.ReadFile("testdata/wasm/bad-pointer.wasm")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := solveWASM(t.Context(), module, "aa", 1); err == nil {
		t.Fatal("accepted out-of-bounds input pointer")
	}
}

func TestFetchWASM(t *testing.T) {
	// Fetch an asset under the deployment prefix with its version and existing cookie.
	const prefix = "/protected"
	const version = "test version"
	const cookieName = "challenge-cookie"
	const cookieValue = "test-value"
	const userAgent = "anubis-fetch-test"
	module, err := os.ReadFile("testdata/wasm/loop.wasm")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != prefix+wasmModulePath+"sha256.wasm" || r.URL.Query().Get("cacheBuster") != version {
			t.Errorf("wrong asset URL: %s", r.URL)
		}
		cookie, err := r.Cookie(cookieName)
		if err != nil || cookie.Value != cookieValue {
			t.Errorf("challenge cookie missing: %v", err)
		}
		if r.UserAgent() != userAgent {
			t.Errorf("wrong User-Agent: %s", r.UserAgent())
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(module)
	}))
	defer server.Close()
	origin, err := url.Parse(server.URL + "/page?original=query")
	if err != nil {
		t.Fatal(err)
	}
	jar := newJar()
	jar.SetCookies(origin, []*http.Cookie{{Name: cookieName, Value: cookieValue}})
	client := req.C().SetCookieJar(jar).SetUserAgent(userAgent)
	got, err := fetchWASM(t.Context(), client, origin, &challenge{method: wasmSHA256, basePrefix: prefix, version: version})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, module) {
		t.Fatal("downloaded module differs from response body")
	}
}

func TestFetchWASMRejectsBadResponses(t *testing.T) {
	// Reject HTTP failures and oversized streamed bodies without compiling a module.
	for _, status := range []int{http.StatusNotFound, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			// A successful HTTP status still requires the response to fit the size limit.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status == http.StatusOK {
					w.Write(make([]byte, maxWASMModuleBytes+1))
				}
			}))
			defer server.Close()
			origin, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fetchWASM(t.Context(), req.C(), origin, &challenge{method: wasmSHA256}); err == nil {
				t.Fatal("accepted invalid WASM response")
			}
		})
	}
}
