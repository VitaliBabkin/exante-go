package exante

import (
	"net/url"
	"strconv"
	"time"
)

// The filters of the list methods share these setters: a zero value is left out of the query, so the API applies its
// default.

func setString(v url.Values, key, val string) {
	if val != "" {
		v.Set(key, val)
	}
}

func setPositive(v url.Values, key string, n int) {
	if n > 0 {
		v.Set(key, strconv.Itoa(n))
	}
}

func setTime(v url.Values, key string, t time.Time, format func(time.Time) string) {
	if !t.IsZero() {
		v.Set(key, format(t))
	}
}

// The wire forms of a time: each endpoint family takes its own.

// formatEpochMillis writes t as milliseconds since the Unix epoch, the form the history endpoints take.
func formatEpochMillis(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

// formatMillisTime writes t in UTC with milliseconds, like 2017-05-18T10:00:00.000Z, the form of the order endpoints.
func formatMillisTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// formatOffsetTime writes t as an ISO date-time with a numeric offset, the form the transaction filters accept.
func formatOffsetTime(t time.Time) string { return t.Format("2006-01-02T15:04:05-07:00") }
