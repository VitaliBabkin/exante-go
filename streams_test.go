package exante

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type item[T any] struct {
	v   T
	err error
}

// collect drains a stream that ends by itself.
func collect[T any](seq iter.Seq2[T, error]) []item[T] {
	var out []item[T]
	for v, err := range seq {
		out = append(out, item[T]{v, err})
	}
	return out
}

func TestOrderStream(t *testing.T) {
	rec := &recorder{status: 200, body: fixture(t, "order_stream.ndjson")}
	c := newTestClient(t, rec)

	got := collect(c.OrderStream(context.Background()))

	if len(got) != 3 {
		t.Fatalf("got %d elements, want 2 updates and the closing error: %+v", len(got), got)
	}
	if got[0].err != nil || got[0].v.Order.State.Status != StatusWorking {
		t.Errorf("first element = %+v", got[0])
	}
	if got[1].err != nil || got[1].v.Order.State.Status != StatusFilled ||
		got[1].v.Order.State.Fills[0].Quantity != "6" {
		t.Errorf("second element = %+v", got[1])
	}
	if !errors.Is(got[2].err, ErrStreamClosed) {
		t.Errorf("final error = %v, want ErrStreamClosed", got[2].err)
	}
	req := rec.only(t)
	if req.Method != "GET" || req.URI != "/trade/3.0/stream/orders" {
		t.Errorf("request = %s %s", req.Method, req.URI)
	}
	if req.Header.Get("Accept") != "application/x-json-stream" {
		t.Errorf("Accept = %q", req.Header.Get("Accept"))
	}
	if req.Header.Get("Authorization") == "" || req.Header.Get("User-Agent") == "" {
		t.Errorf("stream request lacks Authorization or User-Agent: %v", req.Header)
	}
}

func TestTradeStream(t *testing.T) {
	rec := &recorder{status: 200, body: fixture(t, "trade_stream.ndjson")}
	c := newTestClient(t, rec)

	got := collect(c.TradeStream(context.Background()))

	if len(got) != 3 || !errors.Is(got[2].err, ErrStreamClosed) {
		t.Fatalf("elements = %+v, want 2 trades and ErrStreamClosed", got)
	}
	if tr := got[0].v; tr.OrderID != "2abe909b-ec53-4091-a0d4-ea4f82aa047c" || tr.Quantity != "1" ||
		tr.Price != "101.1" || tr.Position != 1 || tr.Timestamp.Year() != 2017 {
		t.Errorf("first trade = %+v", tr)
	}
	if tr := got[1].v; tr.Quantity != "2" || tr.Price != "101.2" || tr.Timestamp.Nanosecond() != 500_000_000 {
		t.Errorf("second trade (numbers instead of strings) = %+v", tr)
	}
	if uri := rec.only(t).URI; uri != "/trade/3.0/stream/trades" {
		t.Errorf("URI = %q", uri)
	}
}

func TestQuoteFeed(t *testing.T) {
	rec := &recorder{status: 200, body: fixture(t, "quote_feed.ndjson")}
	c := newTestClient(t, rec)

	got := collect(c.QuoteFeed(context.Background(), FeedMarketDepth, "AAPL.NASDAQ", "EUR/USD.E.FX"))

	if len(got) != 4 || !errors.Is(got[3].err, ErrStreamClosed) {
		t.Fatalf("elements = %+v, want start, quote, stop and ErrStreamClosed", got)
	}
	if s := got[0].v.Status; s == nil || s.Event != SubscriptionStart || s.SymbolID != "AAPL.NASDAQ" {
		t.Errorf("first = %+v", got[0].v)
	}
	if q := got[1].v.Data; q == nil || q.Bid[0].Price != "101.02" || q.Ask[0].Size != "200" {
		t.Errorf("second = %+v", got[1].v)
	}
	if s := got[2].v.Status; s == nil || s.Event != SubscriptionStop || s.Reason == "" {
		t.Errorf("third = %+v", got[2].v)
	}
	want := "/md/3.0/feed/AAPL.NASDAQ,EUR%2FUSD.E.FX?level=market_depth"
	if uri := rec.only(t).URI; uri != want {
		t.Errorf("URI = %q, want %q", uri, want)
	}
}

func TestQuoteFeedDefaultLevel(t *testing.T) {
	rec := &recorder{status: 200, body: nil}
	c := newTestClient(t, rec)
	collect(c.QuoteFeed(context.Background(), "", "AAPL.NASDAQ"))
	if uri := rec.only(t).URI; uri != "/md/3.0/feed/AAPL.NASDAQ" {
		t.Errorf("URI = %q, want no level parameter", uri)
	}
}

