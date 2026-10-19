// Package exante is a typed, context-first client for the Exante HTTP API: market data and account data under /md and
// orders under /trade.
//
// # Quick start
//
//	client, err := exante.New("https://api-live.exante.eu", exante.Credentials{
//		ClientID:      "469a8180-51fb-408f-a1f0-c3775eeb6ade",
//		ApplicationID: "77b378e8-3a30-4f85-9017-e839501f7589",
//		SharedKey:     "shared secret",
//	})
//	if err != nil {
//		return err
//	}
//	defer client.CloseIdleConnections()
//
//	accounts, err := client.Accounts(ctx)
//	if err != nil {
//		return err
//	}
//	for _, acc := range accounts {
//		summary, err := client.Summary(ctx, acc.AccountID, "EUR")
//		if err != nil {
//			return err
//		}
//		fmt.Println(acc.AccountID, summary.NetAssetValue)
//	}
//
// # Authentication and User-Agent
//
// The client signs an HS256 JWT from [Credentials] and sends it as a Bearer token. The token is cached inside the
// client, never shared with another client, and renewed when a third of its lifetime is left ([WithTokenTTL],
// [WithAudience]). Every request carries a User-Agent header, because the API edge answers the default Go user agent
// with 403 "error code: 1010"; see [WithUserAgent]. Credentials print redacted with %v, %+v, %#v and log/slog, and
// a [Client] prints neither them nor its token.
//
// # API versions
//
// Callers never pass a version. Every method uses the stable 3.0 API, except [Client.NearestGroupSymbol], whose
// endpoint exists in 2.0 only. Each method documents the path it calls.
//
// # Transport
//
// The base URL must use https; http is accepted for the loopback hosts 127.0.0.1, ::1 and localhost only. The client
// never follows redirects, because a redirect would turn a POST into a GET and report success for an order that was
// never placed: a 3xx answer is an *[APIError]. A caller-supplied http.Client ([WithHTTPClient]) is copied and its
// CheckRedirect replaced; the original is not modified. Each call is bounded by [WithTimeout] (default 60 s), response
// bodies are always closed and limited by [WithMaxBodySize] (default 64 MiB), and a request URL over 2000 characters
// fails with [ErrURLTooLong].
//
// # Errors
//
// A non-2xx answer is an *[APIError]. Programming errors wrap the sentinels [ErrNilContext], [ErrEmptyArgument],
// [ErrInvalidArgument], [ErrBadDate] and [ErrInvalidConfig]; match them with errors.Is. A context cancellation or
// deadline surfaces as an error wrapping the context error.
//
// # Placing and changing orders
//
// The client never retries a request. [Client.PlaceOrder], [Client.CancelOrder] and [Client.ReplaceOrder] return an
// error that wraps [ErrOutcomeUnknown] when it happened after the request may have reached the server: a transport
// error, a timeout or cancellation, an unreadable answer or a 5xx status. The order may exist or have changed, so
// never repeat the call blindly; reconcile with [Client.Orders] or [Client.Order] first (a ClientTag helps). Errors
// raised before anything was sent (validation, token signing, URL building) and 4xx answers mean nothing happened.
//
// # Numbers and lists
//
// Prices, quantities and amounts are [Decimal] values holding the exact text of the API; timestamps in milliseconds
// are [Millis]; whole numbers are [Int64]. The API documents numbers as strings but may send either, and both decode;
// an integer is evaluated exactly and must lie in the int64 range.
//
// A list method fails on a 200 answer that is not a JSON array (null and an empty body give an empty list). Only the
// operations whose spec answer is a oneOf ([Client.GroupSymbols], [Client.ExchangeSymbols], [Client.DailyChange],
// [Client.LastQuotes], [Client.Transactions]) also accept a single object, and never one that carries none of the
// known fields.
//
// # Streams
//
// [Client.OrderStream], [Client.TradeStream], [Client.QuoteFeed] and [Client.TradeFeed] return iter.Seq2 sequences,
// read with a range loop:
//
//	for update, err := range client.OrderStream(ctx) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(update.Order.OrderID, update.Order.State.Status)
//	}
//
// A stream connects when the loop starts and reads in the loop's own goroutine; it starts none. It ends when
//
//   - the loop breaks: the connection is closed;
//   - the context is cancelled or its deadline passes: the final element carries an error wrapping the context error;
//   - the server answers with a non-2xx status: the only element is an *[APIError];
//   - the connection fails, the server closes it ([ErrStreamClosed]), it stays silent past [WithStreamIdleTimeout]
//     ([ErrStreamIdle]) or a line exceeds [WithMaxBodySize] ([ErrResponseTooLarge]): the final element carries that
//     error.
//
// A line that is not valid JSON of the expected shape is delivered as an error element and the stream continues
// with the next line; the loop decides whether to stop. A final line cut off by the end of the connection ends the
// stream with an error that also wraps [io.ErrUnexpectedEOF]. The idle limit measures only the wait for the next
// line, never the time the loop body takes. Heartbeats, blank lines and events of kinds the spec does not describe
// are consumed silently. A stream is not resumed: after it ends, start a new one and reconcile state.
package exante
