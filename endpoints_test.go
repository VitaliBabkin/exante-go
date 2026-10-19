package exante

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// supersededOperations are spec operations the client does not expose: their 3.0 twins are used.
var supersededOperations = []string{
	"GET /md/2.0/ohlc/{symbolId}/{duration}",
	"GET /md/2.0/ticks/{symbolId}",
}

const orderUUID = "d642d2ca-fcb5-4910-9de4-7c91f275ca23"

// endpoint is one client method with the request it must produce.
type endpoint struct {
	name   string
	spec   string
	run    func(ctx context.Context, c *Client) (any, error)
	method string
	uri    string
	body   string // expected JSON request body; empty for none
	answer string // fixture file name, or a key of small_answers.json
	status int
	check  func(t *testing.T, got any)
}

func endpoints() []endpoint {
	return slices.Concat(symbolEndpoints(), referenceEndpoints(), historyEndpoints(), accountEndpoints(),
		orderReadEndpoints(), orderWriteEndpoints())
}

func symbolEndpoints() []endpoint {
	return []endpoint{
		{name: "Groups", spec: "GET /md/{version}/groups", method: "GET", uri: "/md/3.0/groups", answer: "groups",
			run: func(ctx context.Context, c *Client) (any, error) { return c.Groups(ctx) }, check: checkGroups},
		{name: "GroupSymbols", spec: "GET /md/{version}/groups/{groupId}", method: "GET", uri: "/md/3.0/groups/NG",
			answer: "group_symbols.json", check: checkGroupSymbols,
			run: func(ctx context.Context, c *Client) (any, error) { return c.GroupSymbols(ctx, "NG") }},
		{name: "NearestGroupSymbol", spec: "GET /md/{version}/groups/{groupId}/nearest", method: "GET",
			uri: "/md/2.0/groups/NG/nearest", answer: "symbol_nearest_v2.json", check: checkNearest,
			run: func(ctx context.Context, c *Client) (any, error) { return c.NearestGroupSymbol(ctx, "NG") }},
		{name: "Exchanges", spec: "GET /md/{version}/exchanges", method: "GET", uri: "/md/3.0/exchanges",
			answer: "exchanges", check: checkExchanges,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Exchanges(ctx) }},
		{name: "ExchangeSymbols", spec: "GET /md/{version}/exchanges/{exchangeId}", method: "GET",
			uri: "/md/3.0/exchanges/NASDAQ", answer: "group_symbols.json",
			run: func(ctx context.Context, c *Client) (any, error) { return c.ExchangeSymbols(ctx, "NASDAQ") }},
		{name: "Symbol", spec: "GET /md/{version}/symbols/{symbolId}", method: "GET",
			uri: "/md/3.0/symbols/AAPL.NASDAQ", answer: "symbol.json", check: checkSymbol,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Symbol(ctx, "AAPL.NASDAQ") }},
		{name: "Schedule", spec: "GET /md/{version}/symbols/{symbolId}/schedule", method: "GET",
			uri: "/md/3.0/symbols/AAPL.NASDAQ/schedule", answer: "schedule.json", check: checkSchedule,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Schedule(ctx, "AAPL.NASDAQ", false) }},
		{name: "Schedule with order types", spec: "GET /md/{version}/symbols/{symbolId}/schedule", method: "GET",
			uri: "/md/3.0/symbols/AAPL.NASDAQ/schedule?types=true", answer: "schedule.json",
			run: func(ctx context.Context, c *Client) (any, error) { return c.Schedule(ctx, "AAPL.NASDAQ", true) }},
		{name: "Specification", spec: "GET /md/{version}/symbols/{symbolId}/specification", method: "GET",
			uri: "/md/3.0/symbols/AAPL.NASDAQ/specification", answer: "specification", check: checkSpecification,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Specification(ctx, "AAPL.NASDAQ") }},
	}
}

