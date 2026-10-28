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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// errA2ANotProxyable means the card cannot be fronted by the gateway; retrying cannot help.
var errA2ANotProxyable = errors.New("the agent cannot be proxied through the gateway")

// a2aCardInterface is the part of a supportedInterfaces entry route derivation reads.
type a2aCardInterface struct {
	binding string
	url     string
	version json.RawMessage
}

// rewritesAsCurrentVersion is true when the gateway can rewrite the entry: it declares 1.0, or no version.
func (i a2aCardInterface) rewritesAsCurrentVersion() bool {
	if len(i.version) == 0 || string(i.version) == "null" {
		return true
	}
	version, ok := jsonString(i.version)
	return ok && (version == "" || version == a2aProtocolVersion)
}

// deriveA2AProxyRoutes takes the upstream from the card URL and fronts each binding the gateway can rewrite.
func deriveA2AProxyRoutes(cardURL string, card json.RawMessage) (upstreamURL string, transports []A2ATransport, err error) {
	upstream, err := a2aProxyUpstream(cardURL)
	if err != nil {
		return "", nil, err
	}
	interfaces, err := a2aCardInterfaces(card)
	if err != nil {
		return "", nil, err
	}
	var skipped []string
	for _, binding := range []string{a2aTransportJSONRPC, a2aTransportHTTPJSON} {
		transport, reason := a2aProxyTransport(upstream, binding, interfaces)
		if reason != "" {
			skipped = append(skipped, reason)
			continue
		}
		transports = append(transports, transport)
	}
	if len(transports) == 0 {
		return "", nil, fmt.Errorf("%w: no JSONRPC or HTTP+JSON interface can be fronted (%s)",
			errA2ANotProxyable, strings.Join(skipped, "; "))
	}
	return upstream.String(), transports, nil
}

// requireProxyableCardURL is the 400 for a card URL the gateway could never front.
func requireProxyableCardURL(cardURL string) error {
	if _, reason := cardURLUpstream(cardURL); reason != "" {
		return fmt.Errorf("%w: %s", utils.ErrA2ACardURLNotProxyable, reason)
	}
	return nil
}

// a2aProxyUpstream strips the well-known suffix; the gateway fetches the card from upstream plus that suffix.
func a2aProxyUpstream(cardURL string) (*url.URL, error) {
	upstream, reason := cardURLUpstream(cardURL)
	if reason != "" {
		return nil, fmt.Errorf("%w: %s", errA2ANotProxyable, reason)
	}
	return upstream, nil
}

// cardURLUpstream is a2aProxyUpstream reporting why it cannot, so each caller wraps its own sentinel.
func cardURLUpstream(cardURL string) (*url.URL, string) {
	u, err := url.Parse(cardURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, "the card URL must be a plain http(s) URL"
	}
	if u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, a2aAgentCardPath) {
		return nil, fmt.Sprintf("the card URL must end in %s with no query, as the gateway fetches the card there", a2aAgentCardPath)
	}
	base := strings.TrimSuffix(u.Path, a2aAgentCardPath)
	if base != "" && (strings.HasSuffix(base, "/") || path.Clean(base) != base || u.RawPath != "") {
		return nil, "the card URL's path is not canonical"
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host, Path: base}, ""
}

// a2aCardInterfaces reads supportedInterfaces with exact key names, as the gateway's rewrite does.
func a2aCardInterfaces(card json.RawMessage) ([]a2aCardInterface, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(card, &doc); err != nil {
		return nil, fmt.Errorf("%w: the agent card is not a JSON object", errA2ANotProxyable)
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(doc["supportedInterfaces"], &entries); err != nil {
		return nil, fmt.Errorf("%w: the agent card's supportedInterfaces is not a list", errA2ANotProxyable)
	}
	interfaces := make([]a2aCardInterface, 0, len(entries))
	for i, entry := range entries {
		// The gateway's card rewrite fails the whole card on an entry without a binding.
		binding, ok := jsonString(entry["protocolBinding"])
		if !ok || binding == "" {
			return nil, fmt.Errorf("%w: supportedInterfaces[%d] has no protocolBinding", errA2ANotProxyable, i)
		}
		link, _ := jsonString(entry["url"])
		interfaces = append(interfaces, a2aCardInterface{binding: binding, url: link, version: entry["protocolVersion"]})
	}
	return interfaces, nil
}

// a2aProxyTransport fronts one binding whose entries share one URL and declare 1.0 or nothing, or says why it cannot.
func a2aProxyTransport(upstream *url.URL, binding string, interfaces []a2aCardInterface) (A2ATransport, string) {
	var link string
	offered := false
	for _, iface := range interfaces {
		if iface.binding != binding {
			continue
		}
		if !iface.rewritesAsCurrentVersion() {
			return A2ATransport{}, fmt.Sprintf("%s declares protocolVersion %s, not %s", binding, iface.version, a2aProtocolVersion)
		}
		if offered && iface.url != link {
			return A2ATransport{}, binding + " interfaces disagree on the URL"
		}
		link, offered = iface.url, true
	}
	if !offered {
		return A2ATransport{}, binding + " is not offered"
	}
	prefix, ok := a2aPathPrefixUnder(upstream, link)
	if !ok {
		return A2ATransport{}, fmt.Sprintf("%s URL %q is not a plain path under %s", binding, link, upstream)
	}
	return A2ATransport{ProtocolBinding: binding, PathPrefix: prefix}, ""
}

// a2aPathPrefixUnder is link's path below upstream's ("" omits it); ok is false off upstream or against the gateway's prefix rules.
func a2aPathPrefixUnder(upstream *url.URL, link string) (string, bool) {
	u, err := url.Parse(link)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", false
	}
	rest, under := strings.CutPrefix(u.Path, upstream.Path)
	if !isSameOrigin(u, upstream) || !under || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return "", false
	}
	if rest == "" || rest == "/" {
		return "", true
	}
	if path.Clean(rest) != rest || strings.ContainsAny(rest, "{}?#%") || strings.ContainsFunc(rest, unicode.IsSpace) {
		return "", false
	}
	return rest, true
}

// isSameOrigin compares scheme, host and port, defaulting the port by scheme.
func isSameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

// jsonString reads a JSON string; ok is false for anything else, absence included.
func jsonString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}
