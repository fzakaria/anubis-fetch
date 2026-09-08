// Command local-anubis runs a pinned Anubis server and a fixed loopback backend.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type algorithm string

const (
	fast              algorithm = "fast"
	sha256            algorithm = "sha256"
	argon2id          algorithm = "argon2id"
	hashx             algorithm = "hashx"
	loopback                    = "127.0.0.1"
	defaultPort                 = 8923
	maxPort                     = 65535
	defaultDifficulty           = 2
	privateFileMode             = 0600
	shutdownTimeout             = 5 * time.Second
	backendContent              = "<html><body>anubis-fetch local backend</body></html>\n"
	policyTemplate              = `bots:
  - name: local-test
    user_agent_regex: ".*"
    action: CHALLENGE
    challenge:
      algorithm: %s
      difficulty: %d
`
)

func main() {
	// Exit after run returns so deferred server and temporary-file cleanup completes.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "local-anubis:", err)
		os.Exit(1)
	}
}

func run() error {
	// Accept the algorithm before flags to match nix run .#local-anubis -- argon2id.
	method := argon2id
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		method = algorithm(args[0])
		args = args[1:]
	}
	switch method {
	case fast, sha256, argon2id, hashx:
	default:
		return fmt.Errorf("unknown algorithm %q: use fast, sha256, argon2id, or hashx", method)
	}

	flags := flag.NewFlagSet("local-anubis", flag.ContinueOnError)
	port := flags.Int("port", defaultPort, "loopback port for Anubis")
	difficulty := flags.Int("difficulty", defaultDifficulty, "challenge difficulty")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *port < 1 || *port > maxPort || *difficulty < 1 {
		return fmt.Errorf("port must be between 1 and %d and difficulty must be positive", maxPort)
	}
	binary := os.Getenv("ANUBIS_BIN")
	if binary == "" {
		return fmt.Errorf("ANUBIS_BIN is unset; run with nix run .#local-anubis")
	}

	// Start a fixed backend on a kernel-assigned loopback port before the proxy.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, backendContent)
	}))
	defer backend.Close()
	dir, err := os.MkdirTemp("", "local-anubis-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Force a challenge for loopback clients instead of using upstream allow rules.
	policy := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policy, []byte(fmt.Sprintf(policyTemplate, method, *difficulty)), privateFileMode); err != nil {
		return err
	}

	// Forward shutdown signals and bound the wait for the Anubis child to exit.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	bind := fmt.Sprintf("%s:%d", loopback, *port)
	server := exec.CommandContext(ctx, binary,
		"--bind", bind,
		"--metrics-bind", loopback+":0",
		"--target", backend.URL,
		"--policy-fname", policy,
		"--use-remote-address", "--cookie-secure=false", "--cookie-partitioned=false",
	)
	server.Stdout, server.Stderr = os.Stdout, os.Stderr
	server.Cancel = func() error { return server.Process.Signal(syscall.SIGTERM) }
	server.WaitDelay = shutdownTimeout
	fmt.Fprintf(os.Stderr, "Serving %s difficulty %d at http://%s/\n", method, *difficulty, bind)
	err = server.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}
