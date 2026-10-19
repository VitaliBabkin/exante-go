package exante

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
)

// Defaults of the client settings.
const (
	defaultTimeout     = 60 * time.Second
	defaultTokenTTL    = time.Hour
	defaultMaxBodySize = 64 << 20
	modulePath         = "github.com/VitaliBabkin/exante-go"
)

// Credentials identify the API application the client acts for.
type Credentials struct {
	// ClientID is the JWT issuer (claim iss).
	ClientID string
	// ApplicationID is the JWT subject (claim sub).
	ApplicationID string
	// SharedKey is the HS256 secret the JWT is signed with.
	SharedKey string
}

// String implements fmt.Stringer. The shared key never appears in the output, and the identifiers are shortened, so a
// formatted Credentials is safe to log.
func (c Credentials) String() string {
	return fmt.Sprintf("exante.Credentials{ClientID: %s, ApplicationID: %s, SharedKey: ***}",
		shorten(c.ClientID), shorten(c.ApplicationID))
}

// GoString implements fmt.GoStringer for the %#v verb, with the same redaction as [Credentials.String].
func (c Credentials) GoString() string { return c.String() }

// LogValue implements slog.LogValuer with the same redaction as [Credentials.String].
func (c Credentials) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("ClientID", shorten(c.ClientID)),
		slog.String("ApplicationID", shorten(c.ApplicationID)),
		slog.String("SharedKey", "***"),
	)
}

// shorten keeps the first four characters of an identifier, enough to tell credentials apart in a log.
func shorten(id string) string {
	r := []rune(id)
	if len(r) <= 4 {
		return "***"
	}
	return string(r[:4]) + "..."
}

// Client talks to the Exante HTTP API. It is safe for concurrent use; create it once and share it. Its token is
// private to the client: two clients never share one.
type Client struct {
	base       *url.URL
	hc         *http.Client
	tokens     *tokenSource
	userAgent  string
	timeout    time.Duration
	maxBody    int64
	streamIdle time.Duration
}

// New returns a client for the API served at baseURL (for example https://api-live.exante.eu). It validates the URL,
// the credentials and the options and never panics.
//
// The client signs its own JWT per [Credentials] and sends a User-Agent header on every request, which the API edge
// requires. API versions are chosen per method; callers never pass one.
func New(baseURL string, cred Credentials, opts ...Option) (*Client, error) {
	base, err := parseBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if cred.ClientID == "" || cred.ApplicationID == "" || cred.SharedKey == "" {
		return nil, fmt.Errorf("%w: ClientID, ApplicationID and SharedKey are all required", ErrInvalidConfig)
	}
	c := &Client{
		base:      base,
		hc:        withoutRedirects(&http.Client{Transport: cloneDefaultTransport()}),
		tokens:    &tokenSource{cred: cred, audience: defaultAudience, ttl: defaultTokenTTL},
		userAgent: defaultUserAgent(),
		timeout:   defaultTimeout,
		maxBody:   defaultMaxBodySize,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// withoutRedirects returns a shallow copy of hc that hands a 3xx answer back instead of following it. A redirect would
// re-send a POST as a GET and report success for an order that was never placed.
func withoutRedirects(hc *http.Client) *http.Client {
	cp := *hc
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &cp
}

// CloseIdleConnections closes idle keep-alive connections of the underlying HTTP client. Call it on shutdown.
func (c *Client) CloseIdleConnections() {
	c.hc.CloseIdleConnections()
}

func parseBaseURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%w: empty base URL", ErrInvalidConfig)
	}
	u, err := url.Parse(raw)
	if err != nil {
		// The raw URL may carry a password; report only what the parser objected to.
		if uerr, ok := errors.AsType[*url.Error](err); ok {
			err = uerr.Err
		}
		return nil, fmt.Errorf("%w: base URL: %w", ErrInvalidConfig, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: base URL has no host", ErrInvalidConfig)
	}
	if secure := u.Scheme == "https" || u.Scheme == "http" && isLoopback(u.Hostname()); !secure {
		return nil, fmt.Errorf("%w: base URL must use https (http is accepted for loopback hosts only)", ErrInvalidConfig)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("%w: base URL must not carry user info, a query or a fragment", ErrInvalidConfig)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u, nil
}

// isLoopback reports whether host is 127.0.0.1, ::1 or localhost, the hosts where the bearer token cannot leave the
// machine in clear text.
func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func cloneDefaultTransport() *http.Transport {
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		return t.Clone()
	}
	return &http.Transport{Proxy: http.ProxyFromEnvironment}
}

// defaultUserAgent names this module and its version as recorded in the build information.
func defaultUserAgent() string {
	version := "devel"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, m := range append([]*debug.Module{&info.Main}, info.Deps...) {
			if m.Path == modulePath && m.Version != "" && m.Version != "(devel)" {
				version = m.Version
			}
		}
	}
	return "exante-go/" + version
}
