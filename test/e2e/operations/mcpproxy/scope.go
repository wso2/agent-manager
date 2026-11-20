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

func CreateScope(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, proxyID string, request framework.MCPProxyScopeRequest) framework.MCPProxyScopeResponse {
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/mcp-proxies/%s/scopes", orgName, proxyID)
	response, err := client.PostWithContext(ctx, path, request)
	g.Expect(err).NotTo(HaveOccurred(), "create MCP scope request failed")
	defer response.Body.Close()
	return framework.ExpectStatusAndDecode[framework.MCPProxyScopeResponse](g, response, http.StatusCreated)
}