func referenceEndpoints() []endpoint {
	return []endpoint{
		{name: "Types", spec: "GET /md/{version}/types", method: "GET", uri: "/md/3.0/types", answer: "types",
			check: checkTypes, run: func(ctx context.Context, c *Client) (any, error) { return c.Types(ctx) }},
		{name: "Currencies", spec: "GET /md/{version}/crossrates", method: "GET", uri: "/md/3.0/crossrates",
			answer: "currencies", check: checkCurrencies,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Currencies(ctx) }},
		{name: "DetailedCurrencies", spec: "GET /md/{version}/symbols/currencies", method: "GET",
			uri: "/md/3.0/symbols/currencies", answer: "detailedCurrencies", check: checkDetailedCurrencies,
			run: func(ctx context.Context, c *Client) (any, error) { return c.DetailedCurrencies(ctx) }},
		{name: "Crossrate", spec: "GET /md/{version}/crossrates/{from}/{to}", method: "GET",
			uri: "/md/3.0/crossrates/EUR/USD", answer: "crossrate", check: checkCrossrate,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Crossrate(ctx, "EUR", "USD") }},
		{name: "DailyChange", spec: "GET /md/{version}/change/{symbolId}", method: "GET",
			uri: "/md/3.0/change/AAPL.NASDAQ,MSFT.NASDAQ", answer: "change", check: checkChange,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.DailyChange(ctx, "AAPL.NASDAQ", "MSFT.NASDAQ")
			}},
		{name: "LastQuotes", spec: "GET /md/{version}/feed/{symbolIds}/last", method: "GET",
			uri: "/md/3.0/feed/AAPL.NASDAQ,NOPE.NASDAQ/last", answer: "quotes_last.json", check: checkLastQuotes,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.LastQuotes(ctx, "AAPL.NASDAQ", "NOPE.NASDAQ")
			}},
	}
}

func historyEndpoints() []endpoint {
	return []endpoint{
		{name: "OHLC", spec: "GET /md/3.0/ohlc/{symbolId}/{duration}", method: "GET",
			uri:    "/md/3.0/ohlc/AAPL.NASDAQ/3600?from=1481565600000&size=60&to=1481572800000&type=trades",
			answer: "candles.json", check: checkCandles,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.OHLC(ctx, "AAPL.NASDAQ", time.Hour, &HistoryQuery{
					From: time.UnixMilli(1481565600000), To: time.UnixMilli(1481572800000), Size: 60, Type: TickTrades,
				})
			}},
		{name: "OHLC without query", spec: "GET /md/3.0/ohlc/{symbolId}/{duration}", method: "GET",
			uri: "/md/3.0/ohlc/EUR%2FUSD.E.FX/86400", answer: "candles.json",
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.OHLC(ctx, "EUR/USD.E.FX", 24*time.Hour, nil)
			}},
		{name: "Ticks", spec: "GET /md/3.0/ticks/{symbolId}", method: "GET",
			uri: "/md/3.0/ticks/AAPL.NASDAQ?size=1000&type=quotes", answer: "ticks.json", check: checkTicks,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.Ticks(ctx, "AAPL.NASDAQ", &HistoryQuery{Size: 1000, Type: TickQuotes})
			}},
		{name: "Ticks with bounds", spec: "GET /md/3.0/ticks/{symbolId}", method: "GET",
			uri: "/md/3.0/ticks/AAPL.NASDAQ?from=1481565600000&to=1481572800000", answer: "ticks.json",
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.Ticks(ctx, "AAPL.NASDAQ", &HistoryQuery{
					From: time.UnixMilli(1481565600000), To: time.UnixMilli(1481572800000),
				})
			}},
	}
}

