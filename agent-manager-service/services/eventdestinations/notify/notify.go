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

// Package notify is the event destination driver that sends each event as a short
// text notification to a chat, email, paging or push service. The services
// themselves are implemented by Shoutrrr (github.com/nicholas-fedor/shoutrrr),
// which its maintainers keep in step with each service's API: this package
// only decides which of them are offered and keeps their traffic safe.
//
// A destination is a service plus a Shoutrrr service URL, such as
// slack://hook:T000/B000/XXX@webhook. The URL carries the service's token, so
// it is the destination's credential: it is stored encrypted and never
// returned.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nicholas-fedor/shoutrrr/pkg/router"
	shoutrrrsmtp "github.com/nicholas-fedor/shoutrrr/pkg/services/email/smtp"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	sendTimeout = 10 * time.Second
)

// service is one Shoutrrr service the driver offers.
type service struct {
	eventdestinations.Service
	// networkHost is set when the URL's host is the server the notification
	// goes to (a self-hosted chat server, an SMTP relay), so it is checked
	// when the destination is saved. Other services put a token or a chat
	// channel name there.
	networkHost bool
	// http is false for services that do not send over HTTP and so cannot
	// use the driver's SSRF-safe client; their host is checked again before
	// every send.
	http bool
}

// offered lists the services destinations can use. A service is offered only if
// Shoutrrr sends to it through the HTTP client the driver supplies (or, for
// SMTP, if its host can be checked before sending); adding one is one entry.
// Give a new service a setup guide path in the console's
// alertingDocLinks.notificationServices (config.template.js), or it shows no link.
var offered = []service{
	svc("slack", "Slack", "Chat", "slack://hook:<webhook-token>@webhook or slack://xoxb:<bot-token>@<channel>", false, true),
	svc("teams", "Microsoft Teams", "Chat", "teams://?host=<Power Automate workflow URL>", false, true),
	svc("googlechat", "Google Chat", "Chat", "googlechat://chat.googleapis.com/v1/spaces/FOO/messages?key=bar&token=baz", false, true),
	svc("discord", "Discord", "Chat", "discord://token@id", false, true),
	svc("telegram", "Telegram", "Chat", "telegram://token@telegram?chats=@channel", false, true),
	svc("mattermost", "Mattermost", "Chat", "mattermost://[username@]mattermost-host/token[/channel]", true, true),
	svc("rocketchat", "Rocket.Chat", "Chat", "rocketchat://[username@]rocketchat-host/token[/channel]", true, true),
	svc("zulip", "Zulip", "Chat", "zulip://bot-mail:bot-key@zulip-domain/?stream=name&topic=name", true, true),
	svc("smtp", "Email (SMTP)", "Email", "smtp://username:password@host:port/?fromaddress=from@example.com&toaddresses=to@example.com", true, false),
	svc("pagerduty", "PagerDuty", "Incident", "pagerduty://[host[:port]]/integration-key", false, true),
	svc("opsgenie", "Opsgenie", "Incident", "opsgenie://api.opsgenie.com/token?responders=team:ops", false, true),
	svc("ntfy", "ntfy", "Push", "ntfy://[username:password@]ntfy.sh/topic", true, true),
	svc("gotify", "Gotify", "Push", "gotify://gotify-host/token", true, true),
	svc("pushover", "Pushover", "Push", "pushover://shoutrrr:apiToken@userKey/?devices=device", false, true),
	svc("twilio", "Twilio SMS", "SMS", "twilio://accountSID:authToken@fromNumber/toNumber", false, true),
}

func svc(id, label, category, format string, networkHost, overHTTP bool) service {
	return service{
		Service: eventdestinations.Service{
			ID: id, Label: label, Category: category, URLFormat: format,
		},
		networkHost: networkHost,
		http:        overHTTP,
	}
}

func lookup(id string) (service, bool) {
	for _, s := range offered {
		if s.ID == id {
			return s, true
		}
	}
	return service{}, false
}

// Config is a notification destination's settings as given: the service
// and, when creating the destination or replacing its URL, the service URL.
type Config struct {
	Service string `json:"service"`
	URL     string `json:"url,omitempty"`
}

