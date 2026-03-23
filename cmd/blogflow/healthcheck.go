package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	healthcheckTimeout = 2 * time.Second
	healthcheckPath    = "/healthz"
	defaultPort        = 8080
)

// runHealthcheck performs an HTTP GET against the local /healthz endpoint
// and exits 0 on success (HTTP 200) or 1 on any failure. This allows
// distroless containers (no curl/wget) to use the binary itself as a
// Docker HEALTHCHECK or Kubernetes liveness probe command.
func runHealthcheck(args []string) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "port to probe")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}

	url := fmt.Sprintf("http://localhost:%d%s", *port, healthcheckPath)

	client := &http.Client{Timeout: healthcheckTimeout}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: unhealthy (status %d)\n", resp.StatusCode)
		return 1
	}
	return 0
}