func accountEndpoints() []endpoint {
	return []endpoint{
		{name: "Accounts", spec: "GET /md/{version}/accounts", method: "GET", uri: "/md/3.0/accounts",
			answer: "accounts", check: checkAccounts,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Accounts(ctx) }},
		{name: "Summary", spec: "GET /md/{version}/summary/{id}/{currency}", method: "GET",
			uri: "/md/3.0/summary/ABC1234.001/EUR", answer: "summary.json", check: checkSummary,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Summary(ctx, "ABC1234.001", "EUR") }},
		{name: "SummaryOn", spec: "GET /md/{version}/summary/{id}/{date}/{currency}", method: "GET",
			uri: "/md/3.0/summary/ABC1234.001/2013-02-16/EUR", answer: "summary.json",
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.SummaryOn(ctx, "ABC1234.001", "2013-02-16", "EUR")
			}},
		{name: "Transactions", spec: "GET /md/{version}/transactions", method: "GET",
			uri: "/md/3.0/transactions?accountId=ABC1234.001&fromDate=2023-01-01T00%3A05%3A00%2B02%3A00" +
				"&limit=10&operationType=TRADE%2CCOMMISSION&order=DESC&orderPos=0",
			answer: "transactions.json", check: checkTransactions,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.Transactions(ctx, &TransactionsQuery{
					AccountID:      "ABC1234.001",
					OperationTypes: []string{"TRADE", "COMMISSION"},
					Limit:          10,
					Order:          SortDescending,
					From:           time.Date(2023, 1, 1, 0, 5, 0, 0, time.FixedZone("EET", 2*3600)),
					OrderPos:       new(0),
				})
			}},
		{name: "Transactions without filter", spec: "GET /md/{version}/transactions", method: "GET",
			uri: "/md/3.0/transactions", answer: "transactions.json",
			run: func(ctx context.Context, c *Client) (any, error) { return c.Transactions(ctx, nil) }},
		{name: "Transactions with every filter", spec: "GET /md/{version}/transactions", method: "GET",
			uri: "/md/3.0/transactions?accountId=ABC1234.001&asset=EUR&fromDate=2023-01-01T00%3A05%3A00%2B02%3A00" +
				"&limit=10&offset=5&operationType=TRADE&order=ASC&orderId=ord-1&orderPos=3&symbolId=AAPL.NASDAQ" +
				"&toDate=2023-02-01T10%3A00%3A00%2B00%3A00&uuid=u-1",
			answer: "transactions.json",
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.Transactions(ctx, &TransactionsQuery{
					UUID: "u-1", AccountID: "ABC1234.001", SymbolID: "AAPL.NASDAQ", Asset: "EUR",
					OperationTypes: []string{"TRADE"}, Offset: 5, Limit: 10, Order: SortAscending,
					From:    time.Date(2023, 1, 1, 0, 5, 0, 0, time.FixedZone("EET", 2*3600)),
					To:      time.Date(2023, 2, 1, 10, 0, 0, 0, time.UTC),
					OrderID: "ord-1", OrderPos: new(3),
				})
			}},
	}
}

func orderReadEndpoints() []endpoint {
	return []endpoint{
		{name: "Orders", spec: "GET /trade/{version}/orders", method: "GET",
			uri: "/trade/3.0/orders?accountId=ABC1234.001&from=2017-05-18T10%3A00%3A00.000Z&limit=10" +
				"&to=2017-05-21T17%3A59%3A59.999Z",
			answer: "orders.json", check: checkOrders,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.Orders(ctx, &OrdersQuery{
					AccountID: "ABC1234.001", Limit: 10,
					From: time.Date(2017, 5, 18, 10, 0, 0, 0, time.UTC),
					To:   time.Date(2017, 5, 21, 17, 59, 59, 999_000_000, time.UTC),
				})
			}},
		{name: "Orders without filter", spec: "GET /trade/{version}/orders", method: "GET",
			uri: "/trade/3.0/orders", answer: "orders.json",
			run: func(ctx context.Context, c *Client) (any, error) { return c.Orders(ctx, nil) }},
		{name: "ActiveOrders", spec: "GET /trade/{version}/orders/active", method: "GET",
			uri:    "/trade/3.0/orders/active?accountId=ABC1234.001&limit=5&symbolId=AAPL.NASDAQ",
			answer: "orders.json",
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.ActiveOrders(ctx, &ActiveOrdersQuery{AccountID: "ABC1234.001", SymbolID: "AAPL.NASDAQ", Limit: 5})
			}},
		{name: "Order", spec: "GET /trade/{version}/orders/{orderId}", method: "GET",
			uri: "/trade/3.0/orders/" + orderUUID, answer: "order.json", check: checkOrder,
			run: func(ctx context.Context, c *Client) (any, error) { return c.Order(ctx, orderUUID) }},
	}
}

