package exante_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/VitaliBabkin/exante-go"
)

// The examples below are compiled but not run: they need credentials and the live API.

func ExampleNew() {
	client, err := exante.New("https://api-live.exante.eu", exante.Credentials{
		ClientID:      "...",
		ApplicationID: "...",
		SharedKey:     "...",
	}, exante.WithTimeout(30*time.Second))
	if err != nil {
		log.Fatal(err)
	}
	defer client.CloseIdleConnections()

	accounts, err := client.Accounts(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	for _, account := range accounts {
		fmt.Println(account.AccountID)
	}
}

func ExampleClient_Summary() {
	var client *exante.Client // from exante.New

	summary, err := client.Summary(context.Background(), "ABC1234.001", "EUR")
	if err != nil {
		log.Fatal(err)
	}
	for _, position := range summary.Positions {
		fmt.Println(position.SymbolID, position.Quantity)
	}
}

func ExampleClient_OHLC() {
	var client *exante.Client // from exante.New

	candles, err := client.OHLC(context.Background(), "AAPL.NASDAQ", time.Hour, &exante.HistoryQuery{Size: 100})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(candles))
}

func ExampleClient_PlaceOrder() {
	var client *exante.Client // from exante.New
	ctx := context.Background()

	orders, err := client.PlaceOrder(ctx, &exante.PlaceParams{
		AccountID: "ABC1234.001", SymbolID: "AAPL.NASDAQ", Side: exante.Buy, Quantity: "6",
		OrderType: exante.OrderLimit, Duration: exante.DurationDay, LimitPrice: "120.5", ClientTag: "batch-17",
	})
	if errors.Is(err, exante.ErrOutcomeUnknown) {
		// The request may have reached the exchange: look the order up by its ClientTag before trying again.
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(orders))
}

func ExampleClient_OrderStream() {
	var client *exante.Client // from exante.New

	for update, err := range client.OrderStream(context.Background()) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(update.Order.OrderID, update.Order.State.Status)
	}
}
