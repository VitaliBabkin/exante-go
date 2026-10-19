package exante

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestHugeExponentsAreRejectedWithoutBigArithmetic(t *testing.T) {
	for _, text := range []string{"1e999999", "1e-999999", "9e999999", "1e1000000", "1e-1000000", "1e1000000000"} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := parseInt64(text)
		runtime.ReadMemStats(&after)
		if err == nil {
			t.Errorf("%s was accepted", text)
		}
		// A million-digit number takes hundreds of kilobytes; the rejection allocates only its message.
		if got := after.TotalAlloc - before.TotalAlloc; got > 16<<10 {
			t.Errorf("%s allocated %d bytes: the exponent must be rejected before any big-number work", text, got)
		}
	}
}

func TestIntegersAreParsedExactly(t *testing.T) {
	tests := []struct {
		text    string
		want    int64
		wantErr bool
	}{
		{"2020", 2020, false},
		{"-5", -5, false},
		{"42.0", 42, false},
		{"1.55083307553e12", 1550833075530, false},
		{"9007199254740993", 9007199254740993, false},
		{"9007199254740993.0", 9007199254740993, false},
		{"9.007199254740993e15", 9007199254740993, false},
		{"9007199254740993e0", 9007199254740993, false},
		{"90071992547409.93e2", 9007199254740993, false},
		{"1.5508330755e12", 1550833075500, false},
		{"10e-1", 1, false},
		{"-9007199254740993.0", -9007199254740993, false},
		{"9223372036854775807", 9223372036854775807, false},
		{"9.223372036854775807e18", 9223372036854775807, false},
		{"-9223372036854775808", -9223372036854775808, false},
		{"-9.223372036854775808e18", -9223372036854775808, false},
		{"9223372036854775808", 0, true},
		{"9.223372036854775808e18", 0, true},
		{"-9.223372036854775809e18", 0, true},
		{"1e30", 0, true},
		{"1.5", 0, true},
		{"9007199254740993.5", 0, true},
		{"1e-1", 0, true},
		{"1e1000000000", 0, true},
		{"1e1000000", 0, true},
		{"1e-1000000", 0, true},
		{"1e19", 0, true},
		{"1e+99999999999999999999", 0, true},
		{"10000000000000000000000000000000000e-30", 10000, false},
		{"0x10", 0, true},
		{"1/2", 0, true},
		{"1_000", 0, true},
		{"abc", 0, true},
		{"NaN", 0, true},
		{"1e", 0, true},
		{"--1", 0, true},
		{strings.Repeat("9", 100), 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			for name, wire := range map[string]string{"number": tt.text, "string": `"` + tt.text + `"`} {
				if name == "number" && !isJSONNumber([]byte(tt.text)) {
					continue
				}
				var n Int64
				var m Millis
				errN := json.Unmarshal([]byte(wire), &n)
				errM := json.Unmarshal([]byte(wire), &m)
				if (errN != nil) != tt.wantErr || (errM != nil) != tt.wantErr {
					t.Fatalf("%s %s: errors = %v / %v, wantErr %v", name, wire, errN, errM, tt.wantErr)
				}
				if !tt.wantErr && (int64(n) != tt.want || int64(m) != tt.want) {
					t.Errorf("%s %s = %d / %d, want %d", name, wire, n, m, tt.want)
				}
			}
		})
	}
}

func TestIntegerAbsentAndWrongKinds(t *testing.T) {
	type holder struct {
		N Int64  `json:"n"`
		M Millis `json:"m"`
	}
	const before = 7 // what a field holds when the JSON does not mention it
	tests := []struct {
		name    string
		json    string
		want    int64
		wantErr bool
	}{
		{"null", `{"n":null,"m":null}`, 0, false},
		{"empty string", `{"n":"","m":""}`, 0, false},
		{"absent", `{}`, before, false},
		{"bool", `{"m":true}`, 0, true},
		{"object", `{"n":{}}`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := holder{N: before, M: before}
			err := json.Unmarshal([]byte(tt.json), &h)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && (int64(h.N) != tt.want || int64(h.M) != tt.want) {
				t.Errorf("got n=%d m=%d, want %d", h.N, h.M, tt.want)
			}
		})
	}
}

func TestPositionsAndFractionDigitsAcceptNumbersAndStrings(t *testing.T) {
	var fill OrderFill
	if err := json.Unmarshal([]byte(`{"position":"3","price":"1"}`), &fill); err != nil || fill.Position != 3 {
		t.Errorf("OrderFill.position as a string = %+v, %v", fill, err)
	}
	var trade Trade
	if err := json.Unmarshal([]byte(`{"position":4}`), &trade); err != nil || trade.Position != 4 {
		t.Errorf("Trade.position as a number = %+v, %v", trade, err)
	}
	var cur CurrencyInfo
	if err := json.Unmarshal([]byte(`{"id":"EUR","fractionDigits":"2"}`), &cur); err != nil || cur.FractionDigits != 2 {
		t.Errorf("CurrencyInfo.fractionDigits as a string = %+v, %v", cur, err)
	}
}

