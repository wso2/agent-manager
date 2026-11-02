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

// Package webhook is the event destination driver that delivers each event as a
// signed HTTPS POST, following Standard Webhooks
// (https://www.standardwebhooks.com): receivers can verify it with any
// Standard Webhooks library.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	sendTimeout = 10 * time.Second
	userAgent   = "WSO2-Agent-Manager-Events/1"
	secretBytes = 32

	// Standard Webhooks headers.
	HeaderID        = "webhook-id"
	HeaderTimestamp = "webhook-timestamp"
	HeaderSignature = "webhook-signature"
	// HeaderEventType carries the CloudEvents type, for routing without
	// parsing the body.
	HeaderEventType = "X-AMP-Event-Type"

	// SecretPrefix marks a signing secret; the rest is base64.
	SecretPrefix = "whsec_"
)

// Config is a webhook destination's settings.
type Config struct {
	URL string `json:"url"`
}

// Driver delivers to webhook destinations.
type Driver struct {
	client       *http.Client
	allowPrivate bool
	now          func() time.Time
}

var _ eventdestinations.Driver = (*Driver)(nil)

// New builds the driver. Unless allowPrivate is set, it refuses URLs that are
// not https or that resolve to non-public addresses, checking again at dial
// time and on every redirect. allowPrivate is for local development only.
func New(allowPrivate bool) *Driver {
	client := ssrf.NewClient(sendTimeout)
	if allowPrivate {
		client = &http.Client{Timeout: sendTimeout}
	} else {
		client.CheckRedirect = httpsSameHostRedirect(client.CheckRedirect)
	}
	return &Driver{client: client, allowPrivate: allowPrivate, now: time.Now}
}

func (d *Driver) Kind() string { return models.DestinationKindWebhook }

func (d *Driver) NormalizeConfig(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: webhook config must be an object with a url", utils.ErrInvalidInput)
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	if err := d.validateURL(ctx, cfg.URL); err != nil {
		return nil, err
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode webhook config: %w", err)
	}
	return out, nil
}

// RedactConfig keeps scheme and host: a webhook path or query often carries
// a credential.
func (d *Driver) RedactConfig(raw json.RawMessage) json.RawMessage {
	cfg, err := parseConfig(raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	masked := d.Origin(raw)
	if u, err := url.Parse(cfg.URL); err == nil && ((u.Path != "" && u.Path != "/") || u.RawQuery != "") {
		masked += "/…"
	}
	out, _ := json.Marshal(Config{URL: masked})
	return out
}

func (d *Driver) Origin(raw json.RawMessage) string {
	cfg, err := parseConfig(raw)
	if err != nil {
		return ""
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func (d *Driver) NewSecret() (string, bool, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", false, fmt.Errorf("failed to generate webhook signing secret: %w", err)
	}
	return SecretPrefix + base64.StdEncoding.EncodeToString(buf), true, nil
}

func (d *Driver) Deliver(ctx context.Context, dest eventdestinations.Destination, msg eventdestinations.Message) eventdestinations.Result {
	cfg, err := parseConfig(dest.Config)
	if err != nil {
		return eventdestinations.Result{Err: fmt.Errorf("invalid webhook config: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(msg.Body))
	if err != nil {
		// The error would quote the URL, which may carry a credential.
		return eventdestinations.Result{Err: errors.New("failed to build webhook request: invalid URL")}
	}
	ts := d.now()
	signature, err := Sign(dest.Secret, msg.ID, ts, msg.Body)
	if err != nil {
		return eventdestinations.Result{Err: err}
	}
	// Structured-mode CloudEvent: the body is the whole event.
	req.Header.Set("Content-Type", events.MediaType)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set(HeaderID, msg.ID)
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts.Unix(), 10))
	req.Header.Set(HeaderSignature, signature)
	req.Header.Set(HeaderEventType, msg.Type)

	resp, err := d.client.Do(req)
	if err != nil {
		// A *url.Error prints the full request URL, and webhook URLs often
		// carry a credential in the path. Keep only the cause: this text is
		// logged and shown in the delivery log.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return eventdestinations.Result{Err: fmt.Errorf("webhook request failed: %w", err), Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return eventdestinations.Result{
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("webhook endpoint returned HTTP %d", resp.StatusCode),
			Retryable:  retryableStatus(resp.StatusCode),
		}
	}
	return eventdestinations.Result{StatusCode: resp.StatusCode}
}

// Sign returns the Standard Webhooks signature header value:
// "v1,<base64 HMAC-SHA256 of '<id>.<unix timestamp>.<body>'>", keyed by the
// base64 part of the whsec_ secret.
func Sign(secret, messageID string, at time.Time, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, SecretPrefix))
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

func (d *Driver) validateURL(ctx context.Context, rawURL string) error {
	if !d.allowPrivate {
		if parsed, err := url.Parse(rawURL); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("%w: url must be an https URL", utils.ErrInvalidInput)
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

func parseConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	if len(raw) == 0 {
		return cfg, errors.New("empty config")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// retryableStatus reports whether a failed response is worth retrying. A
// 5xx, 408, 425 or 429 is; other 4xx answers mean the request itself was
// refused and will be refused again.
func retryableStatus(code int) bool {
	return code >= 500 || code == 408 || code == 425 || code == 429
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