func TestTradeFeed(t *testing.T) {
	rec := &recorder{status: 200, body: fixture(t, "trade_feed.ndjson")}
	c := newTestClient(t, rec)

	got := collect(c.TradeFeed(context.Background(), "AAPL.NASDAQ", "MSFT.NASDAQ"))

	if len(got) != 3 || !errors.Is(got[2].err, ErrStreamClosed) {
		t.Fatalf("elements = %+v", got)
	}
	if d := got[0].v.Data; d == nil || d.Price != "101.02" || d.Size != "42" || d.Timestamp != 1550833075530 {
		t.Errorf("first = %+v", got[0].v)
	}
	if s := got[1].v.Status; s == nil || s.SymbolID != "MSFT.NASDAQ" {
		t.Errorf("second = %+v", got[1].v)
	}
	if uri := rec.only(t).URI; uri != "/md/3.0/feed/trades/AAPL.NASDAQ,MSFT.NASDAQ" {
		t.Errorf("URI = %q", uri)
	}
}

func TestStreamNon2xx(t *testing.T) {
	rec := reply(http.StatusPaymentRequired, "no subscription")
	c := newTestClient(t, rec)

	got := collect(c.QuoteFeed(context.Background(), FeedBestPrice, "AAPL.NASDAQ"))

	if len(got) != 1 {
		t.Fatalf("got %d elements, want exactly the error", len(got))
	}
	apiErr, ok := errors.AsType[*APIError](got[0].err)
	if !ok || apiErr.StatusCode != http.StatusPaymentRequired || string(apiErr.Raw) != "no subscription" {
		t.Errorf("error = %v", got[0].err)
	}
}

func TestStreamMalformedLineIsDeliveredAndStreamContinues(t *testing.T) {
	body := "{not json\n" +
		`{"event":"trade","orderId":"o1","timestamp":"bad time","quantity":"1","price":"1","position":1}` + "\n" +
		`{"event":"trade","orderId":"o2","timestamp":"2017-08-14T02:40:00Z","quantity":"1","price":"1","position":2}` + "\n"
	c := newTestClient(t, reply(200, body))

	got := collect(c.TradeStream(context.Background()))

	if len(got) != 4 {
		t.Fatalf("elements = %+v, want 2 errors, 1 trade and the closing error", got)
	}
	if got[0].err == nil || got[1].err == nil {
		t.Errorf("malformed lines were not reported: %+v", got[:2])
	}
	if got[2].err != nil || got[2].v.OrderID != "o2" {
		t.Errorf("valid line after the bad ones = %+v", got[2])
	}
	if !errors.Is(got[3].err, ErrStreamClosed) {
		t.Errorf("final error = %v", got[3].err)
	}
}

func TestStreamLineTooLong(t *testing.T) {
	body := `{"event":"trade","orderId":"` + strings.Repeat("x", 500) + `"}` + "\n"
	c := newTestClient(t, reply(200, body), WithMaxBodySize(200))

	got := collect(c.TradeStream(context.Background()))

	if len(got) != 1 || !errors.Is(got[0].err, ErrResponseTooLarge) {
		t.Errorf("elements = %+v, want ErrResponseTooLarge", got)
	}
}

// openStreamServer answers with first, flushes it and then keeps the connection open until the client goes away.
// The returned channel is closed when the handler returns, which proves the connection was released.
func openStreamServer(t *testing.T, first string) (*Client, <-chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		_, _ = w.Write([]byte(first))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	return c, done
}

const tradeLine = `{"event":"trade","orderId":"o1","timestamp":"2017-08-14T02:40:00Z","quantity":"1","price":"1","position":1}` + "\n"

func TestStreamEndsOnContextCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, done := openStreamServer(t, tradeLine)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var got []item[Trade]
		for v, err := range c.TradeStream(ctx) {
			got = append(got, item[Trade]{v, err})
			if err == nil {
				cancel()
			}
		}

		if len(got) != 2 || got[0].err != nil || !errors.Is(got[1].err, context.Canceled) {
			t.Fatalf("elements = %+v, want one trade and a context.Canceled error", got)
		}
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Error("the server connection was not released after cancellation")
		}
	})
}

func TestStreamEndsOnContextDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _ := openStreamServer(t, tradeLine)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		got := collect(c.TradeStream(ctx))

		if len(got) != 2 || !errors.Is(got[1].err, context.DeadlineExceeded) {
			t.Fatalf("elements = %+v, want one trade and a deadline error", got)
		}
	})
}

