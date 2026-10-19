package exante

import (
	"context"
	"errors"
	"math"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestAPIErrorWithBody(t *testing.T) {
	rec := &recorder{status: http.StatusBadRequest, body: fixture(t, "error_400.json")}
	c := newTestClient(t, rec)

	_, err := c.Orders(context.Background(), &OrdersQuery{Limit: 1})
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("error %v is not an *APIError", err)
	}
	if apiErr.StatusCode != 400 || apiErr.Method != "GET" || apiErr.Path != "/trade/3.0/orders" {
		t.Errorf("APIError = %+v", apiErr)
	}
	if apiErr.Body == nil || apiErr.Body.Group != ErrorGroupClient ||
		apiErr.Body.Message != "the query parameters do not have correct format" {
		t.Errorf("Body = %+v, want the decoded error object", apiErr.Body)
	}
	if !strings.Contains(string(apiErr.Raw), "correct format") {
		t.Errorf("Raw = %q", apiErr.Raw)
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "correct format") {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestAPIErrorRawBodyTruncated(t *testing.T) {
	body := "error code: 1010" + strings.Repeat("x", 10_000)
	c := newTestClient(t, reply(http.StatusForbidden, body))

	_, err := c.Groups(context.Background())
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("error %v is not an *APIError", err)
	}
	if apiErr.Body != nil {
		t.Errorf("Body = %+v, want nil for a non-JSON answer", apiErr.Body)
	}
	if len(apiErr.Raw) != 4096 || !strings.HasPrefix(string(apiErr.Raw), "error code: 1010") {
		t.Errorf("Raw has %d bytes %.20q, want 4096 starting with the edge message", len(apiErr.Raw), apiErr.Raw)
	}
}

func TestAPIErrorRetryAfter(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"seconds", "7", 7 * time.Second},
		{"absent", "", 0},
		{"garbage", "soon", 0},
		{"negative", "-3", 0},
		{"overflow", "99999999999999999", time.Duration(math.MaxInt64/int64(time.Second)) * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.header != "" {
					w.Header().Set("Retry-After", tt.header)
				}
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			_, err := c.Groups(context.Background())
			apiErr, ok := errors.AsType[*APIError](err)
			if !ok {
				t.Fatalf("error %v is not an *APIError", err)
			}
			if apiErr.RetryAfter != tt.want {
				t.Errorf("RetryAfter = %v, want %v", apiErr.RetryAfter, tt.want)
			}
		})
	}
}

func TestParseRetryAfterDate(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	if got := parseRetryAfter(now.Add(90*time.Second).Format(http.TimeFormat), now); got != 90*time.Second {
		t.Errorf("future date: %v, want 90s", got)
	}
	if got := parseRetryAfter(now.Add(-time.Hour).Format(http.TimeFormat), now); got != 0 {
		t.Errorf("past date: %v, want 0", got)
	}
}

func TestNilContext(t *testing.T) {
	c := newTestClient(t, reply(200, `[]`))
	var nilCtx context.Context
	if _, err := c.Groups(nilCtx); !errors.Is(err, ErrNilContext) {
		t.Errorf("error = %v, want ErrNilContext", err)
	}
	n := 0
	for _, err := range c.OrderStream(nilCtx) {
		n++
		if !errors.Is(err, ErrNilContext) {
			t.Errorf("stream error = %v, want ErrNilContext", err)
		}
	}
	if n != 1 {
		t.Errorf("the stream yielded %d elements, want exactly the one error", n)
	}
	if _, err := c.CancelOrder(nilCtx, orderUUID); !errors.Is(err, ErrNilContext) || errors.Is(err, ErrOutcomeUnknown) {
		t.Errorf("CancelOrder error = %v, want ErrNilContext and no unknown outcome", err)
	}
}

