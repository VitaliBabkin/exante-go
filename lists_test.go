package exante

import (
	"context"
	"testing"
)

// listOperation is a client method returning a list, with the length of the list it returned.
type listOperation struct {
	name string
	run  func(c *Client) (int, error)
}

func count[T any](items []T, err error) (int, error) { return len(items), err }

// strictListOperations are the list methods whose spec answer is a plain array.
func strictListOperations() []listOperation {
	ctx := context.Background()
	return []listOperation{
		{"Groups", func(c *Client) (int, error) { return count(c.Groups(ctx)) }},
		{"Exchanges", func(c *Client) (int, error) { return count(c.Exchanges(ctx)) }},
		{"Types", func(c *Client) (int, error) { return count(c.Types(ctx)) }},
		{"Accounts", func(c *Client) (int, error) { return count(c.Accounts(ctx)) }},
		{"Orders", func(c *Client) (int, error) { return count(c.Orders(ctx, nil)) }},
		{"ActiveOrders", func(c *Client) (int, error) { return count(c.ActiveOrders(ctx, nil)) }},
		{"OHLC", func(c *Client) (int, error) { return count(c.OHLC(ctx, "AAPL.NASDAQ", 60e9, nil)) }},
		{"Ticks", func(c *Client) (int, error) { return count(c.Ticks(ctx, "AAPL.NASDAQ", nil)) }},
	}
}

// oneOfListOperation is a list method whose spec answer is a oneOf of shapes, so the API may answer with a bare
// object; object is a minimal valid object of its element type.
type oneOfListOperation struct {
	listOperation
	object string
}

func oneOfListOperations() []oneOfListOperation {
	ctx := context.Background()
	return []oneOfListOperation{
		{listOperation{"GroupSymbols", func(c *Client) (int, error) { return count(c.GroupSymbols(ctx, "NG")) }},
			`{"symbolId":"NG.NYMEX.U2026"}`},
		{listOperation{"ExchangeSymbols", func(c *Client) (int, error) { return count(c.ExchangeSymbols(ctx, "NYMEX")) }},
			`{"symbolId":"NG.NYMEX.U2026"}`},
		{listOperation{"DailyChange", func(c *Client) (int, error) { return count(c.DailyChange(ctx, "AAPL.NASDAQ")) }},
			`{"symbolId":"AAPL.NASDAQ","dailyChange":"0.5"}`},
		{listOperation{"LastQuotes", func(c *Client) (int, error) { return count(c.LastQuotes(ctx, "AAPL.NASDAQ")) }},
			`{"symbolId":"AAPL.NASDAQ","timestamp":1,"bid":[],"ask":[]}`},
		{listOperation{"Transactions", func(c *Client) (int, error) { return count(c.Transactions(ctx, nil)) }},
			`{"id":1,"sum":"2"}`},
	}
}

const errorLookingObject = `{"message":"something odd"}`

func TestStrictListsRejectAnythingButAnArray(t *testing.T) {
	for _, op := range strictListOperations() {
		t.Run(op.name, func(t *testing.T) {
			for _, body := range []string{errorLookingObject, `"text"`, `7`, `true`} {
				c := newTestClient(t, reply(200, body))
				if n, err := op.run(c); err == nil {
					t.Errorf("answer %s decoded to %d elements without an error", body, n)
				}
			}
			for _, body := range []string{`[]`, `null`, ``, " \n"} {
				c := newTestClient(t, reply(200, body))
				if n, err := op.run(c); err != nil || n != 0 {
					t.Errorf("answer %q = %d elements, %v; want an empty list", body, n, err)
				}
			}
		})
	}
}

func TestOneOfListsAcceptAnObjectButNeverAPhantomElement(t *testing.T) {
	for _, op := range oneOfListOperations() {
		t.Run(op.name, func(t *testing.T) {
			if n, err := op.run(newTestClient(t, reply(200, op.object))); err != nil || n != 1 {
				t.Errorf("a single object = %d elements, %v; want 1", n, err)
			}
			if n, err := op.run(newTestClient(t, reply(200, errorLookingObject))); err == nil {
				t.Errorf("an unrelated object decoded to %d elements without an error", n)
			}
			for _, body := range []string{`[]`, `null`, ``} {
				if n, err := op.run(newTestClient(t, reply(200, body))); err != nil || n != 0 {
					t.Errorf("answer %q = %d elements, %v; want an empty list", body, n, err)
				}
			}
		})
	}
}

func TestLastQuotesDropsHeartbeats(t *testing.T) {
	body := `[{"symbolId":"AAPL.NASDAQ","timestamp":1,"bid":[],"ask":[]},{"event":"heartbeat"},` +
		`{"symbolId":"X","event":"subscription_stop","reason":"gone"}]`
	c := newTestClient(t, reply(200, body))

	got, err := c.LastQuotes(context.Background(), "AAPL.NASDAQ", "X")

	if err != nil || len(got) != 2 || got[0].Data == nil || got[1].Status == nil {
		t.Errorf("LastQuotes = %+v, %v; want a quote and a status without the heartbeat", got, err)
	}
}
