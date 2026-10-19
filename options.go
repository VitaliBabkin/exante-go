package exante

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Option configures a [Client] in [New].
type Option func(*Client) error

// WithHTTPClient replaces the default HTTP client, for custom transports, proxies or TLS settings. Its Timeout must
// be zero for streams to live longer than that timeout; use [WithTimeout] to bound a request instead.
//
// The Client never follows redirects: a 3xx answer is returned as an *[APIError], because a redirect would re-send a
// POST as a GET. It uses a shallow copy of hc with CheckRedirect replaced; hc itself is not modified.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return fmt.Errorf("%w: nil HTTP client", ErrInvalidConfig)
		}
		c.hc = withoutRedirects(hc)
		return nil
	}
}

// WithTimeout bounds one request, from sending it to reading the whole response body (default 60 s). For a stream it
// bounds the wait for the response headers only.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d <= 0 {
			return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfig)
		}
		c.timeout = d
		return nil
	}
}

// WithUserAgent sets the User-Agent header (default exante-go/<module version>). The API edge refuses requests that
// carry the default Go user agent, so the value must be a descriptive, non-empty product token.
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		if strings.TrimSpace(ua) == "" {
			return fmt.Errorf("%w: empty User-Agent", ErrInvalidConfig)
		}
		c.userAgent = ua
		return nil
	}
}

// WithTokenTTL sets the lifetime of each signed JWT (default one hour, minimum one second). The client signs a new
// token when a third of the lifetime is left.
func WithTokenTTL(d time.Duration) Option {
	return func(c *Client) error {
		if d < time.Second {
			return fmt.Errorf("%w: token TTL must be at least one second", ErrInvalidConfig)
		}
		c.tokens.ttl = d
		return nil
	}
}

// WithAudience replaces the JWT audience (claim aud), which defaults to every endpoint group of the API. Use it when
// the application is entitled to a narrower set.
func WithAudience(aud ...string) Option {
	return func(c *Client) error {
		if len(aud) == 0 {
			return fmt.Errorf("%w: empty audience", ErrInvalidConfig)
		}
		c.tokens.audience = append([]string(nil), aud...)
		return nil
	}
}

// WithMaxBodySize caps the size of a response body, and of one stream line, in bytes (default 64 MiB). A larger
// body fails with [ErrResponseTooLarge].
func WithMaxBodySize(n int64) Option {
	return func(c *Client) error {
		if n <= 0 {
			return fmt.Errorf("%w: body size limit must be positive", ErrInvalidConfig)
		}
		c.maxBody = n
		return nil
	}
}

// WithStreamIdleTimeout ends a stream with [ErrStreamIdle] when no line, heartbeats included, arrives within d.
// The default of zero disables the check. The API sends heartbeats on the order and trade streams; a half-open
// connection otherwise stays silent forever.
func WithStreamIdleTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d < 0 {
			return fmt.Errorf("%w: stream idle timeout must not be negative", ErrInvalidConfig)
		}
		c.streamIdle = d
		return nil
	}
}
