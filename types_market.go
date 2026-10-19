package exante

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// SymbolType is the kind of a financial instrument.
type SymbolType string

// Instrument types listed by the API.
const (
	SymbolTypeFXSpot         SymbolType = "FX_SPOT"
	SymbolTypeCurrency       SymbolType = "CURRENCY"
	SymbolTypeIndex          SymbolType = "INDEX"
	SymbolTypeStock          SymbolType = "STOCK"
	SymbolTypeBond           SymbolType = "BOND"
	SymbolTypeFund           SymbolType = "FUND"
	SymbolTypeFuture         SymbolType = "FUTURE"
	SymbolTypeOption         SymbolType = "OPTION"
	SymbolTypeCFD            SymbolType = "CFD"
	SymbolTypeCalendarSpread SymbolType = "CALENDAR_SPREAD"
)

// SessionName names a trading session of an instrument.
type SessionName string

// Trading session names listed by the API.
const (
	SessionPreMarket   SessionName = "PreMarket"
	SessionMainSession SessionName = "MainSession"
	SessionAfterMarket SessionName = "AfterMarket"
	SessionOffline     SessionName = "Offline"
	SessionOnline      SessionName = "Online"
	SessionExpired     SessionName = "Expired"
)

// TickType selects quotes or trades for [Client.OHLC] and [Client.Ticks].
type TickType string

// Tick types accepted by the API.
const (
	TickQuotes TickType = "quotes"
	TickTrades TickType = "trades"
)

// FeedLevel selects the depth of the quote feed.
type FeedLevel string

// Quote feed levels accepted by the API.
const (
	FeedBestPrice   FeedLevel = "best_price"
	FeedMarketDepth FeedLevel = "market_depth"
)

// FeedStatusEvent is the kind of a [FeedStatus] notice.
type FeedStatusEvent string

// Feed status events listed by the API.
const (
	SubscriptionStart FeedStatusEvent = "subscription_start"
	SubscriptionStop  FeedStatusEvent = "subscription_stop"
)

// SymbolTypes is the list of instrument types of a [Group]. The API documents the field as a single string but
// describes it as a list; SymbolTypes decodes a string, an array or null.
type SymbolTypes []SymbolType

// UnmarshalJSON implements json.Unmarshaler.
func (t *SymbolTypes) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var one SymbolType
		if err := json.Unmarshal(b, &one); err != nil {
			return fmt.Errorf("exante: decoding symbol types: %w", err)
		}
		*t = SymbolTypes{one}
		return nil
	}
	var many []SymbolType
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("exante: decoding symbol types: %w", err)
	}
	*t = many
	return nil
}

// Group is an instrument group.
type Group struct {
	ID       string      `json:"group"`
	Name     string      `json:"name"`
	Types    SymbolTypes `json:"types"`
	Exchange string      `json:"exchange"`
}

// Exchange is a trading venue.
type Exchange struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
}

// TypeDescriptor is an instrument type known to the API.
type TypeDescriptor struct {
	ID SymbolType `json:"id"`
}

// Identifiers are the security identifiers of an instrument.
type Identifiers struct {
	ISIN       string `json:"ISIN"`
	FIGI       string `json:"FIGI"`
	CUSIP      Code   `json:"CUSIP"`
	RIC        string `json:"RIC"`
	SEDOL      Code   `json:"SEDOL"`
	CFI        string `json:"CFI"`
	AssetClass string `json:"assetClass"`
}

// OptionData holds the option-specific properties of an instrument.
type OptionData struct {
	OptionGroupID string  `json:"optionGroupId"`
	OptionRight   string  `json:"optionRight"`
	StrikePrice   Decimal `json:"strikePrice"`
}

// SymbolInfo describes a financial instrument.
type SymbolInfo struct {
	SymbolID           string       `json:"symbolId"`
	Ticker             string       `json:"ticker"`
	Name               string       `json:"name"`
	Description        string       `json:"description"`
	Group              string       `json:"group"`
	UnderlyingSymbolID string       `json:"underlyingSymbolId"`
	Exchange           string       `json:"exchange"`
	Expiration         Millis       `json:"expiration"`
	Country            string       `json:"country"`
	SymbolType         SymbolType   `json:"symbolType"`
	OptionData         *OptionData  `json:"optionData"`
	MinPriceIncrement  Decimal      `json:"minPriceIncrement"`
	Currency           string       `json:"currency"`
	Identifiers        *Identifiers `json:"identifiers"`
	Icon               string       `json:"icon"`
}

