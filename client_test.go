package exante

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		cred    Credentials
		opts    []Option
	}{
		{"empty URL", "", testCred, nil},
		{"blank URL", "   ", testCred, nil},
		{"unparsable URL", "http://[::1", testCred, nil},
		{"no scheme", "api.exante.eu", testCred, nil},
		{"wrong scheme", "ftp://api.exante.eu", testCred, nil},
		{"no host", "https://", testCred, nil},
		{"query in URL", "https://api.exante.eu?x=1", testCred, nil},
		{"user info in URL", "https://user:pw@api.exante.eu", testCred, nil},
		{"plain http", "http://api.exante.eu", testCred, nil},
		{"plain http on a lookalike of localhost", "http://localhost.evil.test", testCred, nil},
		{"empty client id", "https://api.exante.eu", Credentials{ApplicationID: "a", SharedKey: "k"}, nil},
		{"empty application id", "https://api.exante.eu", Credentials{ClientID: "c", SharedKey: "k"}, nil},
		{"empty shared key", "https://api.exante.eu", Credentials{ClientID: "c", ApplicationID: "a"}, nil},
		{"nil HTTP client", "https://api.exante.eu", testCred, []Option{WithHTTPClient(nil)}},
		{"zero timeout", "https://api.exante.eu", testCred, []Option{WithTimeout(0)}},
		{"blank user agent", "https://api.exante.eu", testCred, []Option{WithUserAgent(" ")}},
		{"tiny token TTL", "https://api.exante.eu", testCred, []Option{WithTokenTTL(time.Millisecond)}},
		{"empty audience", "https://api.exante.eu", testCred, []Option{WithAudience()}},
		{"zero body limit", "https://api.exante.eu", testCred, []Option{WithMaxBodySize(0)}},
		{"negative idle timeout", "https://api.exante.eu", testCred, []Option{WithStreamIdleTimeout(-1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.baseURL, tt.cred, tt.opts...)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("New() = %v, %v; want ErrInvalidConfig", c, err)
			}
			if c != nil {
				t.Errorf("client = %v, want nil on error", c)
			}
		})
	}
}

func TestNewAcceptsPlainHTTPOnLoopbackOnly(t *testing.T) {
	for _, base := range []string{"http://127.0.0.1:8080", "http://localhost", "http://[::1]:9000", "https://api.exante.eu"} {
		if _, err := New(base, testCred); err != nil {
			t.Errorf("New(%q) = %v, want it accepted", base, err)
		}
	}
}

func TestNewErrorDoesNotEchoCredentialsInTheURL(t *testing.T) {
	_, err := New("https://user:hunter2@api.exante.eu:bad", testCred)

	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaks the URL password: %v", err)
	}
}

func TestNewAcceptsNilOptionAndBasePath(t *testing.T) {
	rec := reply(200, `[]`)
	srvClient := newTestClient(t, rec).hc
	c, err := New("https://api.test/proxy/exante/", testCred, nil, WithHTTPClient(srvClient))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Groups(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := rec.only(t).URI, "/proxy/exante/md/3.0/groups"; got != want {
		t.Errorf("URI = %q, want %q", got, want)
	}
}

func TestUserAgent(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want func(string) bool
	}{
		{"default", nil, func(ua string) bool { return strings.HasPrefix(ua, "exante-go/") && ua != "exante-go/" }},
		{"custom", []Option{WithUserAgent("my-batch/1.2")}, func(ua string) bool { return ua == "my-batch/1.2" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := reply(200, `[]`)
			c := newTestClient(t, rec, tt.opts...)
			if _, err := c.Groups(context.Background()); err != nil {
				t.Fatal(err)
			}
			if ua := rec.only(t).Header.Get("User-Agent"); !tt.want(ua) {
				t.Errorf("User-Agent = %q", ua)
			}
		})
	}
}

func TestRequestHeaders(t *testing.T) {
	rec := &recorder{status: 200, body: fixture(t, "orders.json")}
	c := newTestClient(t, rec)
	if _, err := c.PlaceOrder(context.Background(), &PlaceParams{
		SymbolID: "AAPL.NASDAQ", Side: Buy, Quantity: "6", OrderType: OrderMarket, Duration: DurationDay,
	}); err != nil {
		t.Fatal(err)
	}
	h := rec.only(t).Header
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := h.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if got := h.Get("Accept-Encoding"); got != "" && got != "gzip" {
		t.Errorf("Accept-Encoding = %q, want only what the transport adds", got)
	}
}

func TestDefaultHTTPClientIsPrivate(t *testing.T) {
	a := newClient(t)
	b := newClient(t)
	if a.hc == http.DefaultClient || a.hc.Transport == http.DefaultTransport || a.hc.Transport == b.hc.Transport {
		t.Error("default clients share an HTTP client or transport with the process defaults")
	}
}

func TestDefaultTransportFallbackWhenTheProcessDefaultIsNotAnHTTPTransport(t *testing.T) {
	saved := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = saved })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") })

	c := newClient(t)
	tr, ok := c.hc.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Errorf("transport = %#v, want an *http.Transport honouring the proxy environment", c.hc.Transport)
	}
}
