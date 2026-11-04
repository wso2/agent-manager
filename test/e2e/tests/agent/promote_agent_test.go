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

// Validates the full lifecycle of the two-environment IT helpdesk agent
// as explicit test steps: create, build, publish a kind, deploy and
// invoke in the default environment, promote to the second environment, deploy
// and invoke there, and verify distributed traces. This is the canonical build
// of the shared promotable IT helpdesk agent and the source of the kind consumed by the catalog domain; the
// configuration and llmprovider domains reuse the same agent by name via
// testsetup.SetupSharedPromotableITHelpdeskAgent.
//
// The second environment, deployment pipeline and project are infrastructure
// (not the agent under test), so they are provisioned in BeforeAll.

package agent

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	gwops "github.com/wso2/agent-manager/test/e2e/operations/gateway"
	traceops "github.com/wso2/agent-manager/test/e2e/operations/trace"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

var _ = Describe("Internal chat agent promotion across environments:", Label("agent", "promotion"), Ordered, func() {
	const agentName = framework.SharedPromotableITHelpdeskAgentName

	var (
		projectName string
		secondEnv   string
		endpointURL string
		apiKey      string
		invokeReq   json.RawMessage
		buildName   string
	)

	BeforeAll(func(ctx SpecContext) {
		Expect(Cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set")
		By("Provisioning two-env infrastructure (environment, pipeline, project)")
		projectName, secondEnv = testsetup.EnsurePromotableInfra(ctx, Client, Cfg)
		invokeReq = framework.DefaultInvokeRequest()
		GinkgoWriter.Printf("Two-env infra ready: project=%s secondEnv=%s\n", projectName, secondEnv)
	})

	It("creates a IT-helpdesk agent to promote across environments", func() {
		agentPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", Cfg.DefaultOrg, projectName, agentName)
		if framework.ResourceExists(Client, agentPath) {
			GinkgoWriter.Printf("Agent already exists, reusing: %s\n", agentName)
			return
		}
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			Request: framework.NewITHelpdeskAgentRequest(
				agentName,
				"Two-env IT helpdesk agent",
				map[string]string{
					"OPENAI_API_KEY": Cfg.OpenAIAPIKey,
					"DATABASE_URL":   "http://localhost:5000",
				},
			),
		})
		Expect(ag.Name).To(Equal(agentName))
		GinkgoWriter.Printf("Agent created: %s\n", agentName)
	})

	It("builds the agent image to completion", func() {
		buildName = build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   agentName,
			Timeout:     20 * time.Minute,
		})
		GinkgoWriter.Printf("Build completed: %s\n", buildName)
	})

	It("deploys the agent to the default environment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, projectName, agentName, Cfg.DefaultEnv, 10*time.Minute)
		GinkgoWriter.Printf("Agent deployed and ready in %s\n", Cfg.DefaultEnv)
	})

	It("serves chat traffic in the default environment", func() {
		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, Cfg.DefaultEnv)
		endpointURL = deployment.FirstEndpointURL(endpoints)
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL should not be empty")

		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, Cfg.DefaultEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-default-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		apiKey = apiKeyResp.ApiKey
		Expect(apiKey).NotTo(BeEmpty(), "agent API key should not be empty")

		GinkgoWriter.Printf("Invoking agent in default env at: %s\n", fmt.Sprintf("%s/chat", endpointURL))
		agentops.InvokeAgentEndpoint(fmt.Sprintf("%s/chat", endpointURL), invokeReq, apiKey)
	})

	It("promotes the agent to the second environment", func() {
		useSource := false
		resp := agentops.PromoteAgent(Default, Client, Cfg.DefaultOrg, projectName, agentName,
			framework.PromoteAgentRequest{
				SourceEnvironment:      Cfg.DefaultEnv,
				TargetEnvironment:      secondEnv,
				UseConfigFromSourceEnv: &useSource,
				Env: []framework.EnvironmentVariable{
					{Key: "OPENAI_API_KEY", Value: Cfg.OpenAIAPIKey, IsSensitive: true},
					{Key: "DEPLOY_ENV", Value: secondEnv},
				},
			})
		Expect(resp.TargetEnvironment).To(Equal(secondEnv))
		GinkgoWriter.Printf("Agent promoted: %s -> %s\n", Cfg.DefaultEnv, secondEnv)
	})

	It("deploys the promoted agent in the second environment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   agentName,
			Environment: secondEnv,
			Timeout:     5 * time.Minute,
		})
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, projectName, agentName, secondEnv, 10*time.Minute)
		GinkgoWriter.Printf("Agent deployed and ready in %s\n", secondEnv)
	})

	It("serves chat traffic in the second environment after promotion", func() {
		gwops.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, secondEnv, 3*time.Minute)

		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, secondEnv)
		endpointURL = deployment.FirstEndpointURL(endpoints)
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL in second env should not be empty")

		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, projectName, agentName, secondEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-promo-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		apiKey = apiKeyResp.ApiKey
		Expect(apiKey).NotTo(BeEmpty(), "agent API key in second env should not be empty")

		GinkgoWriter.Printf("Invoking promoted agent at: %s\n", fmt.Sprintf("%s/chat", endpointURL))
		agentops.InvokeAgentEndpoint(fmt.Sprintf("%s/chat", endpointURL), invokeReq, apiKey)
	})

	It("captures traces for the default-environment invocation", func() {
		traces := traceops.WaitForTraces(Client, &traceops.WaitForTracesParams{
			Organization: Cfg.DefaultOrg,
			Project:      projectName,
			Agent:        agentName,
			Environment:  Cfg.DefaultEnv,
			Timeout:      2 * time.Minute,
		})
		Expect(traces.Traces).NotTo(BeEmpty(), "expected at least one trace after invocation in default env")
		GinkgoWriter.Printf("Traces in %s: %d found\n", Cfg.DefaultEnv, len(traces.Traces))
	})

	It("captures traces for the second-environment invocation", func() {
		traces := traceops.WaitForTraces(Client, &traceops.WaitForTracesParams{
			Organization: Cfg.DefaultOrg,
			Project:      projectName,
			Agent:        agentName,
			Environment:  secondEnv,
			Timeout:      2 * time.Minute,
		})
		Expect(traces.Traces).NotTo(BeEmpty(), "expected at least one trace after invocation in second env")
		GinkgoWriter.Printf("Traces in %s: %d found\n", secondEnv, len(traces.Traces))
	})
})
