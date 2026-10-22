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

// A2AConnectSecurityMode is how a client authenticates to an A2A agent in one environment.
type A2AConnectSecurityMode string

const (
	A2AConnectSecurityNone A2AConnectSecurityMode = "none"
	// A2AConnectSecurityAPIKey is reported with the header the gateway reads the key from.
	A2AConnectSecurityAPIKey A2AConnectSecurityMode = "apiKey"
	A2AConnectSecurityOAuth  A2AConnectSecurityMode = "oauth"
	// A2AConnectSecurityUnmanaged is a direct external agent: the platform does not manage its security.
	A2AConnectSecurityUnmanaged A2AConnectSecurityMode = "unmanaged"
	// A2AConnectSecurityUnknown is a gateway-fronted agent with no config row; the backend does not guess a default.
	A2AConnectSecurityUnknown A2AConnectSecurityMode = "unknown"
)

// A2AConnectPublicationNone is the connect-only publication status of a direct external agent.
const A2AConnectPublicationNone = "none"

// A2AConnectIssuer is an OAuth issuer name resolved against the environment's identity providers; Issuer is "" when unresolved.
type A2AConnectIssuer struct {
	Name   string
	Issuer string
}

type A2AConnectOAuth struct {
	Issuers          []A2AConnectIssuer
	Audiences        []string
	HeaderName       string
	AuthHeaderPrefix string
}

type A2AConnectSecurity struct {
	Mode         A2AConnectSecurityMode
	APIKeyHeader string
	OAuth        *A2AConnectOAuth
}

// A2AConnectPublication is the gateway publication of the agent in the environment.
type A2AConnectPublication struct {
	// Status is an A2APublicationStatus value or A2AConnectPublicationNone.
	Status    string
	LastError string
}

// A2AConnectDetails is what a client needs to connect to an A2A agent in one environment.
type A2AConnectDetails struct {
	Kind         A2AAgentCardSource
	Proxied      bool
	IsProduction bool
	GatewayURL   string
	CardURL      string
	// JSONRPCURL is "" when the agent has no JSON-RPC endpoint.
	JSONRPCURL  string
	Security    A2AConnectSecurity
	Publication A2AConnectPublication
}
