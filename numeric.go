package exante

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Decimal is a number kept as the exact text the API sent. Money, prices and quantities use it instead of float64 so
// that no digit is lost or invented on the way; convert with [Decimal.Float64] only for approximate work.
//
// The API documents these values as JSON strings but may send JSON numbers; Decimal decodes both, and null or an
// absent field as the empty Decimal. It always encodes as a JSON string, the form the API takes in requests.
type Decimal string

// DecimalFromFloat formats f with the shortest text that parses back to f.
func DecimalFromFloat(f float64) Decimal {
	return Decimal(strconv.FormatFloat(f, 'f', -1, 64))
}

// String returns the decimal text.
func (d Decimal) String() string { return string(d) }

// IsEmpty reports whether the value is absent.
func (d Decimal) IsEmpty() bool { return d == "" }

// Float64 parses the value. It fails for the empty Decimal.
func (d Decimal) Float64() (float64, error) {
	f, err := strconv.ParseFloat(string(d), 64)
	if err != nil {
		return 0, fmt.Errorf("exante: decimal %q: %w", string(d), err)
	}
	return f, nil
}

// UnmarshalJSON accepts a JSON string, number or null.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	text, _, err := scalarText(b, "decimal")
	if err != nil {
		return err
	}
	*d = Decimal(text)
	return nil
}

// Code is an identifier that is text but may consist of digits only, such as a CUSIP or a SEDOL. The API documents
// it as a string; Code also decodes a JSON number (whose leading zeros, if any, the sender already lost) and null.
type Code string

// UnmarshalJSON accepts a JSON string, number or null.
func (c *Code) UnmarshalJSON(b []byte) error { return (*Decimal)(c).UnmarshalJSON(b) }

// Int64 is an integer the API documents as a number but may send as a numeric string. It decodes a JSON number
// (a fractional part must be zero), a numeric string and null (zero).
type Int64 int64

// UnmarshalJSON accepts a JSON number, a numeric string or null.
func (n *Int64) UnmarshalJSON(b []byte) error {
	text, _, err := scalarText(b, "integer")
	if err != nil {
		return err
	}
	if text == "" {
		*n = 0
		return nil
	}
	v, err := parseInt64(text)
	if err != nil {
		return err
	}
	*n = Int64(v)
	return nil
}

// Millis is a timestamp in milliseconds since the Unix epoch. Like [Int64] it decodes a number, a numeric string or
// null (zero).
type Millis int64

// UnmarshalJSON accepts a JSON number, a numeric string or null.
func (m *Millis) UnmarshalJSON(b []byte) error { return (*Int64)(m).UnmarshalJSON(b) }

// Time converts the timestamp; zero (absent) gives the zero time.Time.
func (m Millis) Time() time.Time {
	if m == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(m)).UTC()
}

// MillisOf converts t to a timestamp; the zero time gives zero.
func MillisOf(t time.Time) Millis {
	if t.IsZero() {
		return 0
	}
	return Millis(t.UnixMilli())
}

func isJSONNumber(b []byte) bool {
	return len(b) > 0 && (b[0] == '-' || b[0] >= '0' && b[0] <= '9')
}

func isNull(b []byte) bool { return string(bytes.TrimSpace(b)) == "null" }

// scalarText reduces a JSON string, number or null to its text; null gives the empty text. what names the target in
// the error for any other JSON value.
func scalarText(b []byte, what string) (text string, present bool, err error) {
	b = bytes.TrimSpace(b)
	switch {
	case isNull(b):
		return "", false, nil
	case len(b) > 0 && b[0] == '"':
		text, err = unquote(b)
		if err != nil {
			return "", false, fmt.Errorf("exante: decoding %s: %w", what, err)
		}
		return text, true, nil
	case isJSONNumber(b):
		return string(b), true, nil
	}
	return "", false, fmt.Errorf("exante: decoding %s: unexpected JSON value %.20q", what, b)
}

// unquote returns the content of a JSON string; an escape-free valid string is sliced without a second decode.
func unquote(b []byte) (string, error) {
	if n := len(b); n >= 2 && b[n-1] == '"' && bytes.IndexByte(b, '\\') < 0 && utf8.Valid(b) {
		return string(b[1 : n-1]), nil
	}
	var s string
	err := json.Unmarshal(b, &s)
	return s, err
}

