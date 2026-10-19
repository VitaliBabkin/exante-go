package exante

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// ohlcIntervals are the aggregation intervals the API accepts for candles.
var ohlcIntervals = []time.Duration{
	time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute,
	time.Hour, 4 * time.Hour, 6 * time.Hour, 24 * time.Hour,
}

// HistoryQuery narrows the history returned by [Client.OHLC] and [Client.Ticks]. Zero fields are omitted and the API
// applies its defaults; a nil query omits them all.
type HistoryQuery struct {
	// From and To bound the period, with millisecond precision.
	From, To time.Time
	// Size is the maximum number of candles or ticks.
	Size int
	// Type selects quotes or trades; the API defaults to quotes.
	Type TickType
}

func (q *HistoryQuery) values() url.Values {
	v := url.Values{}
	if q == nil {
		return v
	}
	setTime(v, "from", q.From, formatEpochMillis)
	setTime(v, "to", q.To, formatEpochMillis)
	setPositive(v, "size", q.Size)
	setString(v, "type", string(q.Type))
	return v
}

// Groups lists the instrument groups: GET /md/3.0/groups.
func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	return getAs[strictList[Group]](ctx, c, nil, mdPath+"/groups")
}

// GroupSymbols lists the instruments of a group: GET /md/3.0/groups/{groupId}.
func (c *Client) GroupSymbols(ctx context.Context, groupID string) ([]SymbolInfo, error) {
	return getAs[list[SymbolInfo]](ctx, c, nil, mdPath+"/groups/%s", one(groupID))
}

// NearestGroupSymbol returns the instrument of a group that expires first: GET /md/2.0/groups/{groupId}/nearest.
// The endpoint exists in API 2.0 only; the answer is converted to the [SymbolInfo] shape of 3.0.
func (c *Client) NearestGroupSymbol(ctx context.Context, groupID string) (SymbolInfo, error) {
	s, err := getAs[symbolInfoV2](ctx, c, nil, mdPathV2+"/groups/%s/nearest", one(groupID))
	if err != nil {
		return SymbolInfo{}, err
	}
	return s.info(), nil
}

// Exchanges lists the exchanges: GET /md/3.0/exchanges.
func (c *Client) Exchanges(ctx context.Context) ([]Exchange, error) {
	return getAs[strictList[Exchange]](ctx, c, nil, mdPath+"/exchanges")
}

// ExchangeSymbols lists the instruments of an exchange: GET /md/3.0/exchanges/{exchangeId}.
func (c *Client) ExchangeSymbols(ctx context.Context, exchangeID string) ([]SymbolInfo, error) {
	return getAs[list[SymbolInfo]](ctx, c, nil, mdPath+"/exchanges/%s", one(exchangeID))
}

// Symbol returns one instrument: GET /md/3.0/symbols/{symbolId}.
func (c *Client) Symbol(ctx context.Context, symbolID string) (SymbolInfo, error) {
	return getAs[SymbolInfo](ctx, c, nil, mdPath+"/symbols/%s", one(symbolID))
}

// Schedule returns the trading schedule of an instrument: GET /md/3.0/symbols/{symbolId}/schedule. With
// withOrderTypes each session also lists the order types and durations available in it.
func (c *Client) Schedule(ctx context.Context, symbolID string, withOrderTypes bool) (Schedule, error) {
	var query url.Values
	if withOrderTypes {
		query = url.Values{"types": {"true"}}
	}
	return getAs[Schedule](ctx, c, query, mdPath+"/symbols/%s/schedule", one(symbolID))
}

// Specification returns the trading parameters of an instrument: GET /md/3.0/symbols/{symbolId}/specification.
func (c *Client) Specification(ctx context.Context, symbolID string) (Specification, error) {
	return getAs[Specification](ctx, c, nil, mdPath+"/symbols/%s/specification", one(symbolID))
}

// Types lists the known instrument types: GET /md/3.0/types.
func (c *Client) Types(ctx context.Context) ([]TypeDescriptor, error) {
	return getAs[strictList[TypeDescriptor]](ctx, c, nil, mdPath+"/types")
}

// Currencies lists the currency codes the crossrates cover: GET /md/3.0/crossrates.
func (c *Client) Currencies(ctx context.Context) ([]string, error) {
	out, err := getAs[struct {
		Currencies []string `json:"currencies"`
	}](ctx, c, nil, mdPath+"/crossrates")
	return out.Currencies, err
}

// DetailedCurrencies lists the currencies with their fraction digits: GET /md/3.0/symbols/currencies.
func (c *Client) DetailedCurrencies(ctx context.Context) ([]CurrencyInfo, error) {
	out, err := getAs[struct {
		Currencies []CurrencyInfo `json:"currencies"`
	}](ctx, c, nil, mdPath+"/symbols/currencies")
	return out.Currencies, err
}

// Crossrate returns the rate from one currency to another: GET /md/3.0/crossrates/{from}/{to}.
func (c *Client) Crossrate(ctx context.Context, from, to string) (Crossrate, error) {
	return getAs[Crossrate](ctx, c, nil, mdPath+"/crossrates/%s/%s", one(from), one(to))
}

// DailyChange returns the daily price change of the given instruments: GET /md/3.0/change/{symbolIds}.
func (c *Client) DailyChange(ctx context.Context, symbolIDs ...string) ([]DailyChange, error) {
	return getAs[list[DailyChange]](ctx, c, nil, mdPath+"/change/%s", symbolIDs)
}

// LastQuotes returns the last quote of each given instrument: GET /md/3.0/feed/{symbolIds}/last. An instrument the
// feed cannot serve comes back as a [FeedEvent] with Status set.
func (c *Client) LastQuotes(ctx context.Context, symbolIDs ...string) ([]FeedEvent[Quote], error) {
	events, err := getAs[list[FeedEvent[Quote]]](ctx, c, nil, mdPath+"/feed/%s/last", symbolIDs)
	return slices.DeleteFunc(events, FeedEvent[Quote].empty), err
}

// OHLC returns candles of an instrument: GET /md/3.0/ohlc/{symbolId}/{duration}. The interval must be one the API
// offers: 1, 5, 10, 15, 30 minutes, or 1, 4, 6, 24 hours. The literal 3.0 path is used; the spec's 2.0 twin returns
// the same shape.
func (c *Client) OHLC(ctx context.Context, symbolID string, interval time.Duration, q *HistoryQuery) ([]Candle, error) {
	if !slices.Contains(ohlcIntervals, interval) {
		return nil, fmt.Errorf("%w: OHLC interval %s", ErrInvalidArgument, interval)
	}
	seconds := strconv.FormatInt(int64(interval/time.Second), 10)
	return getAs[strictList[Candle]](ctx, c, q.values(), mdPath+"/ohlc/%s/%s", one(symbolID), one(seconds))
}

// Ticks returns historical ticks of an instrument: GET /md/3.0/ticks/{symbolId}. The literal 3.0 path is used; the
// spec's 2.0 twin names the price fields differently.
func (c *Client) Ticks(ctx context.Context, symbolID string, q *HistoryQuery) ([]Tick, error) {
	return getAs[strictList[Tick]](ctx, c, q.values(), mdPath+"/ticks/%s", one(symbolID))
}
