package exante

import "time"

// Side is the direction of an order.
type Side string

// Order sides.
const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// OrderType is the execution style of an order.
type OrderType string

// Order types listed by the API.
const (
	OrderMarket       OrderType = "market"
	OrderLimit        OrderType = "limit"
	OrderStop         OrderType = "stop"
	OrderStopLimit    OrderType = "stop_limit"
	OrderTWAP         OrderType = "twap"
	OrderTrailingStop OrderType = "trailing_stop"
	OrderIceberg      OrderType = "iceberg"
)

// OrderDuration is the time in force of an order.
type OrderDuration string

// Order durations listed by the API.
const (
	DurationDay               OrderDuration = "day"
	DurationFillOrKill        OrderDuration = "fill_or_kill"
	DurationImmediateOrCancel OrderDuration = "immediate_or_cancel"
	DurationGoodTillCancel    OrderDuration = "good_till_cancel"
	DurationGoodTillTime      OrderDuration = "good_till_time"
	DurationAtTheOpening      OrderDuration = "at_the_opening"
	DurationAtTheClose        OrderDuration = "at_the_close"
)

// OrderStatus is the state of an order.
type OrderStatus string

// Order statuses listed by the API.
const (
	StatusPlacing   OrderStatus = "placing"
	StatusWorking   OrderStatus = "working"
	StatusCancelled OrderStatus = "cancelled"
	StatusPending   OrderStatus = "pending"
	StatusFilled    OrderStatus = "filled"
	StatusRejected  OrderStatus = "rejected"
)

// ModifyAction is what a modification request does to an order.
type ModifyAction string

// Modification actions listed by the API.
const (
	ActionReplace ModifyAction = "replace"
	ActionCancel  ModifyAction = "cancel"
)

// OrderFill is one execution of an order.
type OrderFill struct {
	Timestamp time.Time `json:"timestamp"`
	Quantity  Decimal   `json:"quantity"`
	Price     Decimal   `json:"price"`
	Position  Int64     `json:"position"`
}

// OrderState is the current state of an order.
type OrderState struct {
	Status     OrderStatus `json:"status"`
	LastUpdate time.Time   `json:"lastUpdate"`
	Fills      []OrderFill `json:"fills"`
	// Reason explains a rejection.
	Reason string `json:"reason"`
}

// OrderParameters are the parameters an order was placed with.
type OrderParameters struct {
	Side           Side          `json:"side"`
	Quantity       Decimal       `json:"quantity"`
	OCOGroup       string        `json:"ocoGroup"`
	IfDoneParentID string        `json:"ifDoneParentId"`
	Duration       OrderDuration `json:"duration"`
	OrderType      OrderType     `json:"orderType"`
	StopPrice      Decimal       `json:"stopPrice"`
	LimitPrice     Decimal       `json:"limitPrice"`
	PartQuantity   Decimal       `json:"partQuantity"`
	PlaceInterval  Int64         `json:"placeInterval"`
	PriceDistance  Decimal       `json:"priceDistance"`
	GTTExpiration  time.Time     `json:"gttExpiration"`
	SymbolID       string        `json:"symbolId"`
}

// Order is a trading order with its state.
type Order struct {
	OrderID               string          `json:"orderId"`
	PlaceTime             time.Time       `json:"placeTime"`
	CurrentModificationID string          `json:"currentModificationId"`
	AccountID             string          `json:"accountId"`
	Username              string          `json:"username"`
	ClientTag             string          `json:"clientTag"`
	State                 OrderState      `json:"orderState"`
	Parameters            OrderParameters `json:"orderParameters"`
}