func orderWriteEndpoints() []endpoint {
	return []endpoint{
		{name: "PlaceOrder", spec: "POST /trade/{version}/orders", method: "POST", uri: "/trade/3.0/orders",
			status: http.StatusCreated, answer: "orders.json",
			body: `{"accountId":"ABC1234.001","symbolId":"AAPL.NASDAQ","side":"buy","quantity":"6","orderType":"limit",` +
				`"duration":"good_till_time","limitPrice":"120.5","gttExpiration":"2017-08-14T02:40:00.000Z",` +
				`"clientTag":"tag","takeProfit":"130.0","stopLoss":"90.0"}`,
			check: func(t *testing.T, got any) {
				if o := got.([]Order); len(o) != 2 {
					t.Errorf("orders = %+v", o)
				}
			},
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.PlaceOrder(ctx, &PlaceParams{
					AccountID: "ABC1234.001", SymbolID: "AAPL.NASDAQ", Side: Buy, Quantity: "6", OrderType: OrderLimit,
					Duration: DurationGoodTillTime, LimitPrice: "120.5", ClientTag: "tag", TakeProfit: "130.0", StopLoss: "90.0",
					GTTExpiration: time.Date(2017, 8, 14, 2, 40, 0, 0, time.UTC),
				})
			}},
		{name: "PlaceOrder with every optional field", spec: "POST /trade/{version}/orders", method: "POST",
			uri: "/trade/3.0/orders", status: http.StatusCreated, answer: "orders.json",
			body: `{"accountId":"ABC1234.001","symbolId":"AAPL.NASDAQ","side":"sell","quantity":"6","orderType":"twap",` +
				`"duration":"good_till_time","limitPrice":"120.5","stopPrice":"119.0","partQuantity":"2",` +
				`"placeInterval":"30","priceDistance":"1.5","gttExpiration":"2017-08-14T02:40:00.123Z",` +
				`"clientTag":"tag","takeProfit":"130.0","stopLoss":"90.0",` +
				`"ocoGroup":"d440b5b6-a40f-44e5-8c3b-a9a419fea7b3","ifDoneParentId":"3a5bf47e-ec54-4782-b4e3-0091164c7c71"}`,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.PlaceOrder(ctx, &PlaceParams{
					AccountID: "ABC1234.001", SymbolID: "AAPL.NASDAQ", Side: Sell, Quantity: "6", OrderType: OrderTWAP,
					Duration: DurationGoodTillTime, LimitPrice: "120.5", StopPrice: "119.0", PartQuantity: "2",
					PlaceInterval: "30", PriceDistance: "1.5", ClientTag: "tag", TakeProfit: "130.0", StopLoss: "90.0",
					OCOGroup: "d440b5b6-a40f-44e5-8c3b-a9a419fea7b3", IfDoneParentID: "3a5bf47e-ec54-4782-b4e3-0091164c7c71",
					// a non-UTC zone and sub-millisecond digits: the wire form is UTC with milliseconds
					GTTExpiration: time.Date(2017, 8, 14, 4, 40, 0, 123_456_789, time.FixedZone("EET", 2*3600)),
				})
			}},
		{name: "ReplaceOrder with every field", spec: "POST /trade/{version}/orders/{orderId}", method: "POST",
			uri: "/trade/3.0/orders/" + orderUUID, answer: "order.json",
			body: `{"action":"replace","parameters":{"quantity":"7","limitPrice":"101.0","stopPrice":"102.0",` +
				`"priceDistance":"1.0"}}`,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.ReplaceOrder(ctx, orderUUID, &ReplaceParams{
					Quantity: "7", LimitPrice: "101.0", StopPrice: "102.0", PriceDistance: "1.0",
				})
			}},
		{name: "CancelOrder", spec: "POST /trade/{version}/orders/{orderId}", method: "POST",
			uri: "/trade/3.0/orders/" + orderUUID, body: `{"action":"cancel"}`, answer: "order.json", check: checkOrder,
			run: func(ctx context.Context, c *Client) (any, error) { return c.CancelOrder(ctx, orderUUID) }},
		{name: "ReplaceOrder", spec: "POST /trade/{version}/orders/{orderId}", method: "POST",
			uri:  "/trade/3.0/orders/" + orderUUID,
			body: `{"action":"replace","parameters":{"quantity":"7","limitPrice":"101.0"}}`, answer: "order.json",
			check: checkOrder,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.ReplaceOrder(ctx, orderUUID, &ReplaceParams{Quantity: "7", LimitPrice: "101.0"})
			}},
		{name: "EstimateOrder", spec: "POST /trade/{version}/orders/estimate", method: "POST",
			uri: "/trade/3.0/orders/estimate", answer: "margin_estimate.json", check: checkEstimate,
			body: `{"accountId":"ABC1234.001","currency":"EUR","orders":[` +
				`{"symbolId":"AAPL.NASDAQ","side":"buy","quantity":"6","limitPrice":"120.0"},` +
				`{"symbolId":"MSFT.NASDAQ","side":"sell","quantity":"1"}]}`,
			run: func(ctx context.Context, c *Client) (any, error) {
				return c.EstimateOrder(ctx, &EstimateParams{
					AccountID: "ABC1234.001", Currency: "EUR",
					Orders: []EstimateItem{
						{SymbolID: "AAPL.NASDAQ", Side: Buy, Quantity: "6", LimitPrice: "120.0"},
						{SymbolID: "MSFT.NASDAQ", Side: Sell, Quantity: "1"},
					},
				})
			}},
	}
}