// TestTokenIsNeverUsedPastItsEncodedExpiry runs on a clock with a fractional second: the token carries whole seconds,
// so the renewal point must count from the exp actually encoded.
func TestTokenIsNeverUsedPastItsEncodedExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := reply(200, `[]`)
		c := newTestClient(t, rec, WithTokenTTL(time.Second))
		time.Sleep(500 * time.Millisecond) // the clock now reads :00.5

		for range 80 {
			if _, err := c.Accounts(context.Background()); err != nil {
				t.Fatal(err)
			}
			reqs := rec.all()
			_, claims := verifyJWT(t, bearer(t, reqs[len(reqs)-1]), testCred.SharedKey)
			if now := time.Now(); !now.Before(time.Unix(claims.Expires, 0)) {
				t.Fatalf("at %v the request carried a token that expired at %v", now, time.Unix(claims.Expires, 0))
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
}

func TestTokenRenewalPointCountsFromTheEncodedExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := reply(200, `[]`)
		c := newTestClient(t, rec, WithTokenTTL(3*time.Second))
		call := func() string {
			t.Helper()
			if _, err := c.Accounts(context.Background()); err != nil {
				t.Fatal(err)
			}
			return bearer(t, rec.all()[len(rec.all())-1])
		}

		time.Sleep(900 * time.Millisecond) // :00.9, so exp is :03 and renewal is due at :02
		first := call()
		_, claims := verifyJWT(t, first, testCred.SharedKey)
		time.Sleep(900 * time.Millisecond) // :01.8
		if call() != first {
			t.Error("token renewed before the renewal point")
		}
		time.Sleep(300 * time.Millisecond) // :02.1, past the renewal point though only 1.2s after signing
		if call() == first {
			t.Errorf("token not renewed after the renewal point; it expires at %v", time.Unix(claims.Expires, 0))
		}
	})
}

const secretKey = "s3cr3t-shared-key-0123456789"

func TestCredentialsNeverPrintTheSharedKey(t *testing.T) {
	cred := Credentials{ClientID: "469a8180-51fb-408f-a1f0", ApplicationID: "77b378e8-3a30-4f85-9017", SharedKey: secretKey}
	holder := struct{ Cred Credentials }{cred}

	outputs := []string{
		fmt.Sprintf("%v", cred), fmt.Sprintf("%+v", cred), fmt.Sprintf("%#v", cred),
		fmt.Sprintf("%v", &cred), fmt.Sprintf("%+v", &cred), fmt.Sprintf("%#v", &cred),
		fmt.Sprintf("%v", holder), fmt.Sprintf("%+v", holder), fmt.Sprintf("%#v", holder),
		fmt.Sprint(cred), fmt.Sprint(holder),
	}
	var text, js strings.Builder
	slog.New(slog.NewTextHandler(&text, nil)).Info("cred", "cred", cred)
	slog.New(slog.NewJSONHandler(&js, nil)).Info("cred", slog.Any("cred", cred))
	outputs = append(outputs, text.String(), js.String())

	for _, out := range outputs {
		if strings.Contains(out, secretKey) || strings.Contains(out, "s3cr3t") {
			t.Errorf("output leaks the shared key: %s", out)
		}
		if strings.Contains(out, cred.ClientID) || strings.Contains(out, cred.ApplicationID) {
			t.Errorf("output prints a full identifier: %s", out)
		}
	}
	if got := fmt.Sprintf("%+v", cred); !strings.Contains(got, "469a...") || !strings.Contains(got, "***") {
		t.Errorf("redacted form = %q, want shortened identifiers and ***", got)
	}
	if got := shorten("abc"); got != "***" {
		t.Errorf("shorten of a short id = %q", got)
	}
}

func TestClientNeverPrintsCredentialsOrToken(t *testing.T) {
	c, err := New("https://api.test", Credentials{ClientID: "client-1", ApplicationID: "app-1", SharedKey: secretKey})
	if err != nil {
		t.Fatal(err)
	}
	token, err := c.tokens.get()
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%+v", *c)} {
		if strings.Contains(out, secretKey) || strings.Contains(out, token) {
			t.Errorf("output leaks a secret: %s", out)
		}
	}
}

func TestPlaceBodyBytes(t *testing.T) {
	p := &PlaceParams{
		AccountID: "A.001", SymbolID: "AAPL.NASDAQ", Side: Sell, Quantity: "6", OrderType: OrderTWAP,
		Duration: DurationGoodTillTime, LimitPrice: "120.5", StopPrice: "119.0", PartQuantity: "2",
		PlaceInterval: "30", PriceDistance: "1.5", ClientTag: "tag", TakeProfit: "130", StopLoss: "90",
		OCOGroup: "oco", IfDoneParentID: "parent",
		GTTExpiration: time.Date(2017, 8, 14, 4, 40, 0, 123_456_789, time.FixedZone("EET", 2*3600)),
	}
	const want = `{"accountId":"A.001","symbolId":"AAPL.NASDAQ","side":"sell","quantity":"6","orderType":"twap",` +
		`"duration":"good_till_time","limitPrice":"120.5","stopPrice":"119.0","partQuantity":"2","placeInterval":"30",` +
		`"priceDistance":"1.5","clientTag":"tag","takeProfit":"130","stopLoss":"90","ocoGroup":"oco",` +
		`"ifDoneParentId":"parent","gttExpiration":"2017-08-14T02:40:00.123Z"}`

	got, err := json.Marshal(p.wire())

	if err != nil || string(got) != want {
		t.Errorf("body = %s (%v)\nwant   %s", got, err, want)
	}
	minimal, err := json.Marshal((&PlaceParams{SymbolID: "S", Side: Buy, Quantity: "1", OrderType: OrderMarket, Duration: DurationDay}).wire())
	if want := `{"symbolId":"S","side":"buy","quantity":"1","orderType":"market","duration":"day"}`; err != nil || string(minimal) != want {
		t.Errorf("minimal body = %s (%v), want %s", minimal, err, want)
	}
}
