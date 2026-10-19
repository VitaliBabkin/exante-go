package exante

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"net/http"
	"net/url"
	"time"
)

// streamMediaType is the newline-delimited JSON form of every stream endpoint.
const streamMediaType = "application/x-json-stream"

// OrderStream streams the account's order updates: GET /trade/3.0/stream/orders. See the Streams section of the package documentation.
func (c *Client) OrderStream(ctx context.Context) iter.Seq2[OrderUpdate, error] {
	return stream(ctx, c, nil, tradePath+"/stream/orders", nil, decodeEvent[OrderUpdate]("order"))
}

// TradeStream streams the account's trade (fill) events: GET /trade/3.0/stream/trades. See the Streams section of the package documentation.
func (c *Client) TradeStream(ctx context.Context) iter.Seq2[Trade, error] {
	return stream(ctx, c, nil, tradePath+"/stream/trades", nil, decodeEvent[Trade]("trade"))
}

// QuoteFeed streams live quotes of the given instruments: GET /md/3.0/feed/{symbolIds}. Each element holds a
// quote or a subscription notice. An empty level selects the API default, [FeedBestPrice]. See the Streams
// section of the package documentation.
func (c *Client) QuoteFeed(ctx context.Context, level FeedLevel, symbolIDs ...string) iter.Seq2[FeedEvent[Quote], error] {
	var query url.Values
	if level != "" {
		query = url.Values{"level": {string(level)}}
	}
	return stream(ctx, c, query, mdPath+"/feed/%s", [][]string{symbolIDs}, decodeFeed[Quote])
}

// TradeFeed streams live market trades of the given instruments: GET /md/3.0/feed/trades/{symbolIds}. Each element
// holds a trade or a subscription notice. See the Streams section of the package documentation.
func (c *Client) TradeFeed(ctx context.Context, symbolIDs ...string) iter.Seq2[FeedEvent[TradeTick], error] {
	return stream(ctx, c, nil, mdPath+"/feed/trades/%s", [][]string{symbolIDs}, decodeFeed[TradeTick])
}

// decodeFunc turns one stream line into an element. It reports false to drop the line without delivering it.
type decodeFunc[T any] func(line []byte) (v T, deliver bool, err error)

// decodeEvent decodes lines whose "event" field equals kind; other kinds (heartbeats included) are dropped.
func decodeEvent[T any](kind string) decodeFunc[T] {
	return func(line []byte) (T, bool, error) {
		var out T
		var head struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			return out, false, fmt.Errorf("exante: decoding stream line: %w", err)
		}
		if head.Event != kind {
			return out, false, nil
		}
		if err := json.Unmarshal(line, &out); err != nil {
			return out, false, fmt.Errorf("exante: decoding %s event: %w", kind, err)
		}
		return out, true, nil
	}
}

// decodeFeed decodes market-data lines; heartbeats and unknown events decode to an empty [FeedEvent] and are dropped.
func decodeFeed[T any](line []byte) (FeedEvent[T], bool, error) {
	var ev FeedEvent[T]
	if err := ev.UnmarshalJSON(line); err != nil {
		return ev, false, fmt.Errorf("exante: decoding stream line: %w", err)
	}
	return ev, !ev.empty(), nil
}

// stream builds the sequence for the GET stream endpoint tmpl.
func stream[T any](
	ctx context.Context, c *Client, query url.Values, tmpl string, args [][]string, decode decodeFunc[T],
) iter.Seq2[T, error] {
	cl := &call{method: http.MethodGet, tmpl: tmpl, args: args, query: query, accept: streamMediaType}
	return func(yield func(T, error) bool) {
		var zero T
		if ctx == nil {
			yield(zero, ErrNilContext)
			return
		}
		ctx, cancel := context.WithCancelCause(ctx)
		defer cancel(nil)

		resp, err := c.openStream(ctx, cancel, cl)
		if err != nil {
			yield(zero, err)
			return
		}
		defer discardClose(resp.Body)

		readLines(ctx, cancel, c, resp.Body, decode, yield)
	}
}

