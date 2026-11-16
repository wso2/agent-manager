// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package shadowagents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// HTTP limits.
const (
	// DefaultRequestTimeout bounds one HTTP attempt.
	DefaultRequestTimeout = 60 * time.Second
	// MaxJSONBody bounds a JSON response read into memory.
	MaxJSONBody = 32 << 20
	// MaxLine bounds one line of a JSONL file read in memory.
	MaxLine = 8 << 20
	// defaultMaxAttempts is how many times a request is tried on 429/5xx.
	defaultMaxAttempts = 4
	// maxRetryWait is the longest Retry-After honoured; a provider asking for
	// longer leaves the call rate limited instead of stalling the scan.
	maxRetryWait = 60 * time.Second
)

// NewHTTPClient returns the HTTP client every connector and SDK uses. It
// follows a redirect only to the same scheme and host, so a stored
// credential is never sent somewhere the user did not configure.
func NewHTTPClient(rt http.RoundTripper) *http.Client {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{
		Transport:     rt,
		Timeout:       DefaultRequestTimeout,
		CheckRedirect: sameOriginRedirect,
	}
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("stopped after 5 redirects")
	}
	first := via[0].URL
	if req.URL.Scheme != first.Scheme || !strings.EqualFold(req.URL.Host, first.Host) {
		return http.ErrUseLastResponse
	}
	return nil
}

// CallError is a failed provider call, classified for the scan result. Its
// message comes from the provider's error object, scrubbed of anything that
// looks like a credential; a response body is never kept.
type CallError struct {
	Outcome string
	Status  int
	Message string
}

func (e *CallError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s (HTTP %d): %s", e.Outcome, e.Status, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Outcome, e.Message)
}

// OutcomeOf classifies an error for a scan result.
func OutcomeOf(err error) string {
	if err == nil {
		return models.ShadowCallOK
	}
	var ce *CallError
	if errors.As(err, &ce) {
		return ce.Outcome
	}
	return models.ShadowCallFailed
}

// MessageOf returns an error's message, safe to store.
func MessageOf(err error) string {
	if err == nil {
		return ""
	}
	var ce *CallError
	if errors.As(err, &ce) {
		return ScrubText(ce.Error())
	}
	return ScrubText(err.Error())
}

// OutcomeForStatus maps an HTTP status to a call outcome.
func OutcomeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return models.ShadowCallAuthFailed
	case http.StatusForbidden:
		return models.ShadowCallPermissionDenied
	case http.StatusNotFound:
		return models.ShadowCallNotFound
	case http.StatusTooManyRequests:
		return models.ShadowCallRateLimited
	default:
		return models.ShadowCallFailed
	}
}

// Unsupported reports a response that no longer has the documented shape
// (a beta API changed); the call is skipped, the scan goes on.
func Unsupported(format string, args ...any) error {
	return &CallError{Outcome: models.ShadowCallUnsupported, Message: ScrubText(fmt.Sprintf(format, args...))}
}

// API calls one provider's REST API.
type API struct {
	HTTP *http.Client
	// Header sets the auth and version headers on each attempt.
	Header func(h http.Header)
	// Authorize, when set, authorizes each attempt after the headers are
	// set: an OAuth bearer token or a SigV4 signature over req and body. Its
	// error fails the call as auth_failed.
	Authorize func(ctx context.Context, req *http.Request, body []byte) error
	// Logger receives one debug line per attempt: method, host, path,
	// status, attempt. Never a query string, header or body.
	Logger *slog.Logger
	// MaxAttempts defaults to 4.
	MaxAttempts int
	// Sleep waits between attempts; nil uses a timer honouring ctx.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Request is one call.
type Request struct {
	Method string
	URL    string
	Query  url.Values
	// Body is JSON-encoded when not nil.
	Body any
	// Header adds call-specific headers (e.g. a beta header).
	Header http.Header
}

// GetJSON GETs u with query q and decodes the JSON response into out.
func (a *API) GetJSON(ctx context.Context, u string, q url.Values, out any) error {
	return a.DoJSON(ctx, Request{Method: http.MethodGet, URL: u, Query: q}, out)
}

// DoJSON sends r and decodes the JSON response into out.
func (a *API) DoJSON(ctx context.Context, r Request, out any) error {
	resp, err := a.Do(ctx, r)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxJSONBody+1))
	if err != nil {
		return &CallError{Outcome: models.ShadowCallFailed, Message: ScrubText("reading the response: " + err.Error())}
	}
	if len(body) > MaxJSONBody {
		return &CallError{Outcome: models.ShadowCallFailed, Message: "response larger than the allowed size"}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return Unsupported("the response did not match the documented shape: %s", jsonErrorKind(err))
	}
	return nil
}

// EachLine GETs u and calls fn for each non-empty line of the response (a
// JSONL file), in memory, stopping early when fn returns false. Lines are
// bounded by MaxLine and the whole read by maxBytes.
func (a *API) EachLine(ctx context.Context, r Request, maxBytes int64, fn func(line []byte) bool) error {
	resp, err := a.Do(ctx, r)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxBytes))
	sc.Buffer(make([]byte, 64<<10), MaxLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if !fn(line) {
			return nil
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return &CallError{Outcome: models.ShadowCallFailed, Message: ScrubText("reading the file: " + err.Error())}
	}
	return nil
}