func TestEmptyArguments(t *testing.T) {
	rec := reply(200, `{}`)
	c := newTestClient(t, rec)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"symbol", func() error { _, err := c.Symbol(ctx, ""); return err }, ErrEmptyArgument},
		{"group", func() error { _, err := c.GroupSymbols(ctx, ""); return err }, ErrEmptyArgument},
		{"symbol list", func() error { _, err := c.DailyChange(ctx); return err }, ErrEmptyArgument},
		{"empty id in list", func() error { _, err := c.DailyChange(ctx, "A.B", ""); return err }, ErrEmptyArgument},
		{"dot id", func() error { _, err := c.Symbol(ctx, ".."); return err }, ErrInvalidArgument},
		{"crossrate", func() error { _, err := c.Crossrate(ctx, "EUR", ""); return err }, ErrEmptyArgument},
		{"order id", func() error { _, err := c.Order(ctx, ""); return err }, ErrEmptyArgument},
		{"cancel", func() error { _, err := c.CancelOrder(ctx, ""); return err }, ErrEmptyArgument},
		{"summary account", func() error { _, err := c.Summary(ctx, "", "EUR"); return err }, ErrEmptyArgument},
		{"bad date", func() error { _, err := c.SummaryOn(ctx, "A.001", "2013-02-30", "EUR"); return err }, ErrBadDate},
		{"date format", func() error { _, err := c.SummaryOn(ctx, "A.001", "16.02.2013", "EUR"); return err }, ErrBadDate},
		{"empty date", func() error { _, err := c.SummaryOn(ctx, "A.001", "", "EUR"); return err }, ErrBadDate},
		{"ohlc interval", func() error { _, err := c.OHLC(ctx, "A.B", 7*time.Minute, nil); return err }, ErrInvalidArgument},
		{"place no symbol", func() error { _, err := c.PlaceOrder(ctx, &PlaceParams{Side: Buy}); return err }, ErrEmptyArgument},
		{"place nil", func() error { _, err := c.PlaceOrder(ctx, nil); return err }, ErrEmptyArgument},
		{"place no quantity", func() error {
			_, err := c.PlaceOrder(ctx, &PlaceParams{SymbolID: "A.B"})
			return err
		}, ErrEmptyArgument},
		{"cancel dot id", func() error { _, err := c.CancelOrder(ctx, ".."); return err }, ErrInvalidArgument},
		{"replace nil", func() error { _, err := c.ReplaceOrder(ctx, "id", nil); return err }, ErrEmptyArgument},
		{"estimate nil", func() error { _, err := c.EstimateOrder(ctx, nil); return err }, ErrEmptyArgument},
		{"replace no quantity", func() error { _, err := c.ReplaceOrder(ctx, "id", &ReplaceParams{}); return err }, ErrEmptyArgument},
		{"estimate no orders", func() error {
			_, err := c.EstimateOrder(ctx, &EstimateParams{AccountID: "A", Currency: "EUR"})
			return err
		}, ErrEmptyArgument},
		{"estimate bad item", func() error {
			_, err := c.EstimateOrder(ctx, &EstimateParams{AccountID: "A", Currency: "EUR", Orders: []EstimateItem{{}}})
			return err
		}, ErrEmptyArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if errors.Is(err, ErrOutcomeUnknown) {
				t.Errorf("error = %v, must not claim an unknown outcome: nothing was sent", err)
			}
		})
	}
	if n := len(rec.all()); n != 0 {
		t.Errorf("server saw %d requests, want none for invalid input", n)
	}
}

func TestURLLengthLimitIsExactly2000(t *testing.T) {
	rec := reply(200, `{}`)
	c := newTestClient(t, rec)
	prefix := len(c.base.String()) + len("/md/3.0/symbols/")

	if _, err := c.Symbol(context.Background(), strings.Repeat("a", 2000-prefix)); err != nil {
		t.Errorf("a URL of 2000 characters failed: %v", err)
	}
	if _, err := c.Symbol(context.Background(), strings.Repeat("a", 2001-prefix)); !errors.Is(err, ErrURLTooLong) {
		t.Errorf("a URL of 2001 characters: error = %v, want ErrURLTooLong", err)
	}
	ids := slices.Repeat([]string{"SYMBOLXXXXX.NASDAQ"}, 300)
	if _, err := c.DailyChange(context.Background(), ids...); !errors.Is(err, ErrURLTooLong) {
		t.Errorf("a list of %d ids: error = %v, want ErrURLTooLong", len(ids), err)
	}
	if n := len(rec.all()); n != 1 {
		t.Errorf("server saw %d requests, want only the 2000-character one", n)
	}
}

func TestBodySizeCeilingIsExact(t *testing.T) {
	const limit = 100
	body := "[]" + strings.Repeat(" ", limit-2)
	c := newTestClient(t, reply(200, body), WithMaxBodySize(limit))
	if _, err := c.Groups(context.Background()); err != nil {
		t.Errorf("a body of exactly %d bytes: %v", limit, err)
	}

	c = newTestClient(t, reply(200, body+" "), WithMaxBodySize(limit))
	if _, err := c.Groups(context.Background()); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("a body of %d bytes: error = %v, want ErrResponseTooLarge", limit+1, err)
	}
}

