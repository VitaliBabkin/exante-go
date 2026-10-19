package exante

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxURLLength is the longest request URL the API accepts.
const maxURLLength = 2000

// Path prefixes of the API groups. Every method uses the stable 3.0 API except those whose endpoint exists in 2.0 only.
const (
	mdPath    = "/md/3.0"
	mdPathV2  = "/md/2.0"
	tradePath = "/trade/3.0"
)

// one wraps a single path parameter for [getAs] and [writeAs].
func one(s string) []string { return []string{s} }

// expandPath fills the %s placeholders of tmpl. Each argument is a list of identifiers; every identifier is escaped
// as one path segment (symbol ids hold dots, slashes and spaces) and the list is joined with literal commas, the
// form the API takes for several instruments. It returns the path in decoded and in escaped form.
func expandPath(tmpl string, args [][]string) (path, escaped string, err error) {
	parts := strings.Split(tmpl, "%s")
	if len(parts)-1 != len(args) {
		return "", "", fmt.Errorf("exante: internal error: %q takes %d path arguments, got %d", tmpl, len(parts)-1, len(args))
	}
	var plain, raw strings.Builder
	plain.WriteString(parts[0])
	raw.WriteString(parts[0])
	for i, ids := range args {
		if err := checkIDs(ids); err != nil {
			return "", "", fmt.Errorf("path argument %d of %s: %w", i+1, tmpl, err)
		}
		for j, id := range ids {
			if j > 0 {
				plain.WriteByte(',')
				raw.WriteByte(',')
			}
			plain.WriteString(id)
			raw.WriteString(url.PathEscape(id))
		}
		plain.WriteString(parts[i+1])
		raw.WriteString(parts[i+1])
	}
	return plain.String(), raw.String(), nil
}

func checkIDs(ids []string) error {
	if len(ids) == 0 {
		return ErrEmptyArgument
	}
	for _, id := range ids {
		switch id {
		case "":
			return ErrEmptyArgument
		case ".", "..":
			return fmt.Errorf("%w: %q is not a valid identifier", ErrInvalidArgument, id)
		}
	}
	return nil
}

// requestURL joins an endpoint path, given in decoded and escaped form, to the base URL.
func (c *Client) requestURL(path, escaped string, query url.Values) (*url.URL, error) {
	u := *c.base
	u.Path, u.RawPath = c.base.Path+path, c.base.EscapedPath()+escaped
	u.RawQuery = query.Encode()
	if len(u.String()) > maxURLLength {
		return nil, ErrURLTooLong
	}
	return &u, nil
}

// call describes one API request.
type call struct {
	method string
	tmpl   string
	args   [][]string
	query  url.Values
	body   any
	accept string
	// write marks a state-changing call: errors after the request may have been sent carry [ErrOutcomeUnknown].
	write bool
}

// exchange is what one request went through: the escaped path for messages, whether the transport was entered (the
// server may have seen the request) and, on a 2xx answer, the response whose body the caller must close.
type exchange struct {
	resp *http.Response
	path string
	sent bool
}

// getAs runs a GET and decodes the answer as T.
func getAs[T any](ctx context.Context, c *Client, query url.Values, tmpl string, args ...[]string) (T, error) {
	return doAs[T](ctx, c, &call{method: http.MethodGet, tmpl: tmpl, args: args, query: query})
}

// writeAs runs a state-changing POST with a JSON body and decodes the answer as T.
func writeAs[T any](ctx context.Context, c *Client, body any, tmpl string, args ...[]string) (T, error) {
	return doAs[T](ctx, c, &call{method: http.MethodPost, tmpl: tmpl, args: args, body: body, write: true})
}