func TestStreamReleasesConnectionWhenConsumerBreaks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, done := openStreamServer(t, tradeLine+tradeLine)

		n := 0
		for v, err := range c.TradeStream(context.Background()) {
			if err != nil || v.OrderID != "o1" {
				t.Fatalf("element = %+v, %v", v, err)
			}
			if n++; n == 1 {
				break
			}
		}

		time.Sleep(time.Second) // the transport closes the abandoned connection asynchronously
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Error("the server connection was not released after the consumer stopped")
		}
	})
}

func TestStreamIdleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _ := openStreamServer(t, `{"event":"heartbeat"}`+"\n")
		c.streamIdle = 10 * time.Second

		start := time.Now()
		got := collect(c.TradeStream(context.Background()))

		if len(got) != 1 || !errors.Is(got[0].err, ErrStreamIdle) {
			t.Fatalf("elements = %+v, want ErrStreamIdle", got)
		}
		if d := time.Since(start); d != 10*time.Second {
			t.Errorf("stream ended after %v, want 10s", d)
		}
	})
}

func TestStreamHeartbeatsKeepItAlive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for range 6 {
				_, _ = w.Write([]byte(`{"event":"heartbeat"}` + "\n"))
				w.(http.Flusher).Flush()
				time.Sleep(4 * time.Second)
			}
			_, _ = w.Write([]byte(tradeLine))
		}), WithStreamIdleTimeout(5*time.Second))

		got := collect(c.TradeStream(context.Background()))

		if len(got) != 2 || got[0].err != nil || !errors.Is(got[1].err, ErrStreamClosed) {
			t.Errorf("elements = %+v, want the trade and ErrStreamClosed", got)
		}
	})
}

func TestTimeoutBoundsOnlyTheHeadersOfAStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for range 5 {
				_, _ = w.Write([]byte(tradeLine))
				w.(http.Flusher).Flush()
				time.Sleep(2 * time.Second)
			}
		}), WithTimeout(3*time.Second))

		got := collect(c.TradeStream(context.Background()))

		if len(got) != 6 || !errors.Is(got[5].err, ErrStreamClosed) {
			t.Errorf("got %d elements, want 5 trades and ErrStreamClosed (10s of stream with a 3s timeout)", len(got))
		}
	})
}

func TestStreamHeaderTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestClient(t, hangUntilGone, WithTimeout(3*time.Second))

		start := time.Now()
		got := collect(c.TradeStream(context.Background()))

		if len(got) != 1 || !errors.Is(got[0].err, context.DeadlineExceeded) {
			t.Fatalf("elements = %+v, want a deadline error", got)
		}
		if d := time.Since(start); d != 3*time.Second {
			t.Errorf("gave up after %v, want 3s", d)
		}
	})
}

// closeTracker is a response body that reports whether it was closed.
type closeTracker struct {
	io.Reader
	closed atomic.Bool
}

func (b *closeTracker) Close() error {
	b.closed.Store(true)
	return nil
}

// trackingTransport answers every request with a canned response whose body records its closing.
type trackingTransport struct {
	status int
	body   string
	last   *closeTracker
}

func (tt *trackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tt.last = &closeTracker{Reader: strings.NewReader(tt.body)}
	return &http.Response{
		StatusCode: tt.status, Status: http.StatusText(tt.status), Header: http.Header{},
		Body: tt.last, Request: req,
	}, nil
}

func TestResponseBodiesAreAlwaysClosed(t *testing.T) {
	const lines = `{"event":"order","order":{}}` + "\n" + `{"event":"order","order":{}}` + "\n"
	tests := []struct {
		name   string
		status int
		body   string
		run    func(c *Client)
	}{
		{"call", 200, `[]`, func(c *Client) { _, _ = c.Groups(context.Background()) }},
		{"call error", 500, `{"message":"x"}`, func(c *Client) { _, _ = c.Groups(context.Background()) }},
		{"stream drained", 200, lines, func(c *Client) { collect(c.OrderStream(context.Background())) }},
		{"stream broken off", 200, lines, func(c *Client) {
			for range c.OrderStream(context.Background()) {
				break
			}
		}},
		{"stream error status", 503, `down`, func(c *Client) { collect(c.OrderStream(context.Background())) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &trackingTransport{status: tt.status, body: tt.body}
			c := newClient(t, WithHTTPClient(&http.Client{Transport: tr}))
			tt.run(c)
			if tr.last == nil || !tr.last.closed.Load() {
				t.Error("response body was not closed")
			}
		})
	}
}
