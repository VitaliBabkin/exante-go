package exante

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecimalDecoding(t *testing.T) {
	type holder struct {
		V Decimal `json:"v"`
	}
	tests := []struct {
		name    string
		json    string
		want    Decimal
		wantErr bool
	}{
		{"string", `{"v":"123456.01"}`, "123456.01", false},
		{"number", `{"v":123456.01}`, "123456.01", false},
		{"integer", `{"v":42}`, "42", false},
		{"negative", `{"v":-2710.86}`, "-2710.86", false},
		{"exponent keeps text", `{"v":1.5e-7}`, "1.5e-7", false},
		{"trailing zeros kept", `{"v":120.50}`, "120.50", false},
		{"long digits not rounded", `{"v":"0.1234567890123456789"}`, "0.1234567890123456789", false},
		{"null", `{"v":null}`, "", false},
		{"absent", `{}`, "", false},
		{"empty string", `{"v":""}`, "", false},
		{"escaped string", `{"v":"1.5\t"}`, "1.5\t", false},
		{"non-ASCII string", `{"v":"€5"}`, "€5", false},
		{"bool", `{"v":true}`, "", true},
		{"object", `{"v":{}}`, "", true},
		{"array", `{"v":[1]}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h holder
			err := json.Unmarshal([]byte(tt.json), &h)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if h.V != tt.want {
				t.Errorf("value = %q, want %q", h.V, tt.want)
			}
		})
	}
}

func TestDecimalNullResetsValue(t *testing.T) {
	h := struct{ V Decimal }{V: "5"}
	if err := json.Unmarshal([]byte(`{"V":null}`), &h); err != nil {
		t.Fatal(err)
	}
	if h.V != "" {
		t.Errorf("value = %q, want empty after null", h.V)
	}
}

func TestDecimalEncodesAsString(t *testing.T) {
	type req struct {
		Q Decimal `json:"q"`
		P Decimal `json:"p,omitempty"`
	}
	b, err := json.Marshal(req{Q: "6.50"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"q":"6.50"}`; got != want {
		t.Errorf("encoded = %s, want %s", got, want)
	}
}

func TestDecimalHelpers(t *testing.T) {
	a, b := 0.1, 0.2
	if got := DecimalFromFloat(a + b); got.String() != "0.30000000000000004" {
		t.Errorf("DecimalFromFloat = %q", got)
	}
	if got := DecimalFromFloat(120); got != "120" {
		t.Errorf("DecimalFromFloat(120) = %q", got)
	}
	f, err := Decimal("101.02").Float64()
	if err != nil || f != 101.02 {
		t.Errorf("Float64 = %v, %v", f, err)
	}
	if _, err := Decimal("").Float64(); err == nil {
		t.Error("Float64 of the empty Decimal succeeded")
	}
	if !Decimal("").IsEmpty() || Decimal("0").IsEmpty() {
		t.Error("IsEmpty is wrong")
	}
}

func TestCodeDecoding(t *testing.T) {
	tests := []struct {
		name string
		json string
		want Code
	}{
		{"string", `{"c":"037833100"}`, "037833100"},
		{"number", `{"c":2046251}`, "2046251"},
		{"null", `{"c":null}`, ""},
		{"absent", `{}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h struct {
				C Code `json:"c"`
			}
			if err := json.Unmarshal([]byte(tt.json), &h); err != nil {
				t.Fatal(err)
			}
			if h.C != tt.want {
				t.Errorf("got %q, want %q", h.C, tt.want)
			}
		})
	}
}

func TestMillisTime(t *testing.T) {
	want := time.Date(2019, 2, 22, 11, 17, 55, 530_000_000, time.UTC)
	if got := Millis(1550834275530).Time(); !got.Equal(want) {
		t.Errorf("Time() = %v, want %v", got, want)
	}
	if !Millis(0).Time().IsZero() {
		t.Error("zero Millis should give the zero time")
	}
	if MillisOf(want) != 1550834275530 || MillisOf(time.Time{}) != 0 {
		t.Error("MillisOf is wrong")
	}
}

func TestListDecoding(t *testing.T) {
	type item struct {
		ID string `json:"id"`
	}
	tests := []struct {
		name string
		json string
		want int
	}{
		{"array", `[{"id":"a"},{"id":"b"}]`, 2},
		{"single object", `{"id":"a"}`, 1},
		{"null", `null`, 0},
		{"empty array", `[]`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var l list[item]
			if err := json.Unmarshal([]byte(tt.json), &l); err != nil {
				t.Fatal(err)
			}
			if len(l) != tt.want {
				t.Errorf("len = %d, want %d", len(l), tt.want)
			}
		})
	}
	var l list[item]
	if err := json.Unmarshal([]byte(`"text"`), &l); err == nil {
		t.Error("a JSON string decoded as a list")
	}
}

func TestSymbolTypesDecoding(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []SymbolType
	}{
		{"string", `"FUTURE"`, []SymbolType{SymbolTypeFuture}},
		{"array", `["STOCK","OPTION"]`, []SymbolType{SymbolTypeStock, SymbolTypeOption}},
		{"null", `null`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got SymbolTypes
			if err := json.Unmarshal([]byte(tt.json), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
	var got SymbolTypes
	if err := json.Unmarshal([]byte(`42`), &got); err == nil {
		t.Error("a JSON number decoded as symbol types")
	}
}

func TestFeedEventDecoding(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		wantData   bool
		wantStatus FeedStatusEvent
	}{
		{"quote", `{"timestamp":1,"symbolId":"A.B","bid":[],"ask":[]}`, true, ""},
		{"start", `{"event":"subscription_start","symbolId":"A.B","timestamp":1}`, false, SubscriptionStart},
		{"stop", `{"event":"subscription_stop","symbolId":"A.B","timestamp":"1","reason":"gone"}`, false, SubscriptionStop},
		{"heartbeat", `{"event":"heartbeat"}`, false, ""},
		{"unknown event", `{"event":"newKind","symbolId":"A.B"}`, false, ""},
		{"null", `null`, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ev FeedEvent[Quote]
			if err := json.Unmarshal([]byte(tt.json), &ev); err != nil {
				t.Fatal(err)
			}
			if (ev.Data != nil) != tt.wantData {
				t.Errorf("Data = %v, want set=%v", ev.Data, tt.wantData)
			}
			if (ev.Status != nil) != (tt.wantStatus != "") || (ev.Status != nil && ev.Status.Event != tt.wantStatus) {
				t.Errorf("Status = %+v, want event %q", ev.Status, tt.wantStatus)
			}
		})
	}
	var ev FeedEvent[Quote]
	if err := json.Unmarshal([]byte(`[1]`), &ev); err == nil {
		t.Error("a JSON array decoded as a feed event")
	}
}