// Driver delivers to notification destinations.
type Driver struct {
	client       types.HTTPClient
	allowPrivate bool
}

var (
	_ eventdestinations.CredentialDriver = (*Driver)(nil)
	_ eventdestinations.ServiceCatalog   = (*Driver)(nil)
)

// New builds the driver. Unless allowPrivate is set, every service's traffic
// goes through an SSRF-hardened client, and hosts named in a URL must resolve
// to public addresses. allowPrivate is for local development only.
func New(allowPrivate bool) *Driver {
	var client types.HTTPClient = ssrf.NewClient(sendTimeout)
	if allowPrivate {
		client = &http.Client{Timeout: sendTimeout}
	}
	return &Driver{client: client, allowPrivate: allowPrivate}
}

func (d *Driver) Kind() string { return models.DestinationKindNotify }

// Services lists the offered services.
func (d *Driver) Services() []eventdestinations.Service {
	out := make([]eventdestinations.Service, len(offered))
	for i, s := range offered {
		out[i] = s.Service
	}
	return out
}

// NormalizeConfig checks the service and, if given, its URL. The URL may be
// left out when updating a destination, to keep the one it has.
func (d *Driver) NormalizeConfig(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: notify config must be an object with a service", utils.ErrInvalidInput)
	}
	cfg.Service = strings.TrimSpace(cfg.Service)
	cfg.URL = strings.TrimSpace(cfg.URL)
	s, ok := lookup(cfg.Service)
	if !ok {
		return nil, fmt.Errorf("%w: service must be one of %s", utils.ErrInvalidInput, strings.Join(serviceIDs(), ", "))
	}
	if cfg.URL != "" {
		if err := d.validateURL(ctx, s, cfg.URL); err != nil {
			return nil, err
		}
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode notify config: %w", err)
	}
	return out, nil
}

// SplitCredential stores only the service in plain; the URL is the credential.
func (d *Driver) SplitCredential(raw json.RawMessage) (json.RawMessage, string) {
	cfg, _ := parseConfig(raw)
	stored, _ := json.Marshal(Config{Service: cfg.Service})
	return stored, cfg.URL
}

// RedactConfig has nothing to mask: the stored settings name only the service.
func (d *Driver) RedactConfig(raw json.RawMessage) json.RawMessage {
	cfg, err := parseConfig(raw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	out, _ := json.Marshal(Config{Service: cfg.Service})
	return out
}

func (d *Driver) Origin(raw json.RawMessage) string {
	cfg, err := parseConfig(raw)
	if err != nil {
		return ""
	}
	return cfg.Service
}

// NewSecret: notification destinations have no generated secret; their URL is a
// credential the user supplies.
func (d *Driver) NewSecret() (string, bool, error) { return "", false, nil }

// Deliver sends msg, rendered as text, to the service URL in dest.Secret.
func (d *Driver) Deliver(ctx context.Context, dest eventdestinations.Destination, msg eventdestinations.Message) eventdestinations.Result {
	cfg, err := parseConfig(dest.Config)
	if err != nil {
		return eventdestinations.Result{Err: fmt.Errorf("invalid notify config: %w", err)}
	}
	s, ok := lookup(cfg.Service)
	if !ok {
		return eventdestinations.Result{Err: fmt.Errorf("notify service %q is no longer offered", cfg.Service)}
	}
	if dest.Secret == "" {
		return eventdestinations.Result{Err: errors.New("notify destination has no service URL")}
	}
	if !s.http {
		// Not covered by the SSRF-safe client: check the host again, as its
		// DNS may have changed since the destination was saved.
		if err := d.checkHost(ctx, dest.Secret); err != nil {
			return eventdestinations.Result{Err: err}
		}
	}
	sender, err := d.locate(dest.Secret)
	if err != nil {
		return eventdestinations.Result{Err: fmt.Errorf("invalid %s URL: %s", s.ID, scrub(err, dest.Secret))}
	}
	text := eventdestinations.Render(msg)
	params := types.Params{}
	params.SetTitle(text.Title)
	body := text.Title + "\n" + text.Body

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		if smtpSender, ok := sender.(*shoutrrrsmtp.Service); ok {
			done <- deliverSMTP(ctx, smtpSender.Config, body, &params, d.allowPrivate)
			return
		}
		if cs, ok := sender.(types.ContextSender); ok {
			done <- cs.SendContext(ctx, body, &params)
			return
		}
		done <- sender.Send(body, &params)
	}()
	select {
	case err = <-done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		// Shoutrrr reports no status code; a send that failed may succeed
		// later (the service was down, or rate limited the request).
		var permanent permanentSMTPError
		return eventdestinations.Result{Err: fmt.Errorf("%s notification failed: %s", s.ID, scrub(err, dest.Secret)), Retryable: !errors.As(err, &permanent)}
	}
	return eventdestinations.Result{}
}

