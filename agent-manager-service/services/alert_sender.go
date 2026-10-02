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

package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	alertSendTimeout = 10 * time.Second

	// Headers set on every alert request. Custom headers cannot override them.
	AlertHeaderSignature = "X-AMP-Signature"
	AlertHeaderEventID   = "X-AMP-Event-Id"
	AlertHeaderEventType = "X-AMP-Event-Type"
	alertUserAgent       = "WSO2-Agent-Manager-Alerts/1"
)

// alertHeaderNamePattern is an RFC 7230 header field name token.
var alertHeaderNamePattern = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_\x60|~-]+$`)

// reservedAlertHeaders are set by the sender and must not be supplied by users.
var reservedAlertHeaders = map[string]bool{
	"content-type":   true,
	"content-length": true,
	"host":           true,
	"user-agent":     true,
}

// AlertTarget is a resolved, decrypted alert endpoint.
type AlertTarget struct {
	URL           string
	Headers       map[string]string
	SigningSecret string
}

// AlertSender delivers one alert request and returns the HTTP status code.
// A non-nil error means the request did not produce a 2xx response.
type AlertSender interface {
	Send(ctx context.Context, target AlertTarget, eventID, eventType string, body []byte) (int, error)
	// ValidateURL checks that an endpoint URL is acceptable to send to.
	ValidateURL(ctx context.Context, rawURL string) error
}

type httpAlertSender struct {
	client       *http.Client
	allowPrivate bool
	now          func() time.Time
}

// NewAlertSender builds the HTTP sender. Unless private endpoints are allowed
// by config, the client refuses to reach non-public addresses, re-checking at
// dial time and on every redirect.
func NewAlertSender(cfg config.Config) AlertSender {
	allowPrivate := cfg.Alerting.AllowPrivateEndpoints
	client := ssrf.NewClient(alertSendTimeout)
	if allowPrivate {
		client = &http.Client{Timeout: alertSendTimeout}
	} else {
		client.CheckRedirect = httpsOnlyRedirect(client.CheckRedirect)
	}
	return &httpAlertSender{client: client, allowPrivate: allowPrivate, now: time.Now}
}

// httpsOnlyRedirect refuses any redirect hop that leaves https or leaves the
// configured host, before running the SSRF check. Go keeps Authorization on a
// same-host redirect and every custom header (an X-Api-Key, say) on any
// redirect, so either hop would hand the stored headers to a destination they
// were not entered for.
func httpsOnlyRedirect(next func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("%w: alert endpoint redirected to a non-https URL", utils.ErrInvalidURL)
		}
		if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			return fmt.Errorf("%w: alert endpoint redirected to another host", utils.ErrInvalidURL)
		}
		return next(req, via)
	}
}

func (s *httpAlertSender) ValidateURL(ctx context.Context, rawURL string) error {
	if !s.allowPrivate {
		// Alerts carry the stored headers (often an Authorization token), so a
		// public endpoint must use TLS. Checked before the SSRF lookup so a
		// plain-http URL is rejected without resolving it.
		if parsed, err := url.Parse(rawURL); err != nil || parsed.Scheme != "https" {
			return fmt.Errorf("%w: alert endpoint must use https", utils.ErrInvalidInput)
		}
		if err := ssrf.ValidateURL(ctx, rawURL); err != nil {
			return fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
		}
		return nil
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%w: url must be an absolute http or https URL", utils.ErrInvalidInput)
	}
	return nil
}

func (s *httpAlertSender) Send(ctx context.Context, target AlertTarget, eventID, eventType string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to build alert request: %w", err)
	}
	for name, value := range target.Headers {
		req.Header.Set(name, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", alertUserAgent)
	req.Header.Set(AlertHeaderEventID, eventID)
	req.Header.Set(AlertHeaderEventType, eventType)
	req.Header.Set(AlertHeaderSignature, SignAlertPayload(target.SigningSecret, s.now(), body))

	resp, err := s.client.Do(req)
	if err != nil {
		// A *url.Error prints the full request URL, and webhook URLs often
		// carry their credential in the path (Slack, Teams). Keep only the
		// cause, since this text is logged and stored on the delivery row.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, fmt.Errorf("alert request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain a bounded amount so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("alert endpoint returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// SignAlertPayload returns the X-AMP-Signature value: "t=<unix>,v1=<hex>",
// where v1 is HMAC-SHA256 over "<unix>.<body>" keyed by the signing secret.
// Receivers recompute it to verify origin and reject stale timestamps.
func SignAlertPayload(secret string, at time.Time, body []byte) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// validateAlertHeaders rejects malformed header names and names the sender owns.
func validateAlertHeaders(headers map[string]string) error {
	for name, value := range headers {
		if !alertHeaderNamePattern.MatchString(name) {
			return fmt.Errorf("%w: invalid header name %q", utils.ErrInvalidInput, name)
		}
		lower := strings.ToLower(name)
		if reservedAlertHeaders[lower] || strings.HasPrefix(lower, "x-amp-") {
			return fmt.Errorf("%w: header %q is reserved", utils.ErrInvalidInput, name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%w: header %q has an invalid value", utils.ErrInvalidInput, name)
		}
	}
	return nil
}