func checkGroups(t *testing.T, got any) {
	t.Helper()
	g := got.([]Group)
	if len(g) != 2 || g[0].ID != "NG" || g[0].Exchange != "NYMEX" {
		t.Fatalf("groups = %+v", g)
	}
	if !slices.Equal(g[0].Types, SymbolTypes{SymbolTypeFuture}) {
		t.Errorf("types as a string = %v", g[0].Types)
	}
	if !slices.Equal(g[1].Types, SymbolTypes{SymbolTypeStock, SymbolTypeOption}) {
		t.Errorf("types as an array = %v", g[1].Types)
	}
}

func checkGroupSymbols(t *testing.T, got any) {
	t.Helper()
	s := got.([]SymbolInfo)
	if len(s) != 2 {
		t.Fatalf("symbols = %+v", s)
	}
	if s[0].SymbolID != "NG.NYMEX.U2026" || s[0].MinPriceIncrement != "0.001" || s[0].Expiration.Time().Year() != 2026 {
		t.Errorf("future = %+v", s[0])
	}
	if o := s[1].OptionData; o == nil || o.StrikePrice != "30.5" || o.OptionRight != "PUT" {
		t.Errorf("option data = %+v", o)
	}
}

func checkNearest(t *testing.T, got any) {
	t.Helper()
	s := got.(SymbolInfo)
	if s.SymbolID != "NG.NYMEX.U2026" || s.SymbolType != SymbolTypeFuture || s.MinPriceIncrement != "0.001" ||
		s.Expiration != 1788134400000 {
		t.Errorf("symbol = %+v", s)
	}
	if o := s.OptionData; o == nil || o.OptionRight != "PUT" || o.StrikePrice != "3.5" {
		t.Errorf("option data = %+v", o)
	}
}

func checkExchanges(t *testing.T, got any) {
	t.Helper()
	if e := got.([]Exchange); len(e) != 2 || e[0].ID != "NASDAQ" || e[0].Country != "US" || e[1].Name != "" {
		t.Errorf("exchanges = %+v", e)
	}
}

func checkSymbol(t *testing.T, got any) {
	t.Helper()
	s := got.(SymbolInfo)
	if s.Ticker != "AAPL" || s.SymbolType != SymbolTypeStock || s.MinPriceIncrement != "0.01" {
		t.Errorf("symbol = %+v", s)
	}
	if id := s.Identifiers; id == nil || id.ISIN != "US0378331005" || id.SEDOL != "2046251" || id.CUSIP != "037833100" {
		t.Errorf("identifiers = %+v", id)
	}
}

