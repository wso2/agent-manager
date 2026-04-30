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
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

// WaitForDeployed polls the deployments API until the agent is "active" in
// the specified environment.
func WaitForDeployed(client *framework.AMPClient, params *WaitForDeploymentParams) {
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		params.OrgName, params.ProjectName, params.AgentName)

	Eventually(func(g Gomega) {
		resp, err := client.Get(path)
		g.Expect(err).NotTo(HaveOccurred(), "get deployments request failed")
		defer resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			StopTrying(fmt.Sprintf("deployments check returned %d", resp.StatusCode)).Now()
		}
		g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "deployments check returned %d", resp.StatusCode)
		deploymentsMap := framework.DecodeBody[map[string]framework.DeploymentDetailsResponse](g, resp)

		dep, exists := deploymentsMap[params.Environment]
		g.Expect(exists).To(BeTrue(), "environment %q not found in deployments", params.Environment)

		ginkgo.GinkgoWriter.Printf("Deployment status: %s\n", dep.Status)
		g.Expect(dep.Status).To(Equal("active"), "deployment not yet active")
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())
}

// GetEndpoints retrieves the endpoints for an agent in a given environment.
func GetEndpoints(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) map[string]framework.EndpointConfiguration {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/endpoints?environment=%s",
		orgName, projName, agentName, environment)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get endpoints request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[map[string]framework.EndpointConfiguration](g, resp)
}
