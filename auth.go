package exante

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// jwtHeader is the fixed JOSE header: the API accepts HS256 only.
const jwtHeader = `{"alg":"HS256","typ":"JWT"}`

// defaultAudience lists every endpoint group of the API, so one token serves every method.
var defaultAudience = []string{
	"accounts", "change", "crossrates", "feed", "ohlc", "orders", "summary", "symbols", "transactions",
}

type jwtClaims struct {
	Issuer   string   `json:"iss"`
	Subject  string   `json:"sub"`
	IssuedAt int64    `json:"iat"`
	Expires  int64    `json:"exp"`
	Audience []string `json:"aud"`
}

// tokenSource signs a JWT and keeps it until a third of its lifetime is left. The zero value is not usable.
type tokenSource struct {
	cred     Credentials
	audience []string
	ttl      time.Duration

	mu      sync.Mutex
	token   string
	renewAt time.Time
}

// get returns the cached token, signing a new one when the cached token is missing or due for renewal.
func (s *tokenSource) get() (string, error) {
	now := time.Now()
	// The token carries whole seconds; renewal counts from the expiry actually encoded, so that no token is reused
	// past its exp.
	exp := now.Add(s.ttl).Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && now.Before(s.renewAt) {
		return s.token, nil
	}
	token, err := s.sign(now.Unix(), exp)
	if err != nil {
		return "", err
	}
	s.token = token
	s.renewAt = time.Unix(exp, 0).Add(-s.ttl / 3)
	return token, nil
}

func (s *tokenSource) sign(iat, exp int64) (string, error) {
	payload, err := json.Marshal(jwtClaims{
		Issuer:   s.cred.ClientID,
		Subject:  s.cred.ApplicationID,
		IssuedAt: iat,
		Expires:  exp,
		Audience: s.audience,
	})
	if err != nil {
		return "", fmt.Errorf("exante: encoding JWT claims: %w", err)
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString([]byte(jwtHeader)) + "." + enc.EncodeToString(payload)

	mac := hmac.New(sha256.New, []byte(s.cred.SharedKey))
	mac.Write([]byte(signingInput))
	return signingInput + "." + enc.EncodeToString(mac.Sum(nil)), nil
}
