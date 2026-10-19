# exante-go

[![Go Reference](https://pkg.go.dev/badge/github.com/VitaliBabkin/exante-go.svg)](https://pkg.go.dev/github.com/VitaliBabkin/exante-go)
[![CI](https://github.com/VitaliBabkin/exante-go/actions/workflows/ci.yml/badge.svg)](https://github.com/VitaliBabkin/exante-go/actions/workflows/ci.yml)

A typed, context-first Go client for the Exante HTTP API: market data and account data under `/md`, orders and order
streams under `/trade`. Standard library only; Go 1.27.

The package documentation is the reference for the behaviour of the client: authentication, transport rules, errors,
the outcome of failed order writes, number and list decoding, streams. Read it with
`go doc github.com/VitaliBabkin/exante-go` or on pkg.go.dev.

## Install

```sh
go get github.com/VitaliBabkin/exante-go
```

## Quick start

```go
client, err := exante.New("https://api-live.exante.eu", exante.Credentials{
	ClientID:      "...",
	ApplicationID: "...",
	SharedKey:     "...",
})
if err != nil {
	return err
}
defer client.CloseIdleConnections()

accounts, err := client.Accounts(ctx)
```

## Examples

Market data and account data:

```go
symbol, err := client.Symbol(ctx, "AAPL.NASDAQ")
candles, err := client.OHLC(ctx, "AAPL.NASDAQ", time.Hour, &exante.HistoryQuery{Size: 100})
summary, err := client.Summary(ctx, "ABC1234.001", "EUR")
```

Orders. A write that fails after the request may have been sent wraps `ErrOutcomeUnknown`; reconcile with `Orders` or
`Order` before repeating it:

```go
orders, err := client.PlaceOrder(ctx, &exante.PlaceParams{
	AccountID: "ABC1234.001", SymbolID: "AAPL.NASDAQ", Side: exante.Buy, Quantity: "6",
	OrderType: exante.OrderLimit, Duration: exante.DurationDay, LimitPrice: "120.5", ClientTag: "batch-17",
})
if errors.Is(err, exante.ErrOutcomeUnknown) {
	// look the order up by its ClientTag before trying again
}
```

Streams (`iter.Seq2`):

```go
for update, err := range client.OrderStream(ctx) {
	if err != nil {
		return err
	}
	fmt.Println(update.Order.OrderID, update.Order.State.Status)
}
```

## Methods

Market data: `Groups`, `GroupSymbols`, `NearestGroupSymbol`, `Exchanges`, `ExchangeSymbols`, `Symbol`, `Schedule`,
`Specification`, `Types`, `Currencies`, `DetailedCurrencies`, `Crossrate`, `DailyChange`, `LastQuotes`, `OHLC`,
`Ticks`.

Accounts: `Accounts`, `Summary`, `SummaryOn`, `Transactions`.

Orders: `Orders`, `ActiveOrders`, `Order`, `PlaceOrder`, `CancelOrder`, `ReplaceOrder`, `EstimateOrder`.

Streams: `OrderStream`, `TradeStream`, `QuoteFeed`, `TradeFeed`.

Filters (`HistoryQuery`, `TransactionsQuery`, `OrdersQuery`, `ActiveOrdersQuery`) are passed by pointer; `nil` means no
filter. The old "all symbols" and "symbols by type" endpoints no longer exist; enumerate with `Groups`, `GroupSymbols`,
`Exchanges`, `ExchangeSymbols`.

## Tests

`go test ./...` runs against `httptest` servers on the in-memory network with `testing/synctest`; nothing touches the
live API. The client has been verified against the specification only, not against the live service.
