package exante

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TransactionsQuery filters [Client.Transactions]. Zero fields are omitted.
type TransactionsQuery struct {
	UUID      string
	AccountID string
	SymbolID  string
	Asset     string
	// OperationTypes filters by transaction type, for example "TRADE".
	OperationTypes []string
	// Offset skips that many transactions; Limit caps the count.
	Offset, Limit int
	Order         SortOrder
	// From and To bound the transaction time.
	From, To time.Time
	OrderID  string
	// OrderPos filters by the position in the order; nil omits the filter (position 0 is a valid filter).
	OrderPos *int
}

func (q *TransactionsQuery) values() url.Values {
	v := url.Values{}
	if q == nil {
		return v
	}
	setString(v, "uuid", q.UUID)
	setString(v, "accountId", q.AccountID)
	setString(v, "symbolId", q.SymbolID)
	setString(v, "asset", q.Asset)
	setString(v, "operationType", strings.Join(q.OperationTypes, ","))
	setPositive(v, "offset", q.Offset)
	setPositive(v, "limit", q.Limit)
	setString(v, "order", string(q.Order))
	setTime(v, "fromDate", q.From, formatOffsetTime)
	setTime(v, "toDate", q.To, formatOffsetTime)
	setString(v, "orderId", q.OrderID)
	if q.OrderPos != nil {
		v.Set("orderPos", strconv.Itoa(*q.OrderPos))
	}
	return v
}

// Accounts lists the accounts with their access levels: GET /md/3.0/accounts.
func (c *Client) Accounts(ctx context.Context) ([]AccountStatus, error) {
	return getAs[strictList[AccountStatus]](ctx, c, nil, mdPath+"/accounts")
}

// Summary returns the current summary of an account valued in the given currency:
// GET /md/3.0/summary/{id}/{currency}.
func (c *Client) Summary(ctx context.Context, accountID, currency string) (AccountSummary, error) {
	return getAs[AccountSummary](ctx, c, nil, mdPath+"/summary/%s/%s", one(accountID), one(currency))
}

// SummaryOn returns the summary of an account for a session date, written YYYY-MM-DD:
// GET /md/3.0/summary/{id}/{date}/{currency}. A malformed date fails with [ErrBadDate].
func (c *Client) SummaryOn(ctx context.Context, accountID, date, currency string) (AccountSummary, error) {
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return AccountSummary{}, fmt.Errorf("%w: %q: %w", ErrBadDate, date, err)
	}
	return getAs[AccountSummary](ctx, c, nil, mdPath+"/summary/%s/%s/%s",
		one(accountID), one(date), one(currency))
}

// Transactions lists account transactions matching the filter: GET /md/3.0/transactions. A nil query selects
// everything the API returns by default.
func (c *Client) Transactions(ctx context.Context, q *TransactionsQuery) ([]Transaction, error) {
	return getAs[list[Transaction]](ctx, c, q.values(), mdPath+"/transactions")
}