// openStream sends the request, bounding the wait for the response headers by the client timeout.
func (c *Client) openStream(ctx context.Context, cancel context.CancelCauseFunc, cl *call) (*http.Response, error) {
	timer := time.AfterFunc(c.timeout, func() { cancel(context.DeadlineExceeded) })
	ex, err := c.send(ctx, cl)
	if !timer.Stop() {
		if ex.resp != nil {
			discardClose(ex.resp.Body)
		}
		return nil, fmt.Errorf("exante: %s %s: %w", cl.method, ex.path, context.DeadlineExceeded)
	}
	return ex.resp, err
}

// readLines delivers the decoded lines of body to yield until the body ends, fails or yield returns false.
func readLines[T any](
	ctx context.Context, cancel context.CancelCauseFunc, c *Client, body io.Reader,
	decode decodeFunc[T], yield func(T, error) bool,
) {
	var zero T
	idle := newIdleWatch(c.streamIdle, cancel)
	defer idle.stop()
	ls := newLineScanner(body, c.maxBody)
	for {
		line, ok := ls.next(idle)
		if !ok || ctx.Err() != nil {
			break
		}
		v, deliver, err := decode(line)
		switch {
		case err != nil && ls.unterminated && !json.Valid(line):
			yield(zero, truncatedLine(endOfStream(ctx, ls.drain())))
			return
		case err != nil:
			if !yield(zero, err) {
				return
			}
		case deliver:
			if !yield(v, nil) {
				return
			}
		}
	}
	yield(zero, endOfStream(ctx, ls.sc.Err()))
}

// lineScanner splits a stream body into lines and remembers whether the line it returned last ended without a
// newline, which for the final line means the connection was cut inside it.
type lineScanner struct {
	sc           *bufio.Scanner
	unterminated bool
}

func newLineScanner(body io.Reader, maxLine int64) *lineScanner {
	ls := &lineScanner{sc: bufio.NewScanner(body)}
	ls.sc.Buffer(make([]byte, 0, min(maxLine, 64<<10)), int(min(maxLine, math.MaxInt32)))
	ls.sc.Split(ls.split)
	return ls
}

func (l *lineScanner) split(data []byte, atEOF bool) (advance int, token []byte, err error) {
	advance, token, err = bufio.ScanLines(data, atEOF)
	l.unterminated = token != nil && advance == len(data) && data[advance-1] != '\n'
	return advance, token, err
}

// next returns the next non-blank line. The idle watch runs only while next waits for the server, never while the
// caller processes a line, so a slow consumer cannot cause [ErrStreamIdle].
func (l *lineScanner) next(idle *idleWatch) ([]byte, bool) {
	for {
		idle.reset()
		ok := l.sc.Scan()
		idle.stop()
		if !ok {
			return nil, false
		}
		if line := bytes.TrimSpace(l.sc.Bytes()); len(line) > 0 {
			return line, true
		}
	}
}

// drain reads past the last line, which makes the scanner report the error that cut the body, if any.
func (l *lineScanner) drain() error {
	l.sc.Scan()
	return l.sc.Err()
}

// truncatedLine marks the end of a stream whose last line is incomplete.
func truncatedLine(end error) error {
	return fmt.Errorf("%w: the last line is incomplete: %w", end, io.ErrUnexpectedEOF)
}

// endOfStream names why a stream body ended. A finished context wins over what the transport reported: cancellation
// and deadline are the caller's own doing, not a server close.
func endOfStream(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil && err == nil:
		return fmt.Errorf("exante: reading stream: %w", context.Cause(ctx))
	case ctx.Err() != nil:
		return fmt.Errorf("exante: reading stream: %w", causeOr(ctx, err))
	case err == nil:
		return ErrStreamClosed
	case errors.Is(err, bufio.ErrTooLong):
		return fmt.Errorf("%w: a stream line exceeds the limit", ErrResponseTooLarge)
	default:
		return fmt.Errorf("exante: reading stream: %w", err)
	}
}

// idleWatch cancels a stream that stays silent. A nil *idleWatch is disabled and all its methods are no-ops.
type idleWatch struct {
	timer *time.Timer
	limit time.Duration
}

func newIdleWatch(limit time.Duration, cancel context.CancelCauseFunc) *idleWatch {
	if limit <= 0 {
		return nil
	}
	return &idleWatch{timer: time.AfterFunc(limit, func() { cancel(ErrStreamIdle) }), limit: limit}
}

func (w *idleWatch) reset() {
	if w != nil {
		w.timer.Reset(w.limit)
	}
}

func (w *idleWatch) stop() {
	if w != nil {
		w.timer.Stop()
	}
}
