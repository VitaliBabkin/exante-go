package exante

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestSlowConsumerDoesNotTriggerTheIdleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for range 5 {
				_, _ = w.Write([]byte(tradeLine))
				w.(http.Flusher).Flush()
				time.Sleep(time.Second)
			}
		}), WithStreamIdleTimeout(3*time.Second))

		var got []item[Trade]
		for v, err := range c.TradeStream(context.Background()) {
			got = append(got, item[Trade]{v, err})
			time.Sleep(10 * time.Second) // the consumer is far slower than the idle limit
		}

		if len(got) != 6 || !errors.Is(got[5].err, ErrStreamClosed) {
			t.Fatalf("elements = %+v, want 5 trades and ErrStreamClosed", got)
		}
		for _, it := range got[:5] {
			if it.err != nil {
				t.Errorf("a trade came with an error: %v", it.err)
			}
		}
	})
}

func TestIdleTimeoutStillFiresWhileWaitingAfterASlowConsumer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _ := openStreamServer(t, tradeLine)
		c.streamIdle = 3 * time.Second

		var got []item[Trade]
		start := time.Now()
		for v, err := range c.TradeStream(context.Background()) {
			got = append(got, item[Trade]{v, err})
			if err == nil {
				time.Sleep(10 * time.Second)
			}
		}

		if len(got) != 2 || !errors.Is(got[1].err, ErrStreamIdle) {
			t.Fatalf("elements = %+v, want a trade and ErrStreamIdle", got)
		}
		if d := time.Since(start); d != 13*time.Second {
			t.Errorf("ended after %v, want 10s of consumer work plus 3s of silence", d)
		}
	})
}

func TestCancelWhileABufferedElementIsProcessedEndsWithTheContextError(t *testing.T) {
	body := strings.Repeat(tradeLine, 3)
	c := newTestClient(t, reply(200, body))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var got []item[Trade]
	for v, err := range c.TradeStream(ctx) {
		got = append(got, item[Trade]{v, err})
		if err == nil {
			cancel()
		}
	}

	if len(got) != 2 || got[0].err != nil {
		t.Fatalf("elements = %+v, want one trade and the final error (the buffered ones are not delivered)", got)
	}
	if !errors.Is(got[1].err, context.Canceled) || errors.Is(got[1].err, ErrStreamClosed) {
		t.Errorf("final error = %v, want context.Canceled and not ErrStreamClosed", got[1].err)
	}
}

func TestDeadlinePassedBeforeAnEndOfBodyIsADeadlineError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newTestClient(t, reply(200, strings.Repeat(tradeLine, 2)))
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		var got []item[Trade]
		for v, err := range c.TradeStream(ctx) {
			got = append(got, item[Trade]{v, err})
			time.Sleep(2 * time.Minute) // the deadline passes while the consumer works
		}

		last := got[len(got)-1].err
		if !errors.Is(last, context.DeadlineExceeded) || errors.Is(last, ErrStreamClosed) {
			t.Errorf("final error = %v, want DeadlineExceeded and not ErrStreamClosed", last)
		}
	})
}

// finalLineCase is a stream whose last line has no newline, with what the consumer must see.
type finalLineCase struct {
	name       string
	handler    http.Handler
	trades     int  // trades delivered before the end
	decodeErr  bool // a decoding error element precedes the end
	truncated  bool // the end wraps io.ErrUnexpectedEOF
	alsoClosed bool // the end also matches ErrStreamClosed
}

func (tt finalLineCase) check(t *testing.T, got []item[Trade]) {
	t.Helper()
	want := tt.trades + 1
	if tt.decodeErr {
		want++
	}
	if len(got) != want {
		t.Fatalf("elements = %+v, want %d", got, want)
	}
	for _, it := range got[:tt.trades] {
		if it.err != nil {
			t.Errorf("a trade came with an error: %v", it.err)
		}
	}
	if tt.decodeErr {
		if err := got[tt.trades].err; err == nil || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, ErrStreamClosed) {
			t.Errorf("decoding error = %v, want the real decoding error", err)
		}
	}
	end := got[len(got)-1].err
	if errors.Is(end, io.ErrUnexpectedEOF) != tt.truncated {
		t.Errorf("end = %v, truncation wanted: %v", end, tt.truncated)
	}
	if tt.alsoClosed && !errors.Is(end, ErrStreamClosed) {
		t.Errorf("end = %v, want it to match ErrStreamClosed", end)
	}
	if tt.truncated && strings.Contains(end.Error(), "decoding") {
		t.Errorf("end reads like a decoding failure: %v", end)
	}
}

// TestFinalLineWithoutNewline covers a stream whose last line has no newline: it is delivered when it decodes, keeps
// its own decoding error when it is complete JSON of the wrong shape, and is reported as a truncation only when its
// JSON is incomplete.
func TestFinalLineWithoutNewline(t *testing.T) {
	cutMidLine := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(tradeLine + `{"event":"trade","orderId":"o2","tim`))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	})
	tests := []finalLineCase{
		{"complete and valid", reply(200, tradeLine+strings.TrimSuffix(tradeLine, "\n")), 2, false, false, true},
		{"complete with a bad field", reply(200, tradeLine+`{"event":"trade","orderId":["not","a","string"]}`), 1, true, false, true},
		{"half an object", reply(200, tradeLine+`{"event":"trade","orderId":"o2","tim`), 1, false, true, true},
		{"connection cut inside the line", cutMidLine, 1, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, collect(newTestClient(t, tt.handler).TradeStream(context.Background())))
		})
	}
}
