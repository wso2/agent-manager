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

package models

import "fmt"

// UpstreamConfig represents the upstream configuration with main and sandbox endpoints
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
	SecretRef *string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"` // KV path reference — mutually exclusive with Value
}

// Validate enforces that Value and SecretRef are mutually exclusive.
// Call this at the top of service methods before any I/O.
func (a *UpstreamAuth) Validate() error {
	if a.Value != nil && a.SecretRef != nil {
		return fmt.Errorf("UpstreamAuth.Value and SecretRef are mutually exclusive; provide only one")
	}
	return nil
}
