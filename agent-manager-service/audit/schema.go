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

// FieldKind documents what a declared detail field holds. It is descriptive
// rather than enforcing — the enforcement is that a key absent from the schema
// is dropped entirely.
type FieldKind string

const (
	// KindIdentifier is an opaque id or handle.
	KindIdentifier FieldKind = "identifier"
	// KindName is a human-readable name.
	KindName FieldKind = "name"
	// KindEmail is an email address. Recorded because it identifies the subject
	// of a membership change, which is what an investigation needs.
	KindEmail FieldKind = "email"
	// KindNameList is a list of names or scope strings.
	KindNameList FieldKind = "name-list"
	// KindCount is a numeric count.
	KindCount FieldKind = "count"
	// KindFlag is a boolean.
	KindFlag FieldKind = "flag"
	// KindEnum is a value from a closed set.
	KindEnum FieldKind = "enum"
	// KindFingerprint is a non-reversible reference to a secret, produced by
	// Fingerprint. Never the secret itself.
	KindFingerprint FieldKind = "fingerprint"
	// KindURL is an operator-supplied URL, reduced to scheme, host and path
	// before it is written. A URL is the one declared value that can carry a
	// credential in its own syntax — userinfo, or a token in the query — so
	// this kind is enforced rather than descriptive: see sanitizeURL.
	KindURL FieldKind = "url"
)

// baseFields are permitted on every action. They describe the request rather
// than the operation, so declaring them per action would be pure repetition.
var baseFields = map[string]FieldKind{
	// Set by the coverage middleware from the route's path parameters.
	"pathParams": KindNameList,
	// Set when the envelope event stands in for a failed request.
	"envelope": KindFlag,
	// Links a Begin intent record to its Complete outcome record.
	"attemptEventId": KindIdentifier,
	// Set on records emitted from the MCP surface, naming the tool invoked.
	"tool": KindName,
	// Set when events were coalesced or suppressed.
	"repeatCount":     KindCount,
	"suppressedCount": KindCount,
	// Set by redactDetails when a key was not declared.
	"_droppedKeys": KindNameList,
}

// detailSchema declares, per action, which detail keys may be recorded.
//
// An action with no entry here permits only baseFields. That is a deliberate
// fail-closed default: a new emit site that passes an undeclared field loses it
// and says so via _droppedKeys, rather than writing something nobody vetted.
var detailSchema = map[Action]map[string]FieldKind{
	ActionAuthnFailure: {
		// Classified reason only — never the token or any fragment of it.
		"reason":     KindEnum,
		"authHeader": KindFlag,
	},
	ActionAuthzDeny: {
		"reason":        KindEnum,
		"missingScope":  KindName,
		"grantedScopes": KindCount,
	},
	ActionAuthzRootOUBypass: {
		"rootOUBypass": KindFlag,
	},
	ActionSystemStartup: {
		"version":  KindName,
		"sinks":    KindNameList,
		"instance": KindIdentifier,
	},
	ActionSystemRBACDisabled: {
		"reason": KindEnum,
	},
	ActionSystemAuditDropped: {
		"droppedTotal": KindCount,
		"sink":         KindName,
	},
}

// DetailSchema returns the permitted detail keys for an action, always
// including the base fields.
func DetailSchema(action Action) map[string]FieldKind {
	declared := detailSchema[action]
	out := make(map[string]FieldKind, len(declared)+len(baseFields))
	for k, v := range baseFields {
		out[k] = v
	}
	for k, v := range declared {
		out[k] = v
	}
	return out
}

// RegisterDetailSchema declares the permitted detail keys for an action.
// Semantic emit sites call this alongside Register so that adding an action
// forces a decision about what it may record.
func RegisterDetailSchema(action Action, fields map[string]FieldKind) {
	if fields == nil {
		fields = map[string]FieldKind{}
	}
	detailSchema[action] = fields
}

// SchemaActions returns every action with an explicit detail schema. Used by
// tests to assert that registered actions have had their detail shape decided.
func SchemaActions() []Action {
	out := make([]Action, 0, len(detailSchema))
	for a := range detailSchema {
		out = append(out, a)
	}
	return out
}
