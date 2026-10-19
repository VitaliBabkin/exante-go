package exante

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// OrdersQuery filters [Client.Orders]. Zero fields are omitted.
type OrdersQuery struct {
	AccountID string
	// From and To bound the order time, with millisecond precision.
	From, To time.Time
	// Limit caps the number of orders.
	Limit int
}

// ActiveOrdersQuery filters [Client.ActiveOrders]. Zero fields are omitted.
type ActiveOrdersQuery struct {
	AccountID string
	SymbolID  string
	// Limit caps the number of orders.
	Limit int
}

func (q *OrdersQuery) values() url.Values {
	v := url.Values{}
	if q == nil {
		return v
	}
	setString(v, "accountId", q.AccountID)
	setTime(v, "from", q.From, formatMillisTime)
	setTime(v, "to", q.To, formatMillisTime)
	setPositive(v, "limit", q.Limit)
	return v
}

func (q *ActiveOrdersQuery) values() url.Values {
	v := url.Values{}
	if q == nil {
		return v
	}
	setString(v, "accountId", q.AccountID)
	setString(v, "symbolId", q.SymbolID)
	setPositive(v, "limit", q.Limit)
	return v
}

// Orders lists historical orders: GET /trade/3.0/orders. A nil query applies no filter.
func (c *Client) Orders(ctx context.Context, q *OrdersQuery) ([]Order, error) {
	return getAs[strictList[Order]](ctx, c, q.values(), tradePath+"/orders")
}

// ActiveOrders lists the active orders: GET /trade/3.0/orders/active. A nil query applies no filter.
func (c *Client) ActiveOrders(ctx context.Context, q *ActiveOrdersQuery) ([]Order, error) {
	return getAs[strictList[Order]](ctx, c, q.values(), tradePath+"/orders/active")
}

// Order returns one order: GET /trade/3.0/orders/{orderId}.
func (c *Client) Order(ctx context.Context, orderID string) (Order, error) {
	return getAs[Order](ctx, c, nil, tradePath+"/orders/%s", one(orderID))
}

// PlaceOrder places a new order: POST /trade/3.0/orders. It returns the created orders; a request with a take-profit
// or stop-loss creates more than one. The answer may be an array, a single object or empty; whatever the API accepted
// is never reported as a plain failure.
//
// The client never retries. Read [ErrOutcomeUnknown] before repeating a failed call: the order may exist, and
// reconciling with [Client.Orders] comes first.
func (c *Client) PlaceOrder(ctx context.Context, p *PlaceParams) ([]Order, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	return writeAs[list[Order]](ctx, c, p.wire(), tradePath+"/orders")
}

// CancelOrder cancels an order: POST /trade/3.0/orders/{orderId} with the action "cancel". It returns the order as
// the API accepted the modification. See [ErrOutcomeUnknown] for failures after the request was sent.
func (c *Client) CancelOrder(ctx context.Context, orderID string) (Order, error) {
	return writeAs[Order](ctx, c, modifyParams{Action: ActionCancel}, tradePath+"/orders/%s", one(orderID))
}

// ReplaceOrder replaces the quantity and prices of an order: POST /trade/3.0/orders/{orderId} with the action
// "replace". See [ErrOutcomeUnknown] for failures after the request was sent.
func (c *Client) ReplaceOrder(ctx context.Context, orderID string, p *ReplaceParams) (Order, error) {
	if p == nil || p.Quantity.IsEmpty() {
		return Order{}, fmt.Errorf("%w: replace quantity", ErrEmptyArgument)
	}
	body := modifyParams{Action: ActionReplace, Parameters: p}
	return writeAs[Order](ctx, c, body, tradePath+"/orders/%s", one(orderID))
}

// EstimateOrder returns the margin and commission the account would have if the given orders were placed:
// POST /trade/3.0/orders/estimate. No order is placed.
func (c *Client) EstimateOrder(ctx context.Context, p *EstimateParams) (MarginEstimate, error) {
	if err := p.validate(); err != nil {
		return MarginEstimate{}, err
	}
	return doAs[MarginEstimate](ctx, c, &call{method: http.MethodPost, tmpl: tradePath + "/orders/estimate", body: p})
}

func (p *PlaceParams) validate() error {
	switch {
	case p == nil:
		return fmt.Errorf("%w: order parameters", ErrEmptyArgument)
	case p.SymbolID == "":
		return fmt.Errorf("%w: order SymbolID", ErrEmptyArgument)
	case p.Side == "":
		return fmt.Errorf("%w: order Side", ErrEmptyArgument)
	case p.Quantity.IsEmpty():
		return fmt.Errorf("%w: order Quantity", ErrEmptyArgument)
	case p.OrderType == "":
		return fmt.Errorf("%w: order OrderType", ErrEmptyArgument)
	case p.Duration == "":
		return fmt.Errorf("%w: order Duration", ErrEmptyArgument)
	}
	return nil
}

func (p *EstimateParams) validate() error {
	switch {
	case p == nil:
		return fmt.Errorf("%w: estimate parameters", ErrEmptyArgument)
	case p.AccountID == "":
		return fmt.Errorf("%w: estimate AccountID", ErrEmptyArgument)
	case p.Currency == "":
		return fmt.Errorf("%w: estimate Currency", ErrEmptyArgument)
	case len(p.Orders) == 0:
		return fmt.Errorf("%w: estimate Orders", ErrEmptyArgument)
	}
	for i, o := range p.Orders {
		if o.SymbolID == "" || o.Side == "" || o.Quantity.IsEmpty() {
			return fmt.Errorf("%w: estimate order %d needs SymbolID, Side and Quantity", ErrEmptyArgument, i)
		}
	}
	return nil
}
