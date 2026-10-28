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
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestBuildPublicProxyURLUsesVhost(t *testing.T) {
	// RuntimeURL present but must be ignored: this is the address handed to callers
	// outside the cluster — external agents and MCP's resource identifier.
	gateway := &models.Gateway{
		Vhost:      "https://dev-acme.gateway.example.com",
		RuntimeURL: "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893",
	}
	contextPath := "/llm/proxy"
	require.Equal(t, "https://dev-acme.gateway.example.com/llm/proxy", buildPublicProxyURL(gateway, &contextPath))
	require.Equal(t, "https://dev-acme.gateway.example.com", buildPublicProxyURL(gateway, nil))
}

func TestBuildMCPProxyURLUsesGatewayVhost(t *testing.T) {
	// MCP keeps the public vhost even with a RuntimeURL registered: this URL doubles as
	// the OAuth resource identifier (MCPResourceServerIdentifier), so an in-cluster
	// address here would put an unroutable audience in every AgentID token.
	gateway := &models.Gateway{
		Vhost:      "https://gateway.example.com",
		RuntimeURL: "http://runtime.acme-dev:22893",
	}
	ctxPath := "/github"
	require.Equal(t, "https://gateway.example.com/github/mcp", buildMCPProxyURL(gateway, models.MCPProxyConfig{Context: &ctxPath}))
	require.Equal(t, "https://gateway.example.com/mcp", buildMCPProxyURL(gateway, models.MCPProxyConfig{}))
}

func TestBuildMCPProxyURLPrefersProxyVhostOverride(t *testing.T) {
	// The deployment spec forwards the proxy's own vhost to the gateway, so the
	// override — not the gateway default — is where the proxy is actually served.
	gateway := &models.Gateway{Vhost: "https://gateway.example.com"}
	vhost := "mcp.example.com"
	ctxPath := "/github"
	require.Equal(t, "https://mcp.example.com/github/mcp",
		buildMCPProxyURL(gateway, models.MCPProxyConfig{Vhost: &vhost, Context: &ctxPath}))
	full := "http://mcp.example.com"
	require.Equal(t, "http://mcp.example.com/mcp",
		buildMCPProxyURL(gateway, models.MCPProxyConfig{Vhost: &full}))
	empty := "  "
	require.Equal(t, "https://gateway.example.com/mcp",
		buildMCPProxyURL(gateway, models.MCPProxyConfig{Vhost: &empty}))
}

func TestBuildPublicProxyURLDefaultsBareVhostToHTTPS(t *testing.T) {
	// Gateways registered with a scheme-less vhost must still yield absolute URLs.
	gateway := &models.Gateway{Vhost: "gw.example.com"}
	contextPath := "/llm/proxy"
	require.Equal(t, "https://gw.example.com/llm/proxy", buildPublicProxyURL(gateway, &contextPath))
	require.Equal(t, "https://gw.example.com", buildPublicProxyURL(gateway, nil))
}

func TestBuildMCPProxyURLDefaultsBareGatewayVhostToHTTPS(t *testing.T) {
	gateway := &models.Gateway{Vhost: "gw.example.com"}
	ctxPath := "/github"
	require.Equal(t, "https://gw.example.com/github/mcp", buildMCPProxyURL(gateway, models.MCPProxyConfig{Context: &ctxPath}))
	require.Equal(t, "https://gw.example.com/mcp", buildMCPProxyURL(gateway, models.MCPProxyConfig{}))
}

func TestResourceIdentifierUnchangedByVhostScheme(t *testing.T) {
	// The RS identifier is the absolute proxy URL and must be identical whether the
	// gateway registered a bare or an https:// vhost.
	bare := &models.Gateway{Vhost: "gw.example.com"}
	prefixed := &models.Gateway{Vhost: "https://gw.example.com"}
	ctxPath := "/github"
	cfg := models.MCPProxyConfig{Context: &ctxPath}
	require.Equal(t, "https://gw.example.com/github/mcp", buildMCPProxyURL(bare, cfg))
	require.Equal(t, buildMCPProxyURL(prefixed, cfg), buildMCPProxyURL(bare, cfg))
}

