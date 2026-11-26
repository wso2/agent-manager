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

// Package mcpproxy provides E2E operation helpers for MCP proxies in the token's
// organization, mirroring the llmprovider/llmproxy operation packages.
package mcpproxy

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// FetchServerInfo connects to an upstream MCP server and returns its capabilities.
func FetchServerInfo(g Gomega, client *framework.AMPClient, req framework.MCPServerInfoFetchRequest) framework.MCPServerInfoFetchResponse {
	resp, err := client.Post("/m/ai-gateway/mcp-proxies/fetch-server-info", req)
	g.Expect(err).NotTo(HaveOccurred(), "fetch MCP server info request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MCPServerInfoFetchResponse](g, resp)
}

// CreateMCPProxy creates an MCP proxy in the token's org, named by handle.
func CreateMCPProxy(g Gomega, client *framework.AMPClient, handle string, spec framework.MCPProxySpec) framework.MCPProxyDocument {
	doc := framework.MCPProxyDocument{
		APIVersion: framework.MCPProxyAPIVersion,
		Kind:       framework.MCPProxyKind,
		Metadata:   framework.DocumentMetadata{Name: handle},
		Spec:       spec,
	}
	resp, err := client.Post("/m/ai-gateway/mcp-proxies", doc)
	g.Expect(err).NotTo(HaveOccurred(), "create MCP proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.MCPProxyDocument](g, resp)
}

// UpdateMCPProxy replaces an MCP proxy's spec. The spec must carry every field to keep.
func UpdateMCPProxy(g Gomega, client *framework.AMPClient, handle string, spec framework.MCPProxySpec) framework.MCPProxyDocument {
	doc := framework.MCPProxyDocument{
		APIVersion: framework.MCPProxyAPIVersion,
		Kind:       framework.MCPProxyKind,
		Metadata:   framework.DocumentMetadata{Name: handle},
		Spec:       spec,
	}
	resp, err := client.Put(fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s", handle), doc)
	g.Expect(err).NotTo(HaveOccurred(), "update MCP proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MCPProxyDocument](g, resp)
}

// GetMCPProxy retrieves an MCP proxy by UUID or handle.
func GetMCPProxy(g Gomega, client *framework.AMPClient, proxyID string) framework.MCPProxyDocument {
	resp, err := client.Get(fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s", proxyID))
	g.Expect(err).NotTo(HaveOccurred(), "get MCP proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MCPProxyDocument](g, resp)
}

// GetMCPProxyExpectStatus performs a GET and asserts the response status, without
// decoding a body. Useful for verifying a proxy is gone after deletion.
func GetMCPProxyExpectStatus(g Gomega, client *framework.AMPClient, proxyID string, expected int) {
	resp, err := client.Get(fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s", proxyID))
	g.Expect(err).NotTo(HaveOccurred(), "get MCP proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, expected)
}

// ListMCPProxies lists the MCP proxies in the token's org.
func ListMCPProxies(g Gomega, client *framework.AMPClient) framework.MCPProxyListResponse {
	resp, err := client.Get("/m/ai-gateway/mcp-proxies")
	g.Expect(err).NotTo(HaveOccurred(), "list MCP proxies request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MCPProxyListResponse](g, resp)
}

// DeleteMCPProxy deletes an MCP proxy by UUID or handle.
func DeleteMCPProxy(g Gomega, client *framework.AMPClient, proxyID string) {
	resp, err := client.Delete(fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s", proxyID))
	g.Expect(err).NotTo(HaveOccurred(), "delete MCP proxy request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}

// DeleteMCPProxyBestEffort is intended for suite teardown after a disposable
// runtime fixture. It never hides a spec failure behind a cleanup failure.
func DeleteMCPProxyBestEffort(ctx context.Context, client *framework.AMPClient, proxyID string) {
	if proxyID == "" {
		return
	}
	path := fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s", proxyID)
	response, err := client.DeleteWithContext(ctx, path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("teardown: delete MCP proxy %q failed: %v\n", proxyID, err)
		return
	}
	defer response.Body.Close()
	ginkgo.GinkgoWriter.Printf("teardown: deleted MCP proxy %q (status %d)\n", proxyID, response.StatusCode)
}
