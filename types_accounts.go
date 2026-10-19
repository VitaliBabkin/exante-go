package exante

// AccountAccess is what an account allows its owner to do.
type AccountAccess string

// Account access levels listed by the API.
const (
	AccessReadOnly  AccountAccess = "ReadOnly"
	AccessCloseOnly AccountAccess = "CloseOnly"
	AccessFull      AccountAccess = "Full"
)

// SortOrder orders the transactions of [TransactionsQuery].
type SortOrder string

// Sort orders accepted by the API.
const (
	SortAscending  SortOrder = "ASC"
	SortDescending SortOrder = "DESC"
)

// AccountStatus is an account with its access level.
type AccountStatus struct {
	AccountID string        `json:"accountId"`
	Status    AccountAccess `json:"status"`
}

// CurrencyPosition is a currency balance of an account.
type CurrencyPosition struct {
	Code           string  `json:"code"`
	Value          Decimal `json:"value"`
	ConvertedValue Decimal `json:"convertedValue"`
}

// Position is an open position of an account. The converted values are in the currency of the report.
type Position struct {
	SymbolID        string     `json:"symbolId"`
	SymbolType      SymbolType `json:"symbolType"`
	Quantity        Decimal    `json:"quantity"`
	Currency        string     `json:"currency"`
	Price           Decimal    `json:"price"`
	AveragePrice    Decimal    `json:"averagePrice"`
	PnL             Decimal    `json:"pnl"`
	ConvertedPnL    Decimal    `json:"convertedPnl"`
	Value           Decimal    `json:"value"`
	ConvertedValue  Decimal    `json:"convertedValue"`
	AccruedInterest Decimal    `json:"accruedInterest"`
}

// AccountSummary is the state of an account at a moment, valued in Currency.
type AccountSummary struct {
	AccountID string `json:"accountId"`
	Currency  string `json:"currency"`
	// SessionDate is a calendar date, YYYY-MM-DD.
	SessionDate        string             `json:"sessionDate"`
	Timestamp          Millis             `json:"timestamp"`
	NetAssetValue      Decimal            `json:"netAssetValue"`
	FreeMoney          Decimal            `json:"freeMoney"`
	MoneyUsedForMargin Decimal            `json:"moneyUsedForMargin"`
	MarginUtilization  Decimal            `json:"marginUtilization"`
	Currencies         []CurrencyPosition `json:"currencies"`
	Positions          []Position         `json:"positions"`
}

// Transaction is one account transaction.
type Transaction struct {
	ID            Int64  `json:"id"`
	UUID          string `json:"uuid"`
	OperationType string `json:"operationType"`
	Timestamp     Millis `json:"timestamp"`
	// Sum is the transaction amount.
	Sum       Decimal `json:"sum"`
	Asset     string  `json:"asset"`
	AccountID string  `json:"accountId"`
	SymbolID  string  `json:"symbolId"`
	OrderID   string  `json:"orderId"`
	OrderPos  Int64   `json:"orderPos"`
	// ValueDate is a calendar date, YYYY-MM-DD.
	ValueDate  string `json:"valueDate"`
	ParentUUID string `json:"parentUuid"`
	Comment    string `json:"comment"`
	// TransactionPrice is the asset price at deposit time; empty for FOREX assets.
	TransactionPrice Decimal `json:"transactionPrice"`
}