// PlaceParams are the parameters of a new order. Side, Quantity, OrderType, Duration and SymbolID are required. The
// spec marks every other field "if applicable" and names no combination as mandatory, so the client checks nothing
// beyond the required fields and leaves the rest to the server.
type PlaceParams struct {
	AccountID     string        `json:"accountId,omitempty"`
	SymbolID      string        `json:"symbolId"`
	Side          Side          `json:"side"`
	Quantity      Decimal       `json:"quantity"`
	OrderType     OrderType     `json:"orderType"`
	Duration      OrderDuration `json:"duration"`
	LimitPrice    Decimal       `json:"limitPrice,omitempty"`
	StopPrice     Decimal       `json:"stopPrice,omitempty"`
	PartQuantity  Decimal       `json:"partQuantity,omitempty"`
	PlaceInterval Decimal       `json:"placeInterval,omitempty"`
	PriceDistance Decimal       `json:"priceDistance,omitempty"`
	// GTTExpiration is the expiration of a good-till-time order. It is sent in UTC with millisecond precision, like the
	// spec's example 2017-08-14T02:40:00.000Z; the zero time omits the field.
	GTTExpiration  time.Time `json:"gttExpiration,omitzero"`
	ClientTag      string    `json:"clientTag,omitempty"`
	TakeProfit     Decimal   `json:"takeProfit,omitempty"`
	StopLoss       Decimal   `json:"stopLoss,omitempty"`
	OCOGroup       string    `json:"ocoGroup,omitempty"`
	IfDoneParentID string    `json:"ifDoneParentId,omitempty"`
}

// placeWire is the request body of a new order: the parameters with GTTExpiration in the wire format.
type placeWire struct {
	*PlaceParams
	GTTExpiration string `json:"gttExpiration,omitempty"`
}

func (p *PlaceParams) wire() placeWire {
	w := placeWire{PlaceParams: p}
	if !p.GTTExpiration.IsZero() {
		w.GTTExpiration = formatMillisTime(p.GTTExpiration)
	}
	return w
}

// ReplaceParams are the new values of an order being replaced. Quantity is required.
type ReplaceParams struct {
	Quantity      Decimal `json:"quantity"`
	LimitPrice    Decimal `json:"limitPrice,omitempty"`
	StopPrice     Decimal `json:"stopPrice,omitempty"`
	PriceDistance Decimal `json:"priceDistance,omitempty"`
}

// modifyParams is the body of a modification request.
type modifyParams struct {
	Action     ModifyAction   `json:"action"`
	Parameters *ReplaceParams `json:"parameters,omitempty"`
}

// EstimateItem is one order of a margin estimate. SymbolID, Side and Quantity are required.
type EstimateItem struct {
	SymbolID   string  `json:"symbolId"`
	Side       Side    `json:"side"`
	Quantity   Decimal `json:"quantity"`
	LimitPrice Decimal `json:"limitPrice,omitempty"`
}

// EstimateParams are the parameters of a margin and commission estimate. All fields are required.
type EstimateParams struct {
	AccountID string         `json:"accountId"`
	Currency  string         `json:"currency"`
	Orders    []EstimateItem `json:"orders"`
}

// MarginEstimate is the margin and commission the account would have if the estimated orders were placed.
type MarginEstimate struct {
	Currency string `json:"currency"`
	// Commission is empty when the risk service does not report it.
	Commission Decimal `json:"commission"`
	// UsedPrice is reported for a single-order estimate only.
	UsedPrice         Decimal `json:"usedPrice"`
	Margin            Decimal `json:"margin"`
	MarginDelta       Decimal `json:"marginDelta"`
	MarginCheckPassed bool    `json:"marginCheckPassed"`
}

// OrderUpdate is an event of the order stream.
type OrderUpdate struct {
	Event string `json:"event"`
	Order Order  `json:"order"`
}

// Trade is an event of the trade stream: one execution of an order.
type Trade struct {
	Event     string    `json:"event"`
	OrderID   string    `json:"orderId"`
	Timestamp time.Time `json:"timestamp"`
	Quantity  Decimal   `json:"quantity"`
	Price     Decimal   `json:"price"`
	Position  Int64     `json:"position"`
}