func checkSchedule(t *testing.T, got any) {
	t.Helper()
	s := got.(Schedule)
	if len(s.Intervals) != 2 || s.Intervals[0].Name != SessionMainSession || s.Intervals[0].Period.Start != 1481533200000 {
		t.Fatalf("schedule = %+v", s)
	}
	want := []OrderDuration{DurationDay, DurationGoodTillCancel, DurationGoodTillTime}
	if !slices.Equal(s.Intervals[0].OrderTypes["limit"], want) || s.Intervals[1].OrderTypes != nil {
		t.Errorf("order types = %+v / %+v", s.Intervals[0].OrderTypes, s.Intervals[1].OrderTypes)
	}
}

func checkSpecification(t *testing.T, got any) {
	t.Helper()
	if s := got.(Specification); s.LotSize != "1.0" || s.Units != "Shares" {
		t.Errorf("specification = %+v", s)
	}
}

func checkTypes(t *testing.T, got any) {
	t.Helper()
	if ty := got.([]TypeDescriptor); len(ty) != 2 || ty[0].ID != SymbolTypeFuture {
		t.Errorf("types = %+v", ty)
	}
}

func checkCurrencies(t *testing.T, got any) {
	t.Helper()
	if cur := got.([]string); !slices.Equal(cur, []string{"EUR", "USD", "GBP"}) {
		t.Errorf("currencies = %v", cur)
	}
}

func checkDetailedCurrencies(t *testing.T, got any) {
	t.Helper()
	if cur := got.([]CurrencyInfo); len(cur) != 1 || cur[0].ID != "EUR" || cur[0].FractionDigits != 8 {
		t.Errorf("currencies = %+v", cur)
	}
}

func checkCrossrate(t *testing.T, got any) {
	t.Helper()
	if r := got.(Crossrate); r.Pair != "EUR/USD" || r.Rate != "1.2" || r.SymbolID != "EUR/USD.E.FX" {
		t.Errorf("crossrate = %+v", r)
	}
}

func checkChange(t *testing.T, got any) {
	t.Helper()
	if ch := got.([]DailyChange); len(ch) != 2 || ch[0].Change != "0.005" || !ch[1].Change.IsEmpty() {
		t.Errorf("changes = %+v", ch)
	}
}

func checkLastQuotes(t *testing.T, got any) {
	t.Helper()
	q := got.([]FeedEvent[Quote])
	if len(q) != 2 || q[0].Data == nil || q[0].Data.Bid[0].Price != "101.02" {
		t.Fatalf("quotes = %+v", q)
	}
	if q[1].Status == nil || q[1].Status.Event != SubscriptionStop {
		t.Errorf("status = %+v", q[1])
	}
}

func checkCandles(t *testing.T, got any) {
	t.Helper()
	cs := got.([]Candle)
	if len(cs) != 2 || cs[0].Open != "112.975" || !cs[0].Volume.IsEmpty() || cs[1].Volume != "3475756" {
		t.Errorf("candles = %+v", cs)
	}
}

func checkTicks(t *testing.T, got any) {
	t.Helper()
	ts := got.([]Tick)
	if len(ts) != 2 || ts[0].Ask[0].Price != "101.05" || ts[1].Price != "101.03" || ts[1].Size != "42" {
		t.Errorf("ticks = %+v", ts)
	}
}

func checkAccounts(t *testing.T, got any) {
	t.Helper()
	if a := got.([]AccountStatus); len(a) != 2 || a[0].Status != AccessFull || a[1].Status != AccessReadOnly {
		t.Errorf("accounts = %+v", a)
	}
}

