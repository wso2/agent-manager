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

// Package agentidentity wraps environment AgentID role and assignment APIs.
package agentidentity

import (
	"context"
	"fmt"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

func rolesPath(orgName, envName string) string {
	return fmt.Sprintf("/m/ai-gateway/orgs/%s/environments/%s/agent-identities/roles", orgName, envName)
}

func CreateRole(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, envName string, request framework.AgentIdentityRoleRequest) framework.AgentIdentityRoleResponse {
	response, err := client.PostWithContext(ctx, rolesPath(orgName, envName), request)
	g.Expect(err).NotTo(HaveOccurred(), "create AgentID role request failed")
	defer response.Body.Close()
	return framework.ExpectStatusAndDecode[framework.AgentIdentityRoleResponse](g, response, http.StatusCreated)
}

func UpdateRole(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, envName, roleID string, request framework.AgentIdentityRoleRequest) framework.AgentIdentityRoleResponse {
	response, err := client.PutWithContext(ctx, rolesPath(orgName, envName)+"/"+roleID, request)
	g.Expect(err).NotTo(HaveOccurred(), "update AgentID role request failed")
	defer response.Body.Close()
	return framework.ExpectStatusAndDecode[framework.AgentIdentityRoleResponse](g, response, http.StatusOK)
}

func DeleteRoleBestEffort(ctx context.Context, client *framework.AMPClient, orgName, envName, roleID string) {
	if roleID == "" {
		return
	}
	response, err := client.DeleteWithContext(ctx, rolesPath(orgName, envName)+"/"+roleID)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("teardown: delete AgentID role %q failed: %v\n", roleID, err)
		return
	}
	defer response.Body.Close()
	ginkgo.GinkgoWriter.Printf("teardown: deleted AgentID role %q (status %d)\n", roleID, response.StatusCode)
}

func AddRoleAssignments(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, envName, roleID string, assignments []framework.AgentIdentityAssignment) {
	path := rolesPath(orgName, envName) + "/" + roleID + "/assignments/add"
	response, err := client.PostWithContext(ctx, path, framework.AgentIdentityAssignmentsRequest{Assignments: assignments})
	g.Expect(err).NotTo(HaveOccurred(), "add AgentID role assignments request failed")
	defer response.Body.Close()
	framework.ExpectStatus(g, response, http.StatusOK)
}

func GetRoleAssignments(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, envName, roleID string) framework.AgentIdentityRoleAssignmentsResponse {
	path := rolesPath(orgName, envName) + "/" + roleID + "/assignments"
	response, err := client.GetWithContext(ctx, path)
	g.Expect(err).NotTo(HaveOccurred(), "get AgentID role assignments request failed")
	defer response.Body.Close()
	return framework.ExpectStatusAndDecode[framework.AgentIdentityRoleAssignmentsResponse](g, response, http.StatusOK)
}

func ListAgents(ctx context.Context, g Gomega, client *framework.AMPClient, orgName, envName string) framework.AgentIdentityAgentListResponse {
	path := fmt.Sprintf("/m/ai-gateway/orgs/%s/environments/%s/agent-identities/agents", orgName, envName)
	response, err := client.GetWithContext(ctx, path)
	g.Expect(err).NotTo(HaveOccurred(), "list AgentID agents request failed")
	defer response.Body.Close()
	return framework.ExpectStatusAndDecode[framework.AgentIdentityAgentListResponse](g, response, http.StatusOK)
}
