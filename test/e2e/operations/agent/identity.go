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

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// AgentIdentityPath is the identities path of an agent, its project in the query; append further parameters with "&".
func AgentIdentityPath(orgName, projName, agentName string) string {
	return fmt.Sprintf("/api/v1/agents/%s/identities?%s", url.PathEscape(agentName), url.Values{"project": {projName}}.Encode())
}

// ListAgentIdentities returns both the decoded safe views and the original
// response body so security specs can prove no secret-shaped field was emitted.
func ListAgentIdentities(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) ([]framework.AgentIdentityEnvironmentView, string) {
	path := AgentIdentityPath(orgName, projName, agentName)
	if environment != "" {
		path += "&" + url.Values{"environment": {environment}}.Encode()
	}
	resp, err := client.GetWithContext(ctx, path)
	g.Expect(err).NotTo(HaveOccurred(), "list agent identities request failed")
	defer resp.Body.Close()
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	body, err := io.ReadAll(resp.Body)
	g.Expect(err).NotTo(HaveOccurred(), "read agent identities response")
	var views []framework.AgentIdentityEnvironmentView
	g.Expect(json.Unmarshal(body, &views)).To(Succeed(), "decode agent identities response")
	return views, string(body)
}

func RegenerateAgentIdentitySecret(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.AgentRegenerateSecretResponse {
	resp, err := client.PostWithContext(ctx, AgentIdentityPath(orgName, projName, agentName), framework.AgentIdentityActionRequest{Environment: environment})
	g.Expect(err).NotTo(HaveOccurred(), "regenerate AgentID secret request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.AgentRegenerateSecretResponse](g, resp, http.StatusOK)
}

// RevokeAgentIdentitySecret also returns the raw body so the suite can assert
// that a revoke response never echoes the credential it just invalidated.
func RevokeAgentIdentitySecret(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) (framework.AgentRevokeSecretResponse, string) {
	path := AgentIdentityPath(orgName, projName, agentName) + "&" + url.Values{"environment": {environment}}.Encode()
	resp, err := client.DeleteWithContext(ctx, path)
	g.Expect(err).NotTo(HaveOccurred(), "revoke AgentID secret request failed")
	defer resp.Body.Close()
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	body, err := io.ReadAll(resp.Body)
	g.Expect(err).NotTo(HaveOccurred(), "read revoke AgentID response")
	var result framework.AgentRevokeSecretResponse
	g.Expect(json.Unmarshal(body, &result)).To(Succeed(), "decode revoke AgentID response")
	return result, string(body)
}