// Do sends r, retrying 429 and 5xx (and 529) responses with backoff,
// honouring Retry-After. A non-2xx final response becomes a *CallError; on
// success the caller closes the body.
func (a *API) Do(ctx context.Context, r Request) (*http.Response, error) {
	var payload []byte
	if r.Body != nil {
		b, err := json.Marshal(r.Body)
		if err != nil {
			return nil, fmt.Errorf("encoding the request: %w", err)
		}
		payload = b
	}
	target, err := url.Parse(r.URL)
	if err != nil || target.Host == "" {
		return nil, &CallError{Outcome: models.ShadowCallFailed, Message: "invalid endpoint URL"}
	}
	if len(r.Query) > 0 {
		q := target.Query()
		for k, vs := range r.Query {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		target.RawQuery = encodeQuery(q)
	}
	attempts := a.MaxAttempts
	if attempts <= 0 {
		attempts = defaultMaxAttempts
	}
	var last *CallError
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, r.Method, target.String(), bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("building the request: %w", err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		if a.Header != nil {
			a.Header(req.Header)
		}
		for k, vs := range r.Header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		if a.Authorize != nil {
			if aerr := a.Authorize(ctx, req, payload); aerr != nil {
				return nil, &CallError{Outcome: models.ShadowCallAuthFailed, Message: ScrubText("getting credentials: " + aerr.Error())}
			}
		}
		start := time.Now()
		resp, err := a.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, &CallError{Outcome: models.ShadowCallFailed, Message: "cancelled: " + ctx.Err().Error()}
			}
			last = &CallError{Outcome: models.ShadowCallFailed, Message: ScrubText(networkError(err))}
			a.log(r.Method, target, 0, attempt, time.Since(start))
			if attempt < attempts {
				if werr := a.wait(ctx, backoff(attempt)); werr != nil {
					return nil, last
				}
				continue
			}
			return nil, last
		}
		a.log(r.Method, target, resp.StatusCode, attempt, time.Since(start))
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		last = errorFromResponse(resp)
		retryAfter, hasRetryAfter := parseRetryAfter(resp.Header)
		_ = resp.Body.Close()
		if !retryable(resp.StatusCode) || attempt == attempts {
			return nil, last
		}
		wait := backoff(attempt)
		if hasRetryAfter {
			if retryAfter > maxRetryWait {
				return nil, last
			}
			wait = retryAfter
		}
		if werr := a.wait(ctx, wait); werr != nil {
			return nil, last
		}
	}
	return nil, last
}

func (a *API) wait(ctx context.Context, d time.Duration) error {
	if a.Sleep != nil {
		return a.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (a *API) log(method string, u *url.URL, status, attempt int, took time.Duration) {
	if a.Logger == nil {
		return
	}
	a.Logger.Debug("Shadow agent discovery provider call",
		"method", method, "host", u.Host, "path", u.Path, "status", status, "attempt", attempt, "durationMs", took.Milliseconds())
}

func retryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
		return true
	default:
		return false
	}
}

func backoff(attempt int) time.Duration {
	base := time.Duration(1<<(attempt-1)) * time.Second
	jitter := time.Duration(rand.Int64N(int64(base / 2))) //nolint:gosec // jitter, not security
	return base + jitter
}

// parseRetryAfter reads Retry-After as seconds or an HTTP date, and the
// retry-after-ms header some providers send.
func parseRetryAfter(h http.Header) (time.Duration, bool) {
	if ms := h.Get("retry-after-ms"); ms != "" {
		if n, err := strconv.ParseFloat(ms, 64); err == nil && n >= 0 {
			return time.Duration(n * float64(time.Millisecond)), true
		}
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil && n >= 0 {
		return time.Duration(n * float64(time.Second)), true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// errorFromResponse builds a CallError from the provider's error object.
// Supported shapes: {"error":{"message"}} (OpenAI, Azure, Google),
// {"error":"text"} (some OpenAI admin errors), {"type":"error","error":
// {"message"}} (Anthropic), {"message"} and {"Message"} (AWS REST).
func errorFromResponse(resp *http.Response) *CallError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	msg := providerErrorMessage(body)
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &CallError{Outcome: OutcomeForStatus(resp.StatusCode), Status: resp.StatusCode, Message: ScrubText(msg)}
}

func providerErrorMessage(body []byte) string {
	var shape struct {
		Error        json.RawMessage `json:"error"`
		Message      string          `json:"message"`
		MessageUpper string          `json:"Message"`
	}
	if json.Unmarshal(body, &shape) != nil {
		return ""
	}
	if len(shape.Error) > 0 {
		var s string
		if json.Unmarshal(shape.Error, &s) == nil {
			return s
		}
		var obj struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
			Type    string `json:"type"`
			Status  string `json:"status"`
		}
		if json.Unmarshal(shape.Error, &obj) == nil {
			parts := []string{}
			for _, p := range []string{obj.Type, obj.Status, fmt.Sprint(nilToEmpty(obj.Code))} {
				if p != "" {
					parts = append(parts, p)
				}
			}
			if obj.Message != "" {
				parts = append(parts, obj.Message)
			}
			return strings.Join(parts, ": ")
		}
	}
	if shape.Message != "" {
		return shape.Message
	}
	return shape.MessageUpper
}

func nilToEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func networkError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return "request to " + Origin(ue.URL) + " failed: " + ue.Err.Error()
	}
	return err.Error()
}

func jsonErrorKind(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Sprintf("field %q has an unexpected type", te.Field)
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return "not valid JSON"
	}
	return "decode error"
}

// encodeQuery encodes q like url.Values.Encode but keeps "[" and "]" in
// keys unescaped, the way the providers document their array parameters
// (group_by[]=..., event_types[]=...).
func encodeQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		key := strings.NewReplacer("%5B", "[", "%5D", "]").Replace(url.QueryEscape(k))
		for _, v := range q[k] {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(key)
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}

func jsonUnmarshal(b []byte, v any) error {
	if v == nil {
		return nil
	}
	return json.Unmarshal(b, v)
}