// maxIntegerText bounds the text parseInt64 will look at; no int64 needs more, and it keeps a hostile exponent from
// making the exact arithmetic expensive.
const maxIntegerText = 40

// parseInt64 reads an integer written plainly or in decimal or exponent notation such as 1.5508330755e12. The text is
// evaluated exactly, never through float64, and must denote an integer within the int64 range.
func parseInt64(s string) (int64, error) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	if len(s) > maxIntegerText || strings.Trim(s, "0123456789+-.eE") != "" {
		return 0, fmt.Errorf("exante: decoding integer %.*q: not a number", maxIntegerText, s)
	}
	if err := checkExponent(s); err != nil {
		return 0, err
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("exante: decoding integer %q: not a number", s)
	}
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, fmt.Errorf("exante: decoding integer %q: not an integral int64 value", s)
	}
	return r.Num().Int64(), nil
}

// checkExponent rejects, before any big-number arithmetic, a decimal exponent that no int64 can result from. The
// mantissa has at most maxIntegerText characters, so an exponent beyond that magnitude either overshoots the int64
// range or leaves a fraction; only a zero mantissa would survive, and rejecting that oddity is harmless.
func checkExponent(s string) error {
	i := strings.IndexAny(s, "eE")
	if i < 0 {
		return nil
	}
	exp, err := strconv.Atoi(s[i+1:])
	if err != nil || exp > maxIntegerText || exp < -maxIntegerText {
		return fmt.Errorf("exante: decoding integer %.*q: exponent out of range", maxIntegerText, s)
	}
	return nil
}

// emptyBodyOK marks the decode targets for which an empty answer body means an empty list.
type emptyBodyOK interface{ acceptsEmptyBody() }

// strictList decodes a JSON array; null and an empty body give an empty list. Any other JSON value is an error: a
// 200 answer that is not a list (an error object, say) must not become a phantom element.
type strictList[T any] []T

func (*strictList[T]) acceptsEmptyBody() {}

// UnmarshalJSON implements json.Unmarshaler.
func (l *strictList[T]) UnmarshalJSON(b []byte) error {
	items, err := decodeList[T](b, false)
	if err != nil {
		return err
	}
	*l = items
	return nil
}

// list decodes a JSON array, null (empty list) or, for the operations whose spec answer is a oneOf of list shapes, a
// single JSON object as a one-element list. An object that carries none of the element's known fields is an error
// rather than a zero element.
type list[T any] []T

func (*list[T]) acceptsEmptyBody() {}

// UnmarshalJSON implements json.Unmarshaler.
func (l *list[T]) UnmarshalJSON(b []byte) error {
	items, err := decodeList[T](b, true)
	if err != nil {
		return err
	}
	*l = items
	return nil
}

// decodeList decodes an array or null; with single, an object is accepted as a one-element list.
func decodeList[T any](b []byte, single bool) ([]T, error) {
	b = bytes.TrimSpace(b)
	switch {
	case isNull(b):
		return nil, nil
	case len(b) > 0 && b[0] == '[':
		var items []T
		err := json.Unmarshal(b, &items)
		return items, err
	case single && len(b) > 0 && b[0] == '{':
		var item T
		if err := json.Unmarshal(b, &item); err != nil {
			return nil, err
		}
		if isZero(item) {
			return nil, errors.New("exante: JSON object carries none of the expected fields")
		}
		return []T{item}, nil
	case single:
		return nil, fmt.Errorf("exante: expected a JSON array or object, got %s", jsonKind(b))
	}
	return nil, fmt.Errorf("exante: expected a JSON array, got %s", jsonKind(b))
}

// isZero reports whether v is the zero value of its type; T must be a concrete type.
func isZero[T any](v T) bool {
	rv := reflect.ValueOf(v)
	return !rv.IsValid() || rv.IsZero()
}

// jsonKind names the kind of the JSON value starting b, for error messages.
func jsonKind(b []byte) string {
	switch {
	case len(b) == 0:
		return "an empty value"
	case b[0] == '{':
		return "an object"
	case b[0] == '"':
		return "a string"
	case b[0] == 't' || b[0] == 'f':
		return "a boolean"
	default:
		return "a number"
	}
}