// doAs runs a call bounded by the client timeout and decodes the JSON answer as T; on error the result is the zero T.
func doAs[T any](ctx context.Context, c *Client, cl *call) (T, error) {
	var out T
	if ctx == nil {
		return out, ErrNilContext
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ex, err := c.send(ctx, cl)
	if err == nil {
		err = c.readAnswer(ctx, cl, ex, &out)
	}
	if err != nil {
		var zero T
		return zero, cl.outcome(ex.sent, err)
	}
	return out, nil
}

// outcome marks the error of a write call that may have reached the server. A definite rejection (an API error below
// 500) and any error raised before the request was sent are returned as they are.
func (cl *call) outcome(sent bool, err error) error {
	if !cl.write || !sent {
		return err
	}
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.StatusCode < http.StatusInternalServerError {
		return err
	}
	return fmt.Errorf("%w: %w", ErrOutcomeUnknown, err)
}

// send sends the request and returns the exchange of a 2xx answer; the caller closes its body. Any other status
// yields an *APIError and the body is closed here.
func (c *Client) send(ctx context.Context, cl *call) (exchange, error) {
	req, path, err := c.newRequest(ctx, cl)
	if err != nil {
		return exchange{}, err
	}
	ex := exchange{path: path}
	token, err := c.tokens.get()
	if err != nil {
		return ex, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	ex.sent = true
	resp, err := c.hc.Do(req)
	if err != nil {
		return ex, fmt.Errorf("exante: %s %s: %w", cl.method, path, causeOr(ctx, err))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer discardClose(resp.Body)
		return ex, newAPIError(resp, cl.method, path)
	}
	ex.resp = resp
	return ex, nil
}

// readAnswer reads the body of ex within the size limit, closes it and decodes the JSON into out.
func (c *Client) readAnswer(ctx context.Context, cl *call, ex exchange, out any) error {
	defer discardClose(ex.resp.Body)
	data, err := readLimited(ex.resp.Body, ex.resp.ContentLength, c.maxBody)
	if err != nil {
		return fmt.Errorf("exante: reading %s %s response: %w", cl.method, ex.path, causeOr(ctx, err))
	}
	if _, ok := out.(emptyBodyOK); ok && len(bytes.TrimSpace(data)) == 0 {
		data = []byte("null")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("exante: decoding %s %s response (HTTP %d): %w", cl.method, ex.path, ex.resp.StatusCode, err)
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, cl *call) (req *http.Request, path string, err error) {
	plain, escaped, err := expandPath(cl.tmpl, cl.args)
	if err != nil {
		return nil, "", err
	}
	u, err := c.requestURL(plain, escaped, cl.query)
	if err != nil {
		return nil, "", err
	}
	path = u.EscapedPath()
	var body io.Reader
	if cl.body != nil {
		data, err := json.Marshal(cl.body)
		if err != nil {
			return nil, path, fmt.Errorf("exante: encoding %s %s request: %w", cl.method, path, err)
		}
		body = bytes.NewReader(data)
	}
	req, err = http.NewRequestWithContext(ctx, cl.method, u.String(), body)
	if err != nil {
		return nil, path, fmt.Errorf("exante: building %s %s request: %w", cl.method, path, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", cmp.Or(cl.accept, "application/json"))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, path, nil
}

func newAPIError(resp *http.Response, method, path string) error {
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	apiErr := &APIError{
		Method:     method,
		Path:       path,
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Raw:        raw,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
	}
	var body ErrorBody
	if json.Unmarshal(raw, &body) == nil && (body.Group != "" || body.Message != "") {
		apiErr.Body = &body
	}
	if readErr != nil {
		return fmt.Errorf("%w (reading the error body: %w)", apiErr, readErr)
	}
	return apiErr
}

// readLimited reads r to the end, failing with [ErrResponseTooLarge] past limit bytes. A body whose announced length
// fits the limit is read into one buffer of exactly that size.
func readLimited(r io.Reader, contentLength, limit int64) ([]byte, error) {
	if contentLength > 0 && contentLength <= limit {
		data := make([]byte, contentLength)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		return data, nil
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrResponseTooLarge
	}
	return data, nil
}

// causeOr returns the context's cancellation cause when the context is done, which names the real reason (a
// deadline, an idle stream) better than the transport error; otherwise err.
func causeOr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", context.Cause(ctx), err)
	}
	return err
}

// discardClose closes a response body whose content was read or is no longer wanted; a close error then carries no
// information the caller could act on.
func discardClose(c io.Closer) {
	_ = c.Close()
}
