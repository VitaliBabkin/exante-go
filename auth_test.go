package exante

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// bearer extracts the JWT from the Authorization header of r.
func bearer(t *testing.T, r seen) string {
	t.Helper()
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		t.Fatalf("Authorization = %q, want a Bearer token", r.Header.Get("Authorization"))
	}
	return token
}

// verifyJWT checks the HS256 signature of token against key and returns its header and claims.
func verifyJWT(t *testing.T, token, key string) (map[string]any, jwtClaims) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Fatalf("signature = %q, want %q", parts[2], want)
	}
	var header map[string]any
	var claims jwtClaims
	for i, dst := range []any{&header, &claims} {
		raw, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil {
			t.Fatalf("decoding part %d: %v", i, err)
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			t.Fatalf("unmarshalling part %d: %v", i, err)
		}
	}
	return header, claims
}

func TestTokenContent(t *testing.T) {
	rec := reply(200, `[]`)
	c := newTestClient(t, rec, WithTokenTTL(2*time.Hour))
	before := time.Now().Unix()
	if _, err := c.Accounts(context.Background()); err != nil {
		t.Fatal(err)
	}
	header, claims := verifyJWT(t, bearer(t, rec.only(t)), testCred.SharedKey)

	if header["alg"] != "HS256" || header["typ"] != "JWT" {
		t.Errorf("header = %v, want HS256 JWT", header)
	}
	if claims.Issuer != testCred.ClientID || claims.Subject != testCred.ApplicationID {
		t.Errorf("iss/sub = %q/%q, want %q/%q", claims.Issuer, claims.Subject, testCred.ClientID, testCred.ApplicationID)
	}
	if claims.IssuedAt < before || claims.IssuedAt > time.Now().Unix() {
		t.Errorf("iat = %d, outside the test window", claims.IssuedAt)
	}
	if got := claims.Expires - claims.IssuedAt; got != int64(2*time.Hour/time.Second) {
		t.Errorf("exp-iat = %d, want 7200", got)
	}
	for _, scope := range []string{"accounts", "feed", "ohlc", "orders", "summary", "symbols", "crossrates", "change", "transactions"} {
		if !slices.Contains(claims.Audience, scope) {
			t.Errorf("aud %v lacks %q", claims.Audience, scope)
		}
	}
}

func TestWithAudience(t *testing.T) {
	rec := reply(200, `[]`)
	c := newTestClient(t, rec, WithAudience("symbols", "feed"))
	if _, err := c.Groups(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, claims := verifyJWT(t, bearer(t, rec.only(t)), testCred.SharedKey)
	if want := []string{"symbols", "feed"}; !slices.Equal(claims.Audience, want) {
		t.Errorf("aud = %v, want %v", claims.Audience, want)
	}
}

func TestTokenReusedThenRenewed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := reply(200, `[]`)
		c := newTestClient(t, rec, WithTokenTTL(30*time.Minute))
		call := func() {
			t.Helper()
			if _, err := c.Accounts(context.Background()); err != nil {
				t.Fatal(err)
			}
		}

		call()
		time.Sleep(19 * time.Minute) // renewal is due at 20 minutes: two thirds of the lifetime
		call()
		time.Sleep(time.Minute + time.Second)
		call()
		time.Sleep(10 * time.Minute) // within the renewed token's first two thirds
		call()

		reqs := rec.all()
		tokens := make([]string, len(reqs))
		for i, r := range reqs {
			tokens[i] = bearer(t, r)
		}
		if tokens[0] != tokens[1] {
			t.Error("token was re-signed before the renewal point")
		}
		if tokens[1] == tokens[2] {
			t.Error("token was not renewed after the renewal point")
		}
		if tokens[2] != tokens[3] {
			t.Error("renewed token was not reused")
		}
		_, claims := verifyJWT(t, tokens[2], testCred.SharedKey)
		if want := time.Now().Add(-10 * time.Minute).Unix(); claims.IssuedAt != want {
			t.Errorf("renewed iat = %d, want %d", claims.IssuedAt, want)
		}
	})
}

func TestClientsDoNotShareTokens(t *testing.T) {
	recA, recB := reply(200, `[]`), reply(200, `[]`)
	a := newTestClient(t, recA)
	b := newTestClient(t, recB)
	ctx := context.Background()
	for range 2 {
		if _, err := a.Accounts(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Accounts(ctx); err != nil {
		t.Fatal(err)
	}
	if a.tokens == b.tokens {
		t.Fatal("two clients share one token source")
	}

	other, err := New("https://api.test", Credentials{ClientID: "client-1", ApplicationID: "app-2", SharedKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	tokA, err := a.tokens.get()
	if err != nil {
		t.Fatal(err)
	}
	tokOther, err := other.tokens.get()
	if err != nil {
		t.Fatal(err)
	}
	if tokA == tokOther {
		t.Error("clients with different credentials produced the same token")
	}
}

func TestConcurrentCallsShareOneToken(t *testing.T) {
	rec := reply(200, `[]`)
	c := newTestClient(t, rec)

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if _, err := c.Accounts(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	reqs := rec.all()
	if len(reqs) != 32 {
		t.Fatalf("server saw %d requests, want 32", len(reqs))
	}
	first := bearer(t, reqs[0])
	for _, r := range reqs {
		if bearer(t, r) != first {
			t.Fatal("concurrent calls used different tokens")
		}
	}
}

func TestTokenCacheReturnsSameString(t *testing.T) {
	c := newClient(t)
	first, err := c.tokens.get()
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		next, err := c.tokens.get()
		if err != nil || next != first {
			t.Fatalf("get = %q, %v; want the cached token", next, err)
		}
	}
}
