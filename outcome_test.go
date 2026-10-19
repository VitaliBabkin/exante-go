package exante

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// cutOff answers 201 with a Content-Length it does not honour and then drops the connection.
var cutOff = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Length", "1000")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`[{"orderId":"o`))
	w.(http.Flusher).Flush()
	panic(http.ErrAbortHandler)
})

func TestWriteErrorsAfterSendWrapOutcomeUnknown(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		call    string
		cause   error // an error the result must also match, if any
	}{
		{"unreadable 2xx on place", reply(201, `"accepted"`), "PlaceOrder", nil},
		{"unreadable 2xx on cancel", reply(200, `[]`), "CancelOrder", nil},
		{"unreadable 2xx on replace", reply(200, `not json`), "ReplaceOrder", nil},
		{"object without known fields on place", reply(201, `{"message":"odd"}`), "PlaceOrder", nil},
		{"500 on place", reply(500, `{"group":"server","message":"boom"}`), "PlaceOrder", nil},
		{"503 on cancel", reply(503, ``), "CancelOrder", nil},
		{"502 on replace", reply(502, `<html>bad gateway</html>`), "ReplaceOrder", nil},
		{"connection cut mid body", cutOff, "PlaceOrder", nil},
		{"answer over the size limit", reply(201, strings.Repeat(" ", 600)), "PlaceOrder", ErrResponseTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, tt.handler, WithMaxBodySize(500))

			err := writeCalls()[tt.call](c)

			if !errors.Is(err, ErrOutcomeUnknown) {
				t.Fatalf("error = %v, want it to wrap ErrOutcomeUnknown", err)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Errorf("error = %v, want it to keep the cause %v", err, tt.cause)
			}
		})
	}
}

func TestWriteKeepsTheOriginalCauseBesidesOutcomeUnknown(t *testing.T) {
	c := newTestClient(t, reply(503, `{"group":"server","message":"maintenance"}`))

	err := writeCalls()["CancelOrder"](c)

	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.StatusCode != 503 || !errors.Is(err, ErrOutcomeUnknown) {
		t.Errorf("error = %v, want an *APIError 503 that also wraps ErrOutcomeUnknown", err)
	}
}

func TestWriteTimeoutAndCancelAfterSendAreOutcomeUnknown(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := newTestClient(t, hangUntilGone, WithTimeout(2*time.Second))

			err := writeCalls()["PlaceOrder"](c)

			if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("error = %v, want ErrOutcomeUnknown and DeadlineExceeded", err)
			}
		})
	})
	t.Run("cancel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := newTestClient(t, hangUntilGone)
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(time.Second, cancel)

			_, err := c.CancelOrder(ctx, orderUUID)

			if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
				t.Errorf("error = %v, want ErrOutcomeUnknown and Canceled", err)
			}
		})
	})
}

func TestWriteTransportErrorIsOutcomeUnknown(t *testing.T) {
	c := newTestClientOver(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	}))

	err := writeCalls()["PlaceOrder"](c)

	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Errorf("error = %v, want ErrOutcomeUnknown", err)
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDefiniteRejectionIsNotOutcomeUnknown(t *testing.T) {
	for name, run := range writeCalls() {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, reply(400, `{"group":"client","message":"bad quantity"}`))

			err := run(c)

			if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.StatusCode != 400 {
				t.Fatalf("error = %v, want an *APIError 400", err)
			}
			if errors.Is(err, ErrOutcomeUnknown) {
				t.Errorf("error = %v, a 4xx answer must not claim an unknown outcome", err)
			}
		})
	}
}

func TestReadErrorsNeverClaimAnUnknownOutcome(t *testing.T) {
	c := newTestClient(t, reply(503, ``))

	_, err := c.Groups(context.Background())

	if err == nil || errors.Is(err, ErrOutcomeUnknown) {
		t.Errorf("error = %v, want a plain error", err)
	}
}

func TestPlaceOrderAcceptsEveryShapeOfASuccessfulAnswer(t *testing.T) {
	single := string(fixture(t, "order.json"))
	tests := []struct {
		name string
		body string
		want int
	}{
		{"array", string(fixture(t, "orders.json")), 2},
		{"single object", single, 1},
		{"empty array", `[]`, 0},
		{"empty body", ``, 0},
		{"null", `null`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, reply(http.StatusCreated, tt.body))

			got, err := c.PlaceOrder(context.Background(), &PlaceParams{
				SymbolID: "AAPL.NASDAQ", Side: Buy, Quantity: "1", OrderType: OrderMarket, Duration: DurationDay,
			})

			if err != nil || len(got) != tt.want {
				t.Errorf("PlaceOrder = %d orders, %v; want %d orders and no error", len(got), err, tt.want)
			}
		})
	}
}

func TestUnreadableAnswerOfAWriteNamesTheStatusOnce(t *testing.T) {
	c := newTestClient(t, reply(http.StatusCreated, `"???"`))

	err := writeCalls()["PlaceOrder"](c)

	if err == nil || !errors.Is(err, ErrOutcomeUnknown) || !strings.Contains(err.Error(), "(HTTP 201)") {
		t.Errorf("error = %v, want ErrOutcomeUnknown and the HTTP 201 status", err)
	}
	if n := strings.Count(err.Error(), "decoding"); n != 1 {
		t.Errorf("error = %v, says \"decoding\" %d times, want one layer", err, n)
	}
}
