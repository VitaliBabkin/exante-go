package exante

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors. Every error the package returns for a programming mistake wraps one of them, so callers match with
// [errors.Is].
var (
	// ErrNilContext is returned when a method receives a nil context.
	ErrNilContext = errors.New("exante: nil context")
	// ErrEmptyArgument is returned when a required argument is empty.
	ErrEmptyArgument = errors.New("exante: empty argument")
	// ErrInvalidArgument is returned when an argument is outside the set the API accepts.
	ErrInvalidArgument = errors.New("exante: invalid argument")
	// ErrBadDate is returned when a date argument is not a calendar date in the form YYYY-MM-DD.
	ErrBadDate = errors.New("exante: bad date")
	// ErrInvalidConfig is returned by [New] for an unusable base URL, credentials or option value.
	ErrInvalidConfig = errors.New("exante: invalid configuration")
	// ErrURLTooLong is returned when a request URL would exceed the 2000 characters the API accepts.
	ErrURLTooLong = errors.New("exante: request URL too long")
	// ErrResponseTooLarge is returned when a response body exceeds the limit set by [WithMaxBodySize].
	ErrResponseTooLarge = errors.New("exante: response body too large")
	// ErrOutcomeUnknown marks an error of a state-changing call ([Client.PlaceOrder], [Client.CancelOrder],
	// [Client.ReplaceOrder]) that happened after the request may have reached the server: a transport error, a timeout
	// or cancellation, an unreadable answer, or a 5xx status. The order may or may not have been changed. Reconcile with
	// [Client.Order] or [Client.Orders] (a ClientTag helps) before repeating the call. Errors raised before anything
	// was sent, and 4xx answers, do not carry it.
	ErrOutcomeUnknown = errors.New("exante: outcome unknown, the request may have been processed")
	// ErrStreamClosed ends a stream whose server closed the connection.
	ErrStreamClosed = errors.New("exante: stream closed by server")
	// ErrStreamIdle ends a stream that received no line within the limit set by [WithStreamIdleTimeout].
	ErrStreamIdle = errors.New("exante: stream idle")
)

// maxErrorBody is the largest part of an error response body kept in [APIError.Raw].
const maxErrorBody = 4 << 10

// ErrorGroup tells who caused an API error.
type ErrorGroup string

// Error groups described by the API.
const (
	ErrorGroupServer ErrorGroup = "server"
	ErrorGroupClient ErrorGroup = "client"
)

// ErrorBody is the JSON error object the trade endpoints answer with.
type ErrorBody struct {
	Group   ErrorGroup `json:"group"`
	Message string     `json:"message"`
}

// APIError is returned for every non-2xx answer. Match it with errors.As:
//
//	if apiErr, ok := errors.AsType[*exante.APIError](err); ok && apiErr.StatusCode == http.StatusNotFound { ... }
type APIError struct {
	// Method and Path identify the request; the query string is not included.
	Method string
	Path   string
	// StatusCode and Status are the HTTP status of the answer.
	StatusCode int
	Status     string
	// Body is the decoded error object, or nil when the answer is not the JSON error object the API describes.
	Body *ErrorBody
	// Raw is the answer body, truncated to 4 KiB.
	Raw []byte
	// RetryAfter is the delay the Retry-After header asks for; zero when the header is absent or unusable.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "exante: %s %s: %d %s", e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))
	switch {
	case e.Body != nil:
		fmt.Fprintf(&b, ": %s: %s", e.Body.Group, e.Body.Message)
	case len(e.Raw) > 0:
		fmt.Fprintf(&b, ": %s", strings.TrimSpace(string(e.Raw)))
	}
	return b.String()
}

// parseRetryAfter reads a Retry-After header holding delay-seconds or an HTTP date.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(min(secs, int64(math.MaxInt64/time.Second))) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0)
	}
	return 0
}
