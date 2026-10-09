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

// A2ACardFetcher fetches and minimally validates an A2A 1.0 public agent card.
type A2ACardFetcher interface {
	// Fetch returns the card as served. guarded selects the SSRF-hardened client for user-supplied URLs.
	Fetch(ctx context.Context, url string, guarded bool) (json.RawMessage, error)
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

func (f *a2aCardFetcher) Fetch(ctx context.Context, url string, guarded bool) (json.RawMessage, error) {
	client := f.plain
	if guarded {
		client = f.guarded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid agent card URL: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		if guarded {
			if errors.Is(err, utils.ErrInvalidURL) {
				return nil, refusedCardURL(err)
			}
			// Raw dial errors are not surfaced for user-supplied hosts.
			return nil, errors.New("could not reach the agent card URL")
		}
		return nil, fmt.Errorf("could not reach the agent card URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("agent card request returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, a2aCardMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read the agent card: %w", err)
	}
	if len(body) > a2aCardMaxBytes {
		return nil, errors.New("agent card exceeds 1 MiB")
	}
	if err := validateA2ACard(body); err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// refusedCardURL keeps SSRF refusals free of DNS detail, since last_error is user-visible.
func refusedCardURL(err error) error {
	if errors.Is(err, ssrf.ErrHostNotPublic) {
		return errors.New("agent card URL is not allowed: " + a2aCardHostNotPublicMsg)
	}
	return fmt.Errorf("agent card URL is not allowed: %w", err)
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
