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
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	webhookSendTimeout = 10 * time.Second
	webhookUserAgent   = "WSO2-Agent-Manager-Webhooks/1"

	// Standard Webhooks headers (https://www.standardwebhooks.com). Receivers
	// can verify them with any Standard Webhooks library.
	WebhookHeaderID        = "webhook-id"
	WebhookHeaderTimestamp = "webhook-timestamp"
	WebhookHeaderSignature = "webhook-signature"
	// WebhookHeaderEventType carries the CloudEvents type, for routing
	// without parsing the body.
	WebhookHeaderEventType = "X-AMP-Event-Type"

	// WebhookSecretPrefix marks a signing secret; the rest is base64.
	WebhookSecretPrefix = "whsec_"
)

// WebhookTarget is a resolved endpoint ready to send to.
type WebhookTarget struct {
	URL    string
	Secret string
}

// WebhookSender sends one signed request and returns the HTTP status code.
// A non-nil error means the endpoint did not answer 2xx.
type WebhookSender interface {
	Send(ctx context.Context, target WebhookTarget, messageID, eventType string, body []byte) (int, error)
	// ValidateURL checks that an endpoint URL is acceptable to send to.
	ValidateURL(ctx context.Context, rawURL string) error
}

type httpWebhookSender struct {
	client       *http.Client
	allowPrivate bool
	now          func() time.Time
}

// NewWebhookSender builds the sender. Unless private endpoints are allowed by
// config, the client refuses to reach non-public addresses, checking again at
// dial time and on every redirect.
func NewWebhookSender(cfg config.Config) WebhookSender {
	allowPrivate := cfg.Webhooks.AllowPrivateEndpoints
	client := ssrf.NewClient(webhookSendTimeout)
	if allowPrivate {
		client = &http.Client{Timeout: webhookSendTimeout}
	} else {
		client.CheckRedirect = httpsSameHostRedirect(client.CheckRedirect)
	}
	return &httpWebhookSender{client: client, allowPrivate: allowPrivate, now: time.Now}
}

// httpsSameHostRedirect refuses a redirect that leaves https or the original
// host, before running the SSRF check.
func httpsSameHostRedirect(next func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("%w: webhook endpoint redirected to a non-https URL", utils.ErrInvalidURL)
		}
		if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			return fmt.Errorf("%w: webhook endpoint redirected to another host", utils.ErrInvalidURL)
		}
		if next != nil {
			return next(req, via)
		}
		return nil
	}
}

func (s *httpWebhookSender) ValidateURL(ctx context.Context, rawURL string) error {
	if !s.allowPrivate {
		if parsed, err := url.Parse(rawURL); err != nil || parsed.Scheme != "https" {
			return fmt.Errorf("%w: url must use https", utils.ErrInvalidInput)
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

func (s *httpWebhookSender) Send(ctx context.Context, target WebhookTarget, messageID, eventType string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to build webhook request: %w", err)
	}
	ts := s.now()
	signature, err := SignWebhook(target.Secret, messageID, ts, body)
	if err != nil {
		return 0, err
	}
	// Structured-mode CloudEvent: the body is the whole event.
	req.Header.Set("Content-Type", events.MediaType)
	req.Header.Set("User-Agent", webhookUserAgent)
	req.Header.Set(WebhookHeaderID, messageID)
	req.Header.Set(WebhookHeaderTimestamp, strconv.FormatInt(ts.Unix(), 10))
	req.Header.Set(WebhookHeaderSignature, signature)
	req.Header.Set(WebhookHeaderEventType, eventType)

	resp, err := s.client.Do(req)
	if err != nil {
		// A *url.Error prints the full request URL, and webhook URLs often
		// carry a credential in the path. Keep only the cause: this text is
		// logged and shown in the delivery log.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("webhook endpoint returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// SignWebhook returns the Standard Webhooks signature header value:
// "v1,<base64 HMAC-SHA256 of '<id>.<unix timestamp>.<body>'>", keyed by the
// base64 part of the whsec_ secret.
func SignWebhook(secret, messageID string, at time.Time, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, WebhookSecretPrefix))
	if err != nil {
		return "", fmt.Errorf("invalid webhook signing secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(messageID))
	mac.Write([]byte("."))
	mac.Write([]byte(strconv.FormatInt(at.Unix(), 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}