func checkSummary(t *testing.T, got any) {
	t.Helper()
	s := got.(AccountSummary)
	if s.AccountID != "ABC1234.001" || s.NetAssetValue != "123456.01" || s.FreeMoney != "123406.01" ||
		s.Timestamp != 1503619200000 || s.SessionDate != "2013-02-16" {
		t.Errorf("summary = %+v", s)
	}
	if len(s.Currencies) != 2 || s.Currencies[1].Value != "100" {
		t.Errorf("currencies = %+v", s.Currencies)
	}
	if len(s.Positions) != 1 || s.Positions[0].AveragePrice != "110.0" || s.Positions[0].SymbolID != "AAPL.NASDAQ" {
		t.Errorf("positions = %+v", s.Positions)
	}
}

func checkTransactions(t *testing.T, got any) {
	t.Helper()
	tx := got.([]Transaction)
	if len(tx) != 2 {
		t.Fatalf("transactions = %+v", tx)
	}
	if tx[0].ID != 42 || tx[0].OrderPos != 2020 || tx[0].TransactionPrice != "123.45" || tx[0].Sum != "101.02" {
		t.Errorf("first = %+v", tx[0])
	}
	if tx[1].ID != 43 || tx[1].Timestamp != 1503619201000 || tx[1].Sum != "-0.5" || tx[1].OrderPos != 0 {
		t.Errorf("second (numbers and strings swapped) = %+v", tx[1])
	}
}

func checkOrders(t *testing.T, got any) {
	t.Helper()
	o := got.([]Order)
	if len(o) != 2 || o[0].State.Status != StatusFilled || o[1].State.Reason != "not enough money" {
		t.Errorf("orders = %+v", o)
	}
}

func checkOrder(t *testing.T, got any) {
	t.Helper()
	o := got.(Order)
	if o.OrderID != orderUUID || o.AccountID != "ABC1234.001" || o.PlaceTime.Year() != 2017 ||
		o.ClientTag != "some client tag" {
		t.Errorf("order = %+v", o)
	}
	if o.State.Status != StatusWorking || o.State.LastUpdate.Nanosecond() != 250_000_000 ||
		len(o.State.Fills) != 1 || o.State.Fills[0].Price != "120.1" || o.State.Fills[0].Position != 1 {
		t.Errorf("state = %+v", o.State)
	}
	p := o.Parameters
	if p.Side != Sell || p.OrderType != OrderLimit || p.Duration != DurationGoodTillTime || p.LimitPrice != "130.0" ||
		p.PlaceInterval != 1 || p.GTTExpiration.IsZero() || p.SymbolID != "AAPL.NASDAQ" {
		t.Errorf("parameters = %+v", p)
	}
}

func checkEstimate(t *testing.T, got any) {
	t.Helper()
	m := got.(MarginEstimate)
	if m.Margin != "1134202.8" || m.MarginDelta != "-2710.86" || m.Commission != "1.81" ||
		m.UsedPrice != "80.0" || !m.MarginCheckPassed || m.Currency != "EUR" {
		t.Errorf("estimate = %+v", m)
	}
}

// answerBody resolves the answer field: a fixture file name, or a key of small_answers.json.
func answerBody(t *testing.T, answer string) []byte {
	t.Helper()
	if strings.HasSuffix(answer, ".json") {
		return fixture(t, answer)
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(fixture(t, "small_answers.json"), &all); err != nil {
		t.Fatal(err)
	}
	raw, ok := all[answer]
	if !ok {
		t.Fatalf("small_answers.json has no key %q", answer)
	}
	return raw
}

// runEndpoint runs one endpoint case against a recording server and returns the request the server saw.
func runEndpoint(t *testing.T, ep endpoint) (seen, any) {
	t.Helper()
	rec := &recorder{status: cmp.Or(ep.status, http.StatusOK), body: answerBody(t, ep.answer)}
	c := newTestClient(t, rec)

	got, err := ep.run(context.Background(), c)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	return rec.only(t), got
}

func TestEndpoints(t *testing.T) {
	ops := loadOperations(t)
	for _, ep := range endpoints() {
		t.Run(ep.name, func(t *testing.T) {
			req, got := runEndpoint(t, ep)
			if req.Method != ep.method || req.URI != ep.uri {
				t.Errorf("request = %s %s, want %s %s", req.Method, req.URI, ep.method, ep.uri)
			}
			if op := operationOf(t, ops, req); op != ep.spec {
				t.Errorf("request matches the spec operation %q, the case claims %q", op, ep.spec)
			}
			assertBody(t, req.Body, ep.body)
			if ep.check != nil {
				ep.check(t, got)
			}
		})
	}
}

func assertBody(t *testing.T, got []byte, want string) {
	t.Helper()
	if want == "" {
		if len(got) != 0 {
			t.Errorf("request body = %s, want none", got)
		}
		return
	}
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("request body %q is not JSON: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("request body = %s, want %s", got, want)
	}
}

// loadOperations reads the operations of the spec, one "VERB /path" per line, from testdata/operations.txt.
func loadOperations(t *testing.T) []string {
	t.Helper()
	var ops []string
	for line := range strings.Lines(string(fixture(t, "operations.txt"))) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			ops = append(ops, line)
		}
	}
	return ops
}

