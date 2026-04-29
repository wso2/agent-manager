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

package deployment

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// WaitForDeploymentParams holds parameters for waiting on a deployment.
type WaitForDeploymentParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Environment string
	Timeout     time.Duration // default: 10 minutes
}

// WaitForDeployed polls the deployments API until the agent is "DEPLOYED" in
// the specified environment.
func WaitForDeployed(t *testing.T, client *framework.AMPClient, params *WaitForDeploymentParams) {
	t.Helper()

	timeout := params.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		params.OrgName, params.ProjectName, params.AgentName)

	framework.Poll(t, "deployment to be DEPLOYED", framework.PollConfig{
		Timeout:         timeout,
		InitialInterval: 10 * time.Second,
		MaxInterval:     30 * time.Second,
	}, func() (struct{}, bool, error) {
		resp, err := client.Get(path)
		if err != nil {
			return struct{}{}, false, fmt.Errorf("get deployments request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			framework.Log(t, "  Deployment check returned %d: %s", resp.StatusCode, string(body))
			return struct{}{}, false, nil
		}

		deploymentsMap := framework.DecodeBody[map[string]framework.DeploymentDetailsResponse](t, resp)

		dep, exists := deploymentsMap[params.Environment]
		if !exists {
			return struct{}{}, false, nil
		}

		if dep.Status == "active" {
			framework.Log(t, "  Deployment: %s", dep.Status)
			return struct{}{}, true, nil
		}
		return struct{}{}, false, nil
	})
}

// GetEndpoints retrieves the endpoints for an agent in a given environment.
func GetEndpoints(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, environment string) map[string]framework.EndpointConfiguration {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/endpoints?environment=%s",
		orgName, projName, agentName, environment)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get endpoints request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[map[string]framework.EndpointConfiguration](t, resp)
}
