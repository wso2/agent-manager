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

import "fmt"

// corsPolicy returns the gateway CORS policy with the given allowed origins,
// methods, headers and credentials flag.
func corsPolicy(allowedOrigins, allowedMethods, allowedHeaders []string, allowCredentials bool) map[string]interface{} {
	return map[string]interface{}{
		"name":    "cors",
		"version": "v1",
		"params": map[string]interface{}{
			"allowedOrigins":   allowedOrigins,
			"allowedMethods":   allowedMethods,
			"allowedHeaders":   allowedHeaders,
			"allowCredentials": allowCredentials,
		},
	}
}

// formatResilienceTimeout renders a timeout in seconds as a gateway duration.
func formatResilienceTimeout(seconds int32) string {
	return fmt.Sprintf("%ds", seconds)
}

// apiKeyAuthPolicy returns the gateway policy for API key authentication.
func apiKeyAuthPolicy() map[string]interface{} {
	return map[string]interface{}{
		"name":    "api-key-auth",
		"version": "v1",
		"params": map[string]interface{}{
			"key": "X-API-Key",
			"in":  "header",
		},
	}
}

// OAuth (jwt-auth) policy contract. These literals are the gateway policy's
// contract and the only place "jwt" wording survives; user-facing surfaces say "OAuth".
const (
	oauthPolicyName    = "jwt-auth"
	oauthPolicyVersion = "v1"
)

// oauthPolicyParams holds the jwt-auth policy parameters resolved for an agent.
// The policy is authentication-only: it validates the token issuer, signature,
// expiry and, optionally, the audience claim.
type oauthPolicyParams struct {
	Issuers          []string
	Audiences        []string
	HeaderName       string
	AuthHeaderPrefix string
	// ForwardToken controls whether the validated token header is forwarded upstream.
	ForwardToken bool
}

// oauthPolicy returns the gateway policy for OAuth (JWT) authentication.
// Empty issuers and audiences are omitted.
func oauthPolicy(p oauthPolicyParams) map[string]interface{} {
	params := map[string]interface{}{}
	if len(p.Issuers) > 0 {
		params["issuers"] = p.Issuers
	}
	if len(p.Audiences) > 0 {
		params["audiences"] = p.Audiences
	}
	params["headerName"] = p.HeaderName
	params["authHeaderPrefix"] = p.AuthHeaderPrefix
	params["forwardToken"] = p.ForwardToken
	return map[string]interface{}{
		"name":    oauthPolicyName,
		"version": oauthPolicyVersion,
		"params":  params,
	}
}

// Deployment statuses reported for an agent's environment.
const (
	deploymentStatusActive      = "active"
	deploymentStatusNotDeployed = "not-deployed"
)
