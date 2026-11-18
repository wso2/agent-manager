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

package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Field length caps. Audit records carry attacker-controlled strings (user
// agent, resource names); bounding them keeps one caller from flooding the sink.
const (
	maxUserAgentLen = 512
	maxStringLen    = 1024
	maxErrorLen     = 512
	maxDetailKeys   = 32
	maxListLen      = 64
)

// sensitiveKeyPattern matches attribute names that look like they hold a
// credential. Used to flag — never to filter: filtering by name is exactly the
// deny-list mistake this package avoids, so a matching key means "record that a
// secret-shaped attribute was set", not "try to scrub it".
var sensitiveKeyPattern = regexp.MustCompile(`(?i)pass|secret|token|key|credential|auth`)

// secretShapedPatterns catch credential material that reached a record despite
// the allow-list — a defence in depth, not the primary control. The primary
// control is that bodies are never read and Detail accepts only scalars.
var secretShapedPatterns = []struct {
	kind    string
	pattern *regexp.Regexp
}{
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{"private-key", regexp.MustCompile(`-----BEGIN[ A-Z]*PRIVATE KEY-----`)},
	{"bearer", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/-]{16,}`)},
	{"high-entropy", regexp.MustCompile(`\b[A-Za-z0-9+/_-]{40,}={0,2}\b`)},
}

// Fingerprint reduces a secret to a non-reversible reference: a short SHA-256
// prefix plus the last four characters. That is enough to correlate "the key
// that was rotated" with "the key that was later used" without the record ever
// holding usable material.
func Fingerprint(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	digest := hex.EncodeToString(sum[:])[:12]
	suffix := value
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	return fmt.Sprintf("sha256:%s…%s", digest, suffix)
}

// Escape existing backslashes too, so a literal escape stays distinct.
var attributeKeyEscapes = strings.NewReplacer("\\", "\\\\", "\u2028", `\u2028`, "\u2029", `\u2029`)

// AttributeKeySummary describes a free-form attribute map without recording any
// value. The user-creation API accepts an open map[string]string that carries
// passwords, so recording keys and counts preserves what forensics needs —
// "an admin set five attributes including a password-shaped one" — while making
// it structurally impossible to write the value.
func AttributeKeySummary(attrs map[string]string) (keys []string, count int, containsSensitive bool) {
	keys = make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, utils.SanitizeForLog(attributeKeyEscapes.Replace(k)))
		if sensitiveKeyPattern.MatchString(k) {
			containsSensitive = true
		}
	}
	sort.Strings(keys)
	if len(keys) > maxListLen {
		keys = keys[:maxListLen]
	}
	return keys, len(attrs), containsSensitive
}

// redact normalises an event immediately before it reaches any sink. It is
// applied once, centrally, rather than per sink — applying it at the sink is
// how one of two sinks ends up emitting unredacted records.
func redact(e *Event) {
	e.ActorID = clean(e.ActorID, maxStringLen)
	e.ActorDisplay = clean(e.ActorDisplay, maxStringLen)
	e.ActorOUID = clean(e.ActorOUID, maxStringLen)
	e.ActorTokenID = clean(e.ActorTokenID, maxStringLen)
	e.OnBehalfOf = clean(e.OnBehalfOf, maxStringLen)
	e.SourceIP = clean(e.SourceIP, maxStringLen)
	e.UserAgent = clean(e.UserAgent, maxUserAgentLen)
	e.CorrelationID = clean(e.CorrelationID, maxStringLen)
	e.RequestPath = clean(e.RequestPath, maxStringLen)
	e.OUID = clean(e.OUID, maxStringLen)
	e.OrgHandle = clean(e.OrgHandle, maxStringLen)
	e.ResourceType = clean(e.ResourceType, maxStringLen)
	e.ResourceID = clean(e.ResourceID, maxStringLen)
	e.ResourceName = clean(e.ResourceName, maxStringLen)
	e.ProjectName = clean(e.ProjectName, maxStringLen)
	e.Environment = clean(e.Environment, maxStringLen)
	e.ErrorCode = clean(e.ErrorCode, maxStringLen)
	// The one field carrying unbounded third-party text. Errors from upstream
	// systems routinely interpolate a whole HTTP response body — Thunder's
	// client does exactly that — so a failed user creation or secret write can
	// carry the echoed payload, credentials included. It needs the same
	// secret-shaped masking the caller-supplied details get, and needs it more.
	e.ErrorMessage = maskSecretShaped(clean(e.ErrorMessage, maxErrorLen))

	e.Details = redactDetails(e.Action, e.Details)
}

// redactDetails drops any key the action's schema does not declare, then scrubs
// the surviving values.
//
// The allow-list is the point: a deny-list fails on the field nobody thought of,
// which is precisely how the existing identity-controller log sanitiser leaks
// everything except the single key it knows about.
func redactDetails(action Action, details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	allowed := DetailSchema(action)
	out := make(map[string]any, len(details))
	var dropped []string

	for k, v := range details {
		if len(out) >= maxDetailKeys {
			dropped = append(dropped, k)
			continue
		}
		kind, ok := allowed[k]
		if !ok {
			dropped = append(dropped, k)
			continue
		}
		// The declared kind is descriptive for every other field. KindURL is
		// the exception, because a URL can hold a credential in its own syntax
		// and no caller should have to remember that at the emit site.
		if kind == KindURL {
			if s, isString := v.(string); isString {
				v = sanitizeURL(s)
			}
		}
		out[k] = scrubValue(v)
	}

	if len(dropped) > 0 {
		// Record that fields were dropped rather than dropping them silently —
		// a schema gap should be visible in the trail, not invisible.
		sort.Strings(dropped)
		if len(dropped) > maxListLen {
			dropped = dropped[:maxListLen]
		}
		out["_droppedKeys"] = dropped
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizeURL reduces an operator-supplied URL to scheme, host and path.
//
// Every other declared value is a name, a count or a flag — shapes that cannot
// hold a credential. A URL can: RFC 3986 userinfo puts one in the authority
// ("https://user:pass@idp.example/jwks"), and a query parameter puts one in
// plain sight. Neither is what makes the URL worth recording. What forensics
// needs from a JWKS URI is which host the gateway now fetches signing keys
// from, and that survives here intact.
//
// Dropping the query rather than filtering known-bad parameter names is the
// same allow-list reasoning the detail schema uses: a deny-list of parameter
// names fails on the one nobody thought of.
func sanitizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		// Not a URL this can reason about. Recording it verbatim would defeat
		// the point, so record only that something unparseable was supplied.
		return "[unparseable-url]"
	}

	sanitized := url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	out := sanitized.String()
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		// Say that something was removed. A URI silently shortened reads as the
		// one the operator configured, and "was there a token on it?" is a
		// question an investigation will ask.
		out += "[redacted-components]"
	}
	return out
}

// scrubValue sanitises a detail value. Only scalars and string slices can reach
// here — Detail rejects everything else — so this is a bounded set of cases.
func scrubValue(v any) any {
	switch val := v.(type) {
	case string:
		return maskSecretShaped(clean(val, maxStringLen))
	case []string:
		if len(val) > maxListLen {
			val = val[:maxListLen]
		}
		out := make([]string, 0, len(val))
		for _, s := range val {
			out = append(out, maskSecretShaped(clean(s, maxStringLen)))
		}
		return out
	default:
		// bool, int, float64 — nothing to scrub.
		return v
	}
}

// maskSecretShaped replaces credential-looking substrings. This is a backstop:
// if it ever fires on a real record, the emit site is passing something it
// should not, and the replacement makes that visible rather than silent.
func maskSecretShaped(s string) string {
	for _, p := range secretShapedPatterns {
		s = p.pattern.ReplaceAllString(s, "[REDACTED:"+p.kind+"]")
	}
	return s
}

// clean strips control characters and bounds length.
func clean(s string, max int) string {
	if s == "" {
		return ""
	}
	return utils.TruncateForLog(strings.TrimSpace(utils.SanitizeForLog(s)), max)
}
