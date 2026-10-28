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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	// a2aAgentCardPath is the A2A 1.0 public card path; there is no agent.json fallback.
	a2aAgentCardPath    = "/.well-known/agent-card.json"
	a2aCardFetchTimeout = 5 * time.Second
	a2aCardMaxBytes     = 1 << 20
)

// A2ACardFetchHeader is a credential sent on a card fetch.
type A2ACardFetchHeader struct {
	Name  string
	Value string
}

// A2ACardFetcher fetches and minimally validates an A2A 1.0 public agent card.
type A2ACardFetcher interface {
	// Fetch returns the card as served from a platform-derived URL, unguarded.
	Fetch(ctx context.Context, url string) (json.RawMessage, error)
	// FetchOrigin fetches a user-supplied URL SSRF-guarded; with a header, redirects stay on the URL's origin.
	FetchOrigin(ctx context.Context, url string, header *A2ACardFetchHeader) (json.RawMessage, error)
}

type a2aCardFetcher struct {
	plain   *http.Client
	guarded *http.Client
}

// NewA2ACardFetcher creates an A2ACardFetcher with the spec's 5s timeout.
func NewA2ACardFetcher() A2ACardFetcher {
	return newA2ACardFetcher(a2aCardFetchTimeout, config.GetConfig().A2ACardFetchDialAddr)
}

func newA2ACardFetcher(timeout time.Duration, dialAddr string) *a2aCardFetcher {
	return &a2aCardFetcher{
		// Platform URLs resolve to loopback in local setups, which the SSRF guard rejects.
		plain:   &http.Client{Timeout: timeout, Transport: plainTransport(dialAddr), CheckRedirect: sameHostRedirectOnly},
		guarded: ssrf.NewClient(timeout),
	}
}

// plainTransport ignores proxy env so a tenant cannot steer the platform fetch.
func plainTransport(dialAddr string) http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	if dialAddr != "" {
		t.DialContext = pinnedDial(dialAddr)
	}
	return t
}

// pinnedDial dials addr regardless of the URL host, keeping the Host header intact (local dev: AMS in a container).
func pinnedDial(addr string) func(ctx context.Context, network, _ string) (net.Conn, error) {
	d := &net.Dialer{Timeout: a2aCardFetchTimeout}
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		return d.DialContext(ctx, network, addr)
	}
}

// sameHostRedirectOnly stops redirects that leave the original scheme and host, except a default-port HTTPS upgrade.
func sameHostRedirectOnly(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	from, to := via[0].URL, req.URL
	sameOrigin := to.Scheme == from.Scheme && to.Host == from.Host
	upgrade := from.Scheme == "http" && to.Scheme == "https" && to.Hostname() == from.Hostname() &&
		(from.Port() == "" || from.Port() == "80") && (to.Port() == "" || to.Port() == "443")
	if !sameOrigin && !upgrade {
		return http.ErrUseLastResponse
	}
	return nil
}

// refusedRedirectHost names where a 3xx the client did not follow pointed, or "".
func refusedRedirectHost(resp *http.Response) string {
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return ""
	}
	loc, err := resp.Location()
	if err != nil {
		return ""
	}
	return loc.Host
}

func (f *a2aCardFetcher) Fetch(ctx context.Context, url string) (json.RawMessage, error) {
	return f.fetch(ctx, f.plain, url, false, nil)
}

func (f *a2aCardFetcher) FetchOrigin(ctx context.Context, url string, header *A2ACardFetchHeader) (json.RawMessage, error) {
	client := f.guarded
	if header != nil {
		client = sameOriginClient(f.guarded)
	}
	return f.fetch(ctx, client, url, true, header)
}

// sameOriginClient is base that additionally refuses redirects off the original origin.
func sameOriginClient(base *http.Client) *http.Client {
	c := *base
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if base.CheckRedirect != nil {
			if err := base.CheckRedirect(req, via); err != nil {
				return err
			}
		}
		return sameHostRedirectOnly(req, via)
	}
	return &c
}

func (f *a2aCardFetcher) fetch(
	ctx context.Context, client *http.Client, url string, guarded bool, header *A2ACardFetchHeader,
) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, newA2APublicError("the agent card URL is invalid", err)
	}
	req.Header.Set("Accept", "application/json")
	if header != nil {
		req.Header.Set(header.Name, header.Value)
	}

	resp, err := client.Do(req)
	if err != nil {
		if guarded && errors.Is(err, utils.ErrInvalidURL) {
			return nil, refusedCardURL(err)
		}
		// The dial error can name internal addresses; the cause is logged via a2aLogError, never shown.
		return nil, newA2APublicError("could not reach the agent card URL", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if target := refusedRedirectHost(resp); target != "" {
		return nil, newA2APublicError(fmt.Sprintf(
			"agent card request was redirected to %s; redirects to another host are not followed", target), nil)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newA2APublicError(fmt.Sprintf("agent card request returned HTTP %d", resp.StatusCode), nil)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, a2aCardMaxBytes+1))
	if err != nil {
		return nil, newA2APublicError("failed to read the agent card", err)
	}
	if len(body) > a2aCardMaxBytes {
		return nil, newA2APublicError("agent card exceeds 1 MiB", nil)
	}
	if err := validateA2ACard(body); err != nil {
		return nil, newA2APublicError(err.Error(), nil)
	}
	return json.RawMessage(body), nil
}

// refusedCardURL keeps SSRF refusals free of DNS detail, since last_error is user-visible.
func refusedCardURL(err error) error {
	if errors.Is(err, ssrf.ErrHostNotPublic) {
		return newA2APublicError("agent card URL is not allowed: "+a2aCardHostNotPublicMsg, err)
	}
	return newA2APublicError("agent card URL is not allowed: "+err.Error(), err)
}

// validateA2ACard checks the minimal A2A 1.0 shape with exact key names, as the console reads them.
func validateA2ACard(body []byte) error {
	var card map[string]json.RawMessage
	if err := json.Unmarshal(body, &card); err != nil || card == nil {
		return errors.New("agent card is not a JSON object")
	}
	var name string
	if err := json.Unmarshal(card["name"], &name); err != nil || name == "" {
		return errors.New(`agent card is missing "name"`)
	}
	var interfaces []map[string]json.RawMessage
	if err := json.Unmarshal(card["supportedInterfaces"], &interfaces); err != nil || len(interfaces) == 0 {
		return errors.New(`agent card has no "supportedInterfaces"`)
	}
	for i, iface := range interfaces {
		var url string
		if err := json.Unmarshal(iface["url"], &url); err != nil || url == "" {
			return fmt.Errorf(`agent card "supportedInterfaces[%d]" has no "url"`, i)
		}
	}
	var skills []json.RawMessage
	if err := json.Unmarshal(card["skills"], &skills); err != nil || skills == nil {
		return errors.New(`agent card is missing the "skills" array`)
	}
	return nil
}