func TestMalformedAnswer(t *testing.T) {
	c := newTestClient(t, reply(200, `{"id": `))
	if _, err := c.Symbol(context.Background(), "AAPL.NASDAQ"); err == nil {
		t.Error("truncated JSON was accepted")
	}
}

func TestContextCancelAbortsSlowServer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		ctx, cancel := context.WithCancel(context.Background())
		errc := make(chan error, 1)
		go func() {
			_, err := c.Groups(ctx)
			errc <- err
		}()
		synctest.Wait()
		cancel()
		err := <-errc
		close(release)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	})
}

func TestClientTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var hits atomic.Int32
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			hangUntilGone(w, r)
		}), WithTimeout(5*time.Second))

		start := time.Now()
		_, err := c.Groups(context.Background())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want context.DeadlineExceeded", err)
		}
		if got := time.Since(start); got != 5*time.Second {
			t.Errorf("call took %v, want exactly the 5s timeout", got)
		}
		if hits.Load() != 1 {
			t.Errorf("server hit %d times, want 1 (no retries)", hits.Load())
		}
	})
}

func TestDefaultTimeoutIsSixtySeconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestClient(t, hangUntilGone)
		start := time.Now()
		if _, err := c.Groups(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want context.DeadlineExceeded", err)
		}
		if got := time.Since(start); got != 60*time.Second {
			t.Errorf("call took %v, want 60s", got)
		}
	})
}

func TestTimeoutCoversBodyRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}), WithTimeout(2*time.Second))
		if _, err := c.Groups(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want context.DeadlineExceeded from the body read", err)
		}
	})
}

func TestPathEscaping(t *testing.T) {
	tests := []struct {
		name string
		call func(c *Client) error
		want string
	}{
		{"dot and exchange", func(c *Client) error { _, err := c.Symbol(context.Background(), "AAPL.NASDAQ"); return err },
			"/md/3.0/symbols/AAPL.NASDAQ"},
		{"slash in id", func(c *Client) error { _, err := c.Symbol(context.Background(), "EUR/USD.E.FX"); return err },
			"/md/3.0/symbols/EUR%2FUSD.E.FX"},
		{"space in id", func(c *Client) error { _, err := c.Symbol(context.Background(), "BRK B.NYSE"); return err },
			"/md/3.0/symbols/BRK%20B.NYSE"},
		{"percent and question mark", func(c *Client) error { _, err := c.Symbol(context.Background(), "A%B?C#D"); return err },
			"/md/3.0/symbols/A%25B%3FC%23D"},
		{"unicode", func(c *Client) error { _, err := c.Symbol(context.Background(), "ГАЗП.MICEX"); return err },
			"/md/3.0/symbols/%D0%93%D0%90%D0%97%D0%9F.MICEX"},
		{"list keeps commas", func(c *Client) error {
			_, err := c.DailyChange(context.Background(), "MSFT.NASDAQ", "EUR/USD.E.FX", "GAZP.MICEX")
			return err
		}, "/md/3.0/change/MSFT.NASDAQ,EUR%2FUSD.E.FX,GAZP.MICEX"},
		{"crossrate currencies", func(c *Client) error { _, err := c.Crossrate(context.Background(), "EUR", "USD"); return err },
			"/md/3.0/crossrates/EUR/USD"},
		{"account with dot in summary", func(c *Client) error {
			_, err := c.Summary(context.Background(), "ABC1234.001", "EUR")
			return err
		}, "/md/3.0/summary/ABC1234.001/EUR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := reply(200, `{}`)
			c := newTestClient(t, rec)
			_ = tt.call(c)
			if got := rec.only(t).URI; got != tt.want {
				t.Errorf("request URI = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEscapedSegmentsDecodeOnServer(t *testing.T) {
	var gotPath atomic.Pointer[string]
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(&r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	}))
	if _, err := c.Symbol(context.Background(), "EUR/USD.E.FX"); err != nil {
		t.Fatal(err)
	}
	if got, want := *gotPath.Load(), "/md/3.0/symbols/EUR/USD.E.FX"; got != want {
		t.Errorf("server-side decoded path = %q, want %q", got, want)
	}
}
