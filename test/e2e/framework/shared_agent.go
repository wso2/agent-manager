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

package framework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2EProjectPrefix is the naming prefix for all e2e test projects.
const E2EProjectPrefix = "e2e-test-"

// E2ESharedProjectName is the project name used by the shared internal chat agent.
const E2ESharedProjectName = "e2e-test-shared"

// SharedAgentName is the agent name used by the shared internal chat agent.
const SharedAgentName = "e2e-test-shared-chat"

// SharedAgent holds details of the shared internal chat agent that is
// provisioned once in BeforeSuite and reused by multiple test suites.
type SharedAgent struct {
	ProjectName string          `json:"projectName"`
	AgentName   string          `json:"agentName"`
	BuildName   string          `json:"buildName"`
	EndpointURL string          `json:"endpointURL"`
	APIKey      string          `json:"apiKey"`
	InvokeReq   json.RawMessage `json:"invokeReq"`
}

// WaitForAPIReady polls the health endpoint until the API is ready.
func WaitForAPIReady(cfg *Config) {
	healthClient := &http.Client{Timeout: 5 * time.Second}
	Eventually(func() int {
		resp, err := healthClient.Get(cfg.AMPBaseURL + "/healthz")
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}).WithTimeout(cfg.ReadinessTimeout).WithPolling(2 * time.Second).Should(Equal(http.StatusOK))
	ginkgo.GinkgoWriter.Println("API is ready")
}

// VerifyDefaultOrg verifies the default organization exists.
func VerifyDefaultOrg(client *AMPClient, orgName string) {
	resp, err := client.Get("/api/v1/orgs")
	Expect(err).NotTo(HaveOccurred(), "list orgs")
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "list orgs status")

	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred(), "read orgs response")

	var list OrganizationListResponse
	Expect(json.Unmarshal(body, &list)).To(Succeed(), "decode orgs response")

	found := false
	for _, org := range list.Organizations {
		if org.Name == orgName {
			found = true
			break
		}
	}
	Expect(found).To(BeTrue(), "default org %q not found in %d organizations", orgName, list.Total)
	ginkgo.GinkgoWriter.Printf("Default org %q verified\n", orgName)
}

// ResourceExists checks if an API resource exists (returns 200).
func ResourceExists(client *AMPClient, path string) bool {
	resp, err := client.Get(path)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// IsAgentDeployed checks if the agent is deployed and active in the given environment.
func IsAgentDeployed(client *AMPClient, cfg *Config, projName, agentName string) bool {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		cfg.DefaultOrg, projName, agentName)
	resp, err := client.Get(path)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	deploymentsMap := DecodeBody[map[string]DeploymentDetailsResponse](Default, resp)
	dep, exists := deploymentsMap[cfg.DefaultEnv]
	if !exists {
		return false
	}
	return dep.Status == "active"
}
