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
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Retention limits.
const (
	maxAttrString   = 256
	maxAttrListLen  = 50
	maxMessageChars = 300
	maxNameChars    = 200
)

// Attribute keys a finding may store. Anything else a connector sets is
// dropped before storage: an allow-list, so a new provider field cannot
// slip a prompt or a secret into the database.
var allowedAttributes = map[string]bool{
	"model":             true, // model or deployment name
	"framework":         true, // agent framework (Google spec.agentFramework, registry framework)
	"agentKind":         true, // Foundry kind (prompt, hosted, external, ...), AgentCore artifact kind
	"agentVersion":      true,
	"orchestrationType": true, // Bedrock classic agent
	"serverProtocol":    true, // AgentCore runtime protocol (HTTP, A2A, AGUI)
	"protocolType":      true, // Google Agent Registry protocols
	"recordType":        true, // AWS Agent Registry record type
	"tools":             true, // []ToolRef: type and name only
	"toolCount":         true,
	"mcpServers":        true, // []MCPServerRef: label and origin only
	"skills":            true, // names or IDs only
	"image":             true, // container image URI
	"repositoryUrl":     true, // source repository (origin + path, no credentials)
	"endpoint":          true, // Foundry project endpoint origin
	"region":            true,
	"location":          true,
	"accountId":         true, // cloud account / subscription
	"accountKind":       true, // Azure account kind
	"resourceId":        true, // ARM ID / ARN / resource name
	"roleArn":           true, // AgentCore runtime / classic agent / Managed Agent role
	"serviceAccount":    true, // Google engine identity
	"project":           true, // OpenAI project / Google project / Foundry project
	"workspace":         true, // Anthropic workspace
	"deployment":        true, // Foundry model deployment name
	"archived":          true,
	"state":             true,
	"status":            true,
	"labels":            true, // platform labels (keys and values, short)
	"otelAgentId":       true, // Foundry external agent telemetry ID
	"description":       true, // the platform's short description field (not instructions)
	"displayName":       true,
	"runtimeReference":  true, // Google registry -> engine reference
}

// AllowedAttributes returns the attributes whose keys are on the allow-list,
// so a stored attribute an earlier version kept is not shown.
func AllowedAttributes(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if allowedAttributes[k] {
			out[k] = v
		}
	}
	return out
}

// ToolRef is the part of a tool definition that may be stored: its type and
// its name. Schemas, arguments, headers and credentials never are.
type ToolRef struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// MCPServerRef is the part of an MCP server reference that may be stored:
// its label and the scheme and host of its URL.
type MCPServerRef struct {
	Label  string `json:"label,omitempty"`
	Origin string `json:"origin,omitempty"`
}

// SanitizeAttributes keeps the allowed keys, with values reduced to short
// strings, numbers, booleans and short lists of them, or ToolRef/MCPServerRef
// lists. Everything else is dropped.
func SanitizeAttributes(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if !allowedAttributes[k] {
			continue
		}
		if clean, ok := sanitizeValue(v); ok {
			out[k] = clean
		}
	}
	return out
}

func sanitizeValue(v any) (any, bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case string:
		if x == "" {
			return nil, false
		}
		return Truncate(x, maxAttrString), true
	case bool, int, int32, int64, float32, float64:
		return x, true
	case []string:
		out := make([]string, 0, min(len(x), maxAttrListLen))
		for _, s := range x {
			if len(out) == maxAttrListLen {
				break
			}
			if s != "" {
				out = append(out, Truncate(s, maxAttrString))
			}
		}
		return out, len(out) > 0
	case []ToolRef:
		out := make([]ToolRef, 0, min(len(x), maxAttrListLen))
		for _, t := range x {
			if len(out) == maxAttrListLen {
				break
			}
			out = append(out, ToolRef{Type: Truncate(t.Type, 64), Name: Truncate(t.Name, 128)})
		}
		return out, len(out) > 0
	case []MCPServerRef:
		out := make([]MCPServerRef, 0, min(len(x), maxAttrListLen))
		for _, m := range x {
			if len(out) == maxAttrListLen {
				break
			}
			out = append(out, MCPServerRef{Label: Truncate(m.Label, 128), Origin: Origin(m.Origin)})
		}
		return out, len(out) > 0
	case map[string]string:
		// Labels: short keys and values only.
		out := make(map[string]string, len(x))
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if len(out) == maxAttrListLen {
				break
			}
			out[Truncate(k, 64)] = Truncate(x[k], 64)
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

// Origin reduces a URL to its scheme and host (and port): no user info,
// path, query or fragment, any of which can carry a credential.
func Origin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// RepositoryURL keeps a source repository URL's scheme, host and path, and
// drops user info, query and fragment. Non-HTTP(S) URLs are dropped.
func RepositoryURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return Truncate(u.Scheme+"://"+u.Host+strings.TrimSuffix(u.EscapedPath(), "/"), maxAttrString)
}

// Truncate cuts s to at most n runes.
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// SanitizeName trims a display name to a safe length.
func SanitizeName(s string) string {
	return Truncate(strings.TrimSpace(s), maxNameChars)
}

// MinimiseOwnerHint keeps no more than a name, an email and a username or
// principal, as the retention rules allow.
func MinimiseOwnerHint(h models.ShadowOwnerHint) models.ShadowOwnerHint {
	return models.ShadowOwnerHint{
		Name:      Truncate(strings.TrimSpace(h.Name), 128),
		Email:     Truncate(strings.TrimSpace(h.Email), 254),
		Username:  Truncate(strings.TrimSpace(h.Username), 256),
		Type:      Truncate(h.Type, 32),
		Source:    Truncate(h.Source, 128),
		CreatedAt: h.CreatedAt,
	}
}

// Patterns that look like credentials, removed from any text that is
// stored or logged (provider error messages may echo a request).
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{8,}`),                                // OpenAI / Anthropic keys
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/\-]+=*`),                   // bearer tokens
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]*`), // JWTs
	regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{12,}\b`),                         // AWS access key IDs
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token|signature|sig)=([^&\s"']+)`),
	regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`), // long base64 blobs (secret keys, SAS)
}

// ScrubText removes credential-like substrings and cuts the text to a short
// message, for errors that are stored in scan results or logged.
func ScrubText(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[redacted]")
	}
	s = strings.Join(strings.Fields(s), " ")
	return Truncate(s, maxMessageChars)
}

// SecretHint returns a redacted hint of a secret: its first and last four
// characters when it is long enough for that to reveal nothing useful.
func SecretHint(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if utf8.RuneCountInString(secret) < 16 {
		return "••••"
	}
	r := []rune(secret)
	return fmt.Sprintf("%s…%s", string(r[:4]), string(r[len(r)-4:]))
}
