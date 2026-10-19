package exante

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// writeCalls are the state-changing methods, taken from the endpoint table so that their arguments pass validation.
func writeCalls() map[string]func(c *Client) error {
	calls := map[string]func(c *Client) error{}
	for _, ep := range orderWriteEndpoints() {
		if slices.Contains([]string{"PlaceOrder", "CancelOrder", "ReplaceOrder"}, ep.name) {
			calls[ep.name] = func(c *Client) error {
				_, err := ep.run(context.Background(), c)
				return err
			}
		}
	}
	return calls
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	} {
		for name, run := range writeCalls() {
			t.Run(http.StatusText(status)+"/"+name, func(t *testing.T) {
				// The in-memory network reaches the recorder under any host, so a followed redirect would show up as
				// a second request.
				rec := &recorder{status: status, location: "https://elsewhere.test/trade/3.0/orders/active"}
				c := newTestClient(t, rec)

				err := run(c)

				apiErr, ok := errors.AsType[*APIError](err)
				if !ok || apiErr.StatusCode != status {
					t.Fatalf("error = %v, want an *APIError with status %d", err, status)
				}
				if errors.Is(err, ErrOutcomeUnknown) {
					t.Errorf("a redirect answer is a definite answer, not an unknown outcome: %v", err)
				}
				reqs := rec.all()
				if len(reqs) != 1 || reqs[0].Method != http.MethodPost {
					t.Errorf("server saw %+v, want exactly the one original POST", reqs)
				}
			})
		}
	}
}

func TestRedirectOnReadIsAnError(t *testing.T) {
	rec := &recorder{status: http.StatusFound, location: "/md/3.0/types"}
	c := newTestClient(t, rec)

	_, err := c.Groups(context.Background())

	if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.StatusCode != http.StatusFound {
		t.Errorf("error = %v, want an *APIError with status 302", err)
	}
	if n := len(rec.all()); n != 1 {
		t.Errorf("server saw %d requests, want 1", n)
	}
}

func TestCallerHTTPClientIsNotModified(t *testing.T) {
	hc := httptest.NewTestServer(t, reply(200, `[]`)).Client()

	c := newClient(t, WithHTTPClient(hc))

	if hc.CheckRedirect != nil {
		t.Error("New changed the caller's http.Client")
	}
	if c.hc == hc || c.hc.CheckRedirect == nil {
		t.Error("the client does not use its own copy with a redirect policy")
	}
}