func TestBuildInternalProxyURLUsesRuntimeURL(t *testing.T) {
	// Plaintext http on a non-standard port: the scheme must survive verbatim.
	gateway := &models.Gateway{
		Vhost:      "https://dev-acme.gateway.example.com",
		RuntimeURL: "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893",
	}
	contextPath := "/llm/proxy"

	withContext, err := buildInternalProxyURL(gateway, &contextPath)
	require.NoError(t, err)
	require.Equal(t, "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893/llm/proxy", withContext)

	bare, err := buildInternalProxyURL(gateway, nil)
	require.NoError(t, err)
	require.Equal(t, "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893", bare)
}

func TestBuildInternalProxyURLRejectsMissingRuntimeURL(t *testing.T) {
	// A gateway with no reachable in-cluster address must fail closed, never fall back
	// to the vhost. Migration 038 only backfills "api-platform-" names, so this is live.
	contextPath := "/llm/proxy"
	empty := &models.Gateway{UUID: uuid.New(), Vhost: "https://gw.example.com"}
	_, err := buildInternalProxyURL(empty, &contextPath)
	require.ErrorIs(t, err, errGatewayRuntimeURLUnregistered)

	whitespace := &models.Gateway{UUID: uuid.New(), Vhost: "https://gw.example.com", RuntimeURL: "   "}
	_, err = buildInternalProxyURL(whitespace, &contextPath)
	require.ErrorIs(t, err, errGatewayRuntimeURLUnregistered)
}

// enableGatewayPublicVhostFallback turns on config.GatewayPublicVhostFallback for one
// test and restores it afterwards.
func enableGatewayPublicVhostFallback(t *testing.T) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.GatewayPublicVhostFallback
	cfg.GatewayPublicVhostFallback = true
	t.Cleanup(func() { cfg.GatewayPublicVhostFallback = orig })
}

func TestBuildInternalProxyURLFallsBackToVhostWhenEnabled(t *testing.T) {
	// A gateway registered with no in-cluster address — every cloud gateway, since only the
	// on-prem gateway extension chart POSTs a runtimeUrl. With the fallback enabled the agent
	// reaches it over the same public host its MCP proxies already use, so this must not fail.
	enableGatewayPublicVhostFallback(t)
	contextPath := "/llm/proxy"

	empty := &models.Gateway{UUID: uuid.New(), Vhost: "https://gw.example.com"}
	withContext, err := buildInternalProxyURL(empty, &contextPath)
	require.NoError(t, err)
	require.Equal(t, "https://gw.example.com/llm/proxy", withContext)
	bare, err := buildInternalProxyURL(empty, nil)
	require.NoError(t, err)
	require.Equal(t, "https://gw.example.com", bare)

	whitespace := &models.Gateway{UUID: uuid.New(), Vhost: "https://gw.example.com", RuntimeURL: "   "}
	got, err := buildInternalProxyURL(whitespace, &contextPath)
	require.NoError(t, err)
	require.Equal(t, "https://gw.example.com/llm/proxy", got)

	// A bare vhost still picks up the https default, as the public builder does.
	noScheme := &models.Gateway{UUID: uuid.New(), Vhost: "gw.example.com"}
	got, err = buildInternalProxyURL(noScheme, &contextPath)
	require.NoError(t, err)
	require.Equal(t, "https://gw.example.com/llm/proxy", got)

	// A registered runtimeUrl still wins: the fallback only covers its absence.
	registered := &models.Gateway{UUID: uuid.New(), Vhost: "https://gw.example.com", RuntimeURL: "http://gw-runtime:22893"}
	got, err = buildInternalProxyURL(registered, &contextPath)
	require.NoError(t, err)
	require.Equal(t, "http://gw-runtime:22893/llm/proxy", got)
}
