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

package testsetup

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
)

// SetupSharedAgent provisions a shared internal chat agent. It is designed to
// be called from BeforeSuite. If the project/agent already exist from a
// previous run, they are reused. Returns nil when the required API keys are
// not configured.
func SetupSharedAgent(client *framework.AMPClient, cfg *framework.Config) *framework.SharedAgent {
	Expect(cfg.TavilyAPIKey).NotTo(BeEmpty(), "TAVILY_API_KEY must be set for shared agent setup")
	Expect(cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set for shared agent setup")

	shared := &framework.SharedAgent{
		ProjectName: framework.E2ESharedProjectName,
		AgentName:   framework.SharedAgentName,
	}

	envVars := map[string]string{
		"TAVILY_API_KEY": cfg.TavilyAPIKey,
		"OPENAI_API_KEY": cfg.OpenAIAPIKey,
		"DATABASE_URL":   "http://localhost:5000",
	}

	// Check if project exists
	projPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", cfg.DefaultOrg, shared.ProjectName)
	if !framework.ResourceExists(client, projPath) {
		ginkgo.By("Creating shared project")
		createProjReq := framework.NewCreateProjectRequest(shared.ProjectName, "E2E Shared Project", "Shared project for e2e tests")
		project.CreateProject(Default, client, &project.CreateProjectParams{
			OrgName: cfg.DefaultOrg,
			Request: createProjReq,
		})
		ginkgo.GinkgoWriter.Printf("Shared project created: %s\n", shared.ProjectName)
	} else {
		ginkgo.GinkgoWriter.Printf("Shared project already exists: %s\n", shared.ProjectName)
	}

	// Check if agent exists
	agentPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s",
		cfg.DefaultOrg, shared.ProjectName, shared.AgentName)
	if !framework.ResourceExists(client, agentPath) {
		ginkgo.By("Creating shared internal chat agent")
		createReq := framework.NewInternalChatAgentRequest(shared.AgentName, "Shared internal chat agent for e2e tests", envVars)
		agentops.CreateAgent(Default, client, &agentops.CreateAgentParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: shared.ProjectName,
			Request:     createReq,
		})
		ginkgo.GinkgoWriter.Printf("Shared agent created: %s\n", shared.AgentName)
	} else {
		ginkgo.GinkgoWriter.Printf("Shared agent already exists: %s\n", shared.AgentName)
	}

	// Check if already deployed and active — skip build/deploy wait if so
	if framework.IsAgentDeployed(client, cfg, shared.ProjectName, shared.AgentName) {
		ginkgo.GinkgoWriter.Println("Shared agent already deployed and active, skipping build/deploy wait")
	} else {
		ginkgo.By("Waiting for shared agent build")
		shared.BuildName = build.WaitForBuildSuccess(client, &build.WaitForBuildParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: shared.ProjectName,
			AgentName:   shared.AgentName,
			Timeout:     20 * time.Minute,
		})
		ginkgo.GinkgoWriter.Printf("Shared agent build: %s\n", shared.BuildName)

		ginkgo.By("Waiting for shared agent deployment")
		deployment.WaitForDeployed(client, &deployment.WaitForDeploymentParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: shared.ProjectName,
			AgentName:   shared.AgentName,
			Environment: cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})

		ginkgo.By("Waiting for shared agent readiness")
		agentops.WaitForRuntimeLog(client, &agentops.WaitForRuntimeLogParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: shared.ProjectName,
			AgentName:   shared.AgentName,
			Environment: cfg.DefaultEnv,
			SearchText:  "Uvicorn running on",
			Timeout:     10 * time.Minute,
		})
	}

	ginkgo.By("Getting shared agent endpoint")
	endpoints := deployment.GetEndpoints(Default, client,
		cfg.DefaultOrg, shared.ProjectName, shared.AgentName, cfg.DefaultEnv)
	for _, ep := range endpoints {
		if ep.URL != "" {
			shared.EndpointURL = ep.URL
			break
		}
	}
	Expect(shared.EndpointURL).NotTo(BeEmpty(), "shared agent endpoint URL should not be empty")

	shared.InvokeReq = framework.DefaultInvokeRequest()

	ginkgo.By("Creating shared agent API key")
	apiKeyResp := agentops.CreateAgentAPIKey(Default, client,
		cfg.DefaultOrg, shared.ProjectName, shared.AgentName, cfg.DefaultEnv,
		framework.CreateAgentAPIKeyRequest{
			DisplayName: "e2e-test-key",
			ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})
	shared.APIKey = apiKeyResp.ApiKey
	Expect(shared.APIKey).NotTo(BeEmpty(), "shared agent API key should not be empty")

	ginkgo.GinkgoWriter.Printf("Shared agent ready: project=%s agent=%s endpoint=%s\n",
		shared.ProjectName, shared.AgentName, shared.EndpointURL)

	return shared
}
