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

package gwmodels

import (
	"fmt"
	"regexp"
)

// durationPattern matches the gateway's accepted duration strings (e.g. "15s", "500ms"),
// mirroring the pattern enforced by the api-platform gateway-controller's Resilience schema.
// Whole numbers only — fractional durations (e.g. "1.5s") are not accepted.
var durationPattern = regexp.MustCompile(`^\d+(ms|s|m|h)$`)

// UpstreamConfig represents the upstream configuration.
//
// Main/Sandbox model the classic single-endpoint shape used by deployable
// entities (LLM providers, and the flattened MCP proxy mappings that deploy to
// gateways). MCP proxy blueprints store their per-environment upstreams in
// MCPEnvironmentConfig instead of here.
type UpstreamConfig struct {
	Main    *UpstreamEndpoint `json:"main,omitempty"`
	Sandbox *UpstreamEndpoint `json:"sandbox,omitempty"`
}

// UpstreamEndpoint represents an upstream endpoint configuration
type UpstreamEndpoint struct {
	URL  string        `json:"url,omitempty"`
	Ref  string        `json:"ref,omitempty"`
	Auth *UpstreamAuth `json:"auth,omitempty"`
}

// UpstreamAuth represents upstream authentication configuration
type UpstreamAuth struct {
	Type      *string `json:"type" yaml:"type"`
	Header    *string `json:"header,omitempty" yaml:"header,omitempty"`
	Value     *string `json:"value,omitempty" yaml:"value,omitempty"`
	SecretRef *string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"` // AES-256-GCM encrypted, base64-encoded value — mutually exclusive with Value
}

// Validate enforces that Value and SecretRef are mutually exclusive.
// Call this at the top of service methods before any I/O.
func (a *UpstreamAuth) Validate() error {
	if a.Value != nil && a.SecretRef != nil {
		return fmt.Errorf("UpstreamAuth.Value and SecretRef are mutually exclusive; provide only one")
	}
	return nil
}

// Resilience carries the gateway's route-level request/idle timeouts. It maps
// directly onto the CRD spec's own top-level "resilience" field with no
// indirection. Both fields are duration strings (e.g. "15s", "500ms"); "0s"
// explicitly disables the timeout, while a nil field is omitted so the
// gateway falls back to its global default.
type Resilience struct {
	Timeout     *string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	IdleTimeout *string `json:"idleTimeout,omitempty" yaml:"idleTimeout,omitempty"`
}

// Validate enforces that Timeout and IdleTimeout, when set, are duration strings the
// gateway accepts (e.g. "15s", "500ms", "0s" to disable).
func (r *Resilience) Validate() error {
	if r == nil {
		return nil
	}
	if r.Timeout != nil && !durationPattern.MatchString(*r.Timeout) {
		return fmt.Errorf("resilience.timeout %q is not a valid duration (expected e.g. \"15s\", \"500ms\")", *r.Timeout)
	}
	if r.IdleTimeout != nil && !durationPattern.MatchString(*r.IdleTimeout) {
		return fmt.Errorf("resilience.idleTimeout %q is not a valid duration (expected e.g. \"15s\", \"500ms\")", *r.IdleTimeout)
	}
	return nil
}