// locate builds the Shoutrrr service for rawURL with the driver's client.
func (d *Driver) locate(rawURL string) (types.Service, error) {
	r, err := router.NewWithOptions(nil, types.SenderOptions{HTTPClient: d.client, Timeout: sendTimeout})
	if err != nil {
		return nil, err
	}
	return r.Locate(rawURL)
}

func (d *Driver) validateURL(ctx context.Context, s service, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: url is not a valid %s URL", utils.ErrInvalidInput, s.Label)
	}
	// "discord+https://…" is Shoutrrr's form for pasting the service's own
	// webhook URL; the part before "+" names the service.
	if scheme, _, _ := strings.Cut(u.Scheme, "+"); scheme != s.ID {
		return fmt.Errorf("%w: a %s URL starts with %s://", utils.ErrInvalidInput, s.Label, s.ID)
	}
	if _, err := d.locate(rawURL); err != nil {
		return fmt.Errorf("%w: url is not a valid %s URL: %s", utils.ErrInvalidInput, s.Label, scrub(err, rawURL))
	}
	if s.networkHost && !strings.Contains(u.Scheme, "+") {
		if err := d.checkHost(ctx, rawURL); err != nil {
			return fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
		}
	}
	return nil
}

// checkHost refuses a URL whose host does not resolve to public addresses.
func (d *Driver) checkHost(ctx context.Context, rawURL string) error {
	if d.allowPrivate {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return errors.New("the URL names no host")
	}
	if err := ssrf.ValidateHost(ctx, u.Hostname()); err != nil {
		return fmt.Errorf("host %q is not allowed: it must resolve to public addresses", u.Hostname())
	}
	return nil
}

// scrub removes the service URL, and its user info, from an error: Shoutrrr
// errors can quote it, and it carries the service's token. The text is
// logged and shown in the delivery log.
func scrub(err error, rawURL string) string {
	msg := err.Error()
	// Services rebuild the request URL from the token (Slack turns
	// "A-B-C" into ".../services/A/B/C"), so the *url.Error of a failed
	// request quotes the credential in a form the replacements below miss.
	for e := err; e != nil; {
		var urlErr *url.Error
		if !errors.As(e, &urlErr) {
			break
		}
		msg = strings.ReplaceAll(msg, urlErr.Error(), urlErr.Op+" <url>: "+urlErr.Err.Error())
		e = urlErr.Err
	}
	if rawURL != "" {
		msg = strings.ReplaceAll(msg, rawURL, "<url>")
	}
	if u, perr := url.Parse(rawURL); perr == nil {
		if u.User != nil {
			msg = strings.ReplaceAll(msg, u.User.String(), "<credentials>")
			if p, ok := u.User.Password(); ok && p != "" {
				msg = strings.ReplaceAll(msg, p, "<credentials>")
			}
		}
		if u.Path != "" && u.Path != "/" {
			msg = strings.ReplaceAll(msg, u.Path, "/<path>")
		}
		if u.RawQuery != "" {
			msg = strings.ReplaceAll(msg, u.RawQuery, "<query>")
		}
		// Service URLs can embed the actual endpoint in a query value (Teams
		// uses host=<workflow URL>). HTTP errors quote that decoded endpoint,
		// not the original service URL or its encoded query string.
		for _, values := range u.Query() {
			for _, value := range values {
				if value != "" {
					msg = strings.ReplaceAll(msg, value, "<query-value>")
				}
			}
		}
	}
	return msg
}

func serviceIDs() []string {
	ids := make([]string, len(offered))
	for i, s := range offered {
		ids[i] = s.ID
	}
	return ids
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
