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

package thundersvc

import (
	"context"
	"fmt"
	"strings"
)

// EnvThunderEndpoints describes how Agent Manager reaches one environment's
// Thunder instance. Management and OAuth token traffic normally share one
// endpoint, but deployments may route them separately.
type EnvThunderEndpoints struct {
	ManagementURL           string
	ManagementResolveToHost string
	// ManagementURLTrusted permits direct access to a deployment-controlled
	// internal endpoint. The safe zero value applies SSRF protection.
	ManagementURLTrusted bool
	TokenURL             string
	TokenResolveToHost   string
	// TokenURLTrusted permits direct access to a deployment-controlled token
	// endpoint. Leave false for URLs derived from stored or user-supplied data.
	TokenURLTrusted     bool
	SystemTokenScope    string
	SystemTokenResource string
}

// EnvThunderEndpointResolver is the deployment seam for selecting Thunder
// management and token endpoints. The default implementation preserves the
// existing on-premises URL probing and token semantics.
type EnvThunderEndpointResolver interface {
	ResolveEndpoints(ctx context.Context, ouID, orgNamespace, envName, storedURL string, callerSupplied bool) (EnvThunderEndpoints, error)
}

type defaultEnvThunderEndpointResolver struct {
	resolveBaseURL resolveBaseURLFunc
}

// DefaultEnvThunderEndpointResolver returns the standard on-premises resolver.
func DefaultEnvThunderEndpointResolver() EnvThunderEndpointResolver {
	return defaultEnvThunderEndpointResolver{resolveBaseURL: ResolveThunderBaseURL}
}

func (r defaultEnvThunderEndpointResolver) ResolveEndpoints(ctx context.Context, _ string, orgNamespace, envName, storedURL string, callerSupplied bool) (EnvThunderEndpoints, error) {
	baseURL, resolveToHost, ok := r.resolveBaseURL(ctx, orgNamespace, envName, storedURL, callerSupplied)
	if !ok {
		return EnvThunderEndpoints{}, fmt.Errorf("%w: %s/%s", ErrThunderUnreachable, orgNamespace, envName)
	}
	directCallerSupplied := callerSupplied && resolveToHost == "" && baseURL == storedURL
	return EnvThunderEndpoints{
		ManagementURL:           strings.TrimRight(baseURL, "/"),
		ManagementResolveToHost: resolveToHost,
		ManagementURLTrusted:    !directCallerSupplied,
		TokenURL:                strings.TrimRight(baseURL, "/") + "/oauth2/token",
		TokenResolveToHost:      resolveToHost,
		TokenURLTrusted:         !directCallerSupplied,
		SystemTokenScope:        "system",
		SystemTokenResource:     SystemResourceIdentifier(ThunderIssuerURL(storedURL)),
	}, nil
}

// ResolveEnvThunderTokenEndpoint resolves the token URL used inside an agent
// workload without reading or exposing the environment's system credential.
func ResolveEnvThunderTokenEndpoint(ctx context.Context, endpointResolver EnvThunderEndpointResolver, readThunderURL ReadThunderURLFunc, ouID, orgNamespace, envName string) (string, error) {
	if endpointResolver == nil {
		endpointResolver = DefaultEnvThunderEndpointResolver()
	}
	storedURL, callerSupplied, err := readThunderURL(ctx, ouID, envName)
	if err != nil {
		return "", fmt.Errorf("failed to read env-thunder url for %s/%s: %w", ouID, envName, err)
	}
	if storedURL == "" {
		return "", ErrThunderNotProvisioned
	}
	endpoints, err := endpointResolver.ResolveEndpoints(ctx, ouID, orgNamespace, envName, storedURL, callerSupplied)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(endpoints.TokenURL) == "" {
		return "", fmt.Errorf("env-thunder token URL is empty for %s/%s", ouID, envName)
	}
	if err := validateThunderEndpointURL(endpoints.TokenURL); err != nil {
		return "", fmt.Errorf("invalid env-thunder token URL for %s/%s: %w", ouID, envName, err)
	}
	return endpoints.TokenURL, nil
}