// operationOf names the spec operation a request hits: the template of ops whose literal segments match, preferring
// the one with the most literals ("symbols/currencies" over "symbols/{symbolId}").
func operationOf(t *testing.T, ops []string, req seen) string {
	t.Helper()
	path, _, _ := strings.Cut(req.URI, "?")
	got := strings.Split(path, "/")
	best, bestLiterals := "", -1
	for _, op := range ops {
		verb, tmpl, _ := strings.Cut(op, " ")
		want := strings.Split(tmpl, "/")
		if verb != req.Method || len(want) != len(got) {
			continue
		}
		literals, ok := 0, true
		for i, seg := range want {
			switch {
			case seg == "{version}":
				ok = ok && (got[i] == "2.0" || got[i] == "3.0")
			case strings.HasPrefix(seg, "{"):
				ok = ok && got[i] != ""
			default:
				ok = ok && seg == got[i]
				literals++
			}
		}
		if ok && literals > bestLiterals {
			best, bestLiterals = op, literals
		}
	}
	if best == "" {
		t.Fatalf("request %s %s matches no spec operation", req.Method, req.URI)
	}
	return best
}

// streamCase is a stream method with the fixture it reads.
type streamCase struct {
	name    string
	fixture string
	run     func(ctx context.Context, c *Client)
}

func streamCases() []streamCase {
	return []streamCase{
		{"OrderStream", "order_stream.ndjson", func(ctx context.Context, c *Client) { collect(c.OrderStream(ctx)) }},
		{"TradeStream", "trade_stream.ndjson", func(ctx context.Context, c *Client) { collect(c.TradeStream(ctx)) }},
		{"QuoteFeed", "quote_feed.ndjson", func(ctx context.Context, c *Client) {
			collect(c.QuoteFeed(ctx, FeedBestPrice, "AAPL.NASDAQ"))
		}},
		{"TradeFeed", "trade_feed.ndjson", func(ctx context.Context, c *Client) {
			collect(c.TradeFeed(ctx, "AAPL.NASDAQ"))
		}},
	}
}

// TestEveryOperationIsCovered derives the spec operations from testdata/operations.txt and the operations actually hit
// from the requests the client sends, so a mislabelled case or a missing method fails.
func TestEveryOperationIsCovered(t *testing.T) {
	ops := loadOperations(t)
	if len(ops) != 32 {
		t.Fatalf("testdata/operations.txt lists %d operations, want the 32 of the spec", len(ops))
	}
	covered := map[string]bool{}
	for _, ep := range endpoints() {
		req, _ := runEndpoint(t, ep)
		covered[operationOf(t, ops, req)] = true
	}
	for _, sc := range streamCases() {
		rec := &recorder{status: http.StatusOK, body: fixture(t, sc.fixture)}
		sc.run(context.Background(), newTestClient(t, rec))
		covered[operationOf(t, ops, rec.only(t))] = true
	}
	for _, op := range supersededOperations {
		if !slices.Contains(ops, op) {
			t.Errorf("superseded operation %q is not in the spec", op)
		}
		covered[op] = true
	}
	for _, op := range ops {
		if !covered[op] {
			t.Errorf("spec operation %q is not exercised by any test", op)
		}
	}
}
