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

package mcpproxy

import (
	"context"
	"fmt"
	"net/http"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateScope adds a tool scope to an MCP proxy. The action naming the scope is the
// document's metadata.name; spec carries its description and tool bindings.
func CreateScope(ctx context.Context, g Gomega, client *framework.AMPClient, proxyID, action string, spec framework.MCPProxyScopeSpec) framework.MCPProxyScopeDocument {
	doc := framework.MCPProxyScopeDocument{
		APIVersion: framework.MCPProxyAPIVersion,
		Kind:       framework.MCPProxyScopeKind,
		Metadata:   framework.DocumentMetadata{Name: action},
		Spec:       spec,
	}
	path := fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s/scopes", proxyID)
	response, err := client.PostWithContext(ctx, path, doc)
	g.Expect(err).NotTo(HaveOccurred(), "create MCP scope request failed")
	defer response.Body.Close()
	return framework.ExpectStatusAndDecode[framework.MCPProxyScopeDocument](g, response, http.StatusCreated)
}