// symbolInfoV2 is the answer shape of the endpoints that exist in API 2.0 only.
type symbolInfoV2 struct {
	ID                 string     `json:"id"`
	Ticker             string     `json:"ticker"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Group              string     `json:"group"`
	UnderlyingSymbolID string     `json:"underlyingSymbolId"`
	Exchange           string     `json:"exchange"`
	Expiration         Millis     `json:"expiration"`
	Country            string     `json:"country"`
	Type               SymbolType `json:"type"`
	OptionData         *struct {
		OptionGroupID string  `json:"optionGroupId"`
		Right         string  `json:"right"`
		StrikePrice   Decimal `json:"strikePrice"`
	} `json:"optionData"`
	MPI         Decimal      `json:"mpi"`
	Currency    string       `json:"currency"`
	Identifiers *Identifiers `json:"identifiers"`
}

// info converts the 2.0 shape to the 3.0 one used by the package.
func (s *symbolInfoV2) info() SymbolInfo {
	out := SymbolInfo{
		SymbolID:           s.ID,
		Ticker:             s.Ticker,
		Name:               s.Name,
		Description:        s.Description,
		Group:              s.Group,
		UnderlyingSymbolID: s.UnderlyingSymbolID,
		Exchange:           s.Exchange,
		Expiration:         s.Expiration,
		Country:            s.Country,
		SymbolType:         s.Type,
		MinPriceIncrement:  s.MPI,
		Currency:           s.Currency,
		Identifiers:        s.Identifiers,
	}
	if o := s.OptionData; o != nil {
		out.OptionData = &OptionData{OptionGroupID: o.OptionGroupID, OptionRight: o.Right, StrikePrice: o.StrikePrice}
	}
	return out
}

// Specification holds the trading parameters of an instrument.
type Specification struct {
	LotSize            Decimal `json:"lotSize"`
	ContractMultiplier Decimal `json:"contractMultiplier"`
	PriceUnit          Decimal `json:"priceUnit"`
	Units              string  `json:"units"`
}

// Interval is a trading session period.
type Interval struct {
	Start Millis `json:"start"`
	End   Millis `json:"end"`
}

// OrderTypeDurations maps an order type (such as "limit" or "twap") to the durations available for it. It uses
// plain strings for the keys because the API also reports types, like "pov" and "vwap", that [OrderType] does not
// list.
type OrderTypeDurations map[string][]OrderDuration

// ScheduleInterval is one session of an instrument schedule.
type ScheduleInterval struct {
	Name       SessionName        `json:"name"`
	Period     Interval           `json:"period"`
	OrderTypes OrderTypeDurations `json:"orderTypes"`
}

// Schedule is the trading schedule of an instrument.
type Schedule struct {
	Intervals []ScheduleInterval `json:"intervals"`
}

// Crossrate is the exchange rate between two currencies.
type Crossrate struct {
	Pair     string  `json:"pair"`
	SymbolID string  `json:"symbolId"`
	Rate     Decimal `json:"rate"`
}

// CurrencyInfo describes a currency known to the API.
type CurrencyInfo struct {
	ID             string `json:"id"`
	FractionDigits Int64  `json:"fractionDigits"`
	Icon           string `json:"icon"`
}

// DailyChange is the price change of an instrument since the previous session close.
type DailyChange struct {
	SymbolID              string  `json:"symbolId"`
	LastSessionClosePrice Decimal `json:"lastSessionClosePrice"`
	Change                Decimal `json:"dailyChange"`
}

// QuoteLevel is one price level of a quote side.
type QuoteLevel struct {
	Price Decimal `json:"price"`
	Size  Decimal `json:"size"`
}

// Quote is a bid/ask snapshot of an instrument.
type Quote struct {
	Timestamp Millis       `json:"timestamp"`
	SymbolID  string       `json:"symbolId"`
	Bid       []QuoteLevel `json:"bid"`
	Ask       []QuoteLevel `json:"ask"`
}

// TradeTick is one market trade of an instrument from the trades feed.
type TradeTick struct {
	Timestamp Millis  `json:"timestamp"`
	SymbolID  string  `json:"symbolId"`
	Price     Decimal `json:"price"`
	Size      Decimal `json:"size"`
}

// FeedStatus reports the start or the end of a subscription to an instrument on a feed.
type FeedStatus struct {
	Timestamp Millis          `json:"timestamp"`
	SymbolID  string          `json:"symbolId"`
	Event     FeedStatusEvent `json:"event"`
	Reason    string          `json:"reason"`
}

// FeedEvent is one element of a feed or of a last-quote answer: market data in Data, or a subscription notice in
// Status. Exactly one of them is set.
type FeedEvent[T any] struct {
	Data   *T
	Status *FeedStatus
}

// empty reports whether the event carries neither data nor a status: a heartbeat or an event of an unknown kind.
func (e FeedEvent[T]) empty() bool { return e.Data == nil && e.Status == nil }

// UnmarshalJSON tells the two shapes apart by the event field, which only status notices carry. A heartbeat, an event
// of an unknown kind and an object that carries none of the known fields decode to the zero FeedEvent.
func (e *FeedEvent[T]) UnmarshalJSON(b []byte) error {
	if isNull(b) {
		*e = FeedEvent[T]{}
		return nil
	}
	var kind struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(b, &kind); err != nil {
		return fmt.Errorf("exante: decoding feed event: %w", err)
	}
	*e = FeedEvent[T]{}
	switch FeedStatusEvent(kind.Event) {
	case "":
		e.Data = new(T)
		if err := json.Unmarshal(b, e.Data); err != nil {
			e.Data = nil
			return fmt.Errorf("exante: decoding feed data: %w", err)
		}
		if isZero(*e.Data) {
			e.Data = nil
		}
	case SubscriptionStart, SubscriptionStop:
		e.Status = new(FeedStatus)
		if err := json.Unmarshal(b, e.Status); err != nil {
			e.Status = nil
			return fmt.Errorf("exante: decoding feed status: %w", err)
		}
	}
	return nil
}

// Candle is one OHLC candle.
type Candle struct {
	Timestamp Millis  `json:"timestamp"`
	Open      Decimal `json:"open"`
	High      Decimal `json:"high"`
	Low       Decimal `json:"low"`
	Close     Decimal `json:"close"`
	// Volume is set for candles built from trades only.
	Volume Decimal `json:"volume"`
}

// Tick is one historical tick: a quote (Bid and Ask) or a trade (Price and Size), depending on the requested
// [TickType].
type Tick struct {
	Timestamp Millis       `json:"timestamp"`
	SymbolID  string       `json:"symbolId"`
	Bid       []QuoteLevel `json:"bid"`
	Ask       []QuoteLevel `json:"ask"`
	Price     Decimal      `json:"price"`
	Size      Decimal      `json:"size"`
}
