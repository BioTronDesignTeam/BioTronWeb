package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/BioTronDesignTeam/exo-gui/backend/internal/auth"
)

var noTestTimeout = fiber.TestConfig{Timeout: 0, FailOnTimeout: false}

func TestHealth(t *testing.T) {
	app := New("http://localhost:5173", []string{"127.0.0.1"}, nil, auth.NewClient("http://oauth-manager:8080"))
	resp, err := app.Test(httptest.NewRequest("GET", "/health", nil), noTestTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("health = %d, want 200", resp.StatusCode)
	}
}

// clientIP serves one request through a real listener — app.Test fakes the peer
// address, and the trusted-proxy check is entirely about who the peer is — and
// reports the client IP Fiber resolved.
func clientIP(t *testing.T, trustedProxies []string, proxyHeader string) string {
	t.Helper()
	app := New("http://localhost:5174", trustedProxies, nil, auth.NewClient("http://oauth-manager:8080"))
	app.Get("/who", func(c fiber.Ctx) error { return c.SendString(c.IP()) })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() {
		_ = app.ShutdownWithTimeout(2 * time.Second)
		<-served
	})

	request, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/who", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cf-Connecting-Ip", proxyHeader)

	var response *http.Response
	// The goroutine above may not have the listener serving yet.
	for attempt := 0; ; attempt++ {
		response, err = http.DefaultClient.Do(request)
		if err == nil {
			break
		}
		if attempt >= 20 {
			t.Fatalf("request: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// Per-IP rate limiting is only per-IP if Fiber resolves the real client address
// out of Cf-Connecting-Ip. If this regresses, every limiter silently collapses
// into a single bucket keyed on the edge proxy — the bug TrustProxy exists to
// fix, and the one Fiber v3's rename of EnableTrustedProxyCheck could have
// reintroduced quietly.
func TestTrustedProxyResolvesTheRealClientIP(t *testing.T) {
	got := clientIP(t, []string{"127.0.0.1", "::1", "172.16.0.0/12"}, "203.0.113.7")
	if got != "203.0.113.7" {
		t.Fatalf("c.IP() = %q, want 203.0.113.7", got)
	}
}

// The other half of the same guarantee: a peer that is not a configured proxy
// must not be able to name its own IP.
func TestUntrustedPeerCannotSpoofTheProxyHeader(t *testing.T) {
	got := clientIP(t, []string{"198.51.100.9"}, "203.0.113.7")
	if got != "127.0.0.1" {
		t.Fatalf("c.IP() = %q, want the real peer 127.0.0.1", got)
	}
}
