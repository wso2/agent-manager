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

// Validates the agent catalog lifecycle:
//  1. Wait for the shared kind (published by the promotion suite) to be available.
//  2. Create a new agent from the published kind, deploy it to the default environment,
//     invoke it, promote it to a new environment, and verify traffic and traces in both.

package catalog

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	catalogops "github.com/wso2/agent-manager/test/e2e/operations/catalog"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	gwops "github.com/wso2/agent-manager/test/e2e/operations/gateway"
	traceops "github.com/wso2/agent-manager/test/e2e/operations/trace"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

var _ = Describe("Agent catalog:", Label("catalog"), Ordered, func() {
	var (
		promotableAgent    *framework.SharedPromotableITHelpdeskAgent
		fromKindAgent string
		envName       string
		projectName   string
		endpointURL   string
		apiKey        string
		invokeReq     json.RawMessage
	)

	BeforeAll(func(ctx SpecContext) {
		Expect(Cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set")

		fromKindAgent = "e2e-catalog-kind-" + uuid.New().String()[:6]

		By("Ensuring the shared promotable IT helpdesk agent is built and deployed")
		promotableAgent = testsetup.SetupSharedPromotableITHelpdeskAgent(ctx, Client, Cfg)
		projectName = promotableAgent.ProjectName
		envName = promotableAgent.SecondEnv

		invokeReq = framework.DefaultInvokeRequest()
		GinkgoWriter.Printf("Setup complete: agent=%s env=%s project=%s\n",
			promotableAgent.AgentName, envName, projectName)
	})

	// The from-kind agent is single-use; delete it so its workload pods (default +
	// second environment) are freed. The shared promotable agent is left intact.
	AfterAll(func() {
		agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, projectName, fromKindAgent)
	})

	It("publishes a pre built  agent as a reusable catalog kind", func() {
		if catalogops.KindExists(Client, Cfg.DefaultOrg, framework.E2ESharedKindName) {
			GinkgoWriter.Printf("Kind already published, skipping: %s\n", framework.E2ESharedKindName)
			return
		}
		buildName := build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: promotableAgent.ProjectName,
			AgentName:   promotableAgent.AgentName,
			Timeout:     20 * time.Minute,
		})
		kind := catalogops.PublishKind(Default, Client,
			Cfg.DefaultOrg, promotableAgent.ProjectName, promotableAgent.AgentName,
			framework.PublishKindRequest{
				KindName:        framework.E2ESharedKindName,
				KindDisplayName: framework.E2ESharedKindName,
				KindDescription: "IT helpdesk agent kind published by the catalog e2e suite",
				Version:         framework.E2ESharedKindVersion,
				BuildName:       buildName,
				ConfigSchema: []framework.KindConfigSchemaEntry{
					{Name: "OPENAI_API_KEY", IsSecret: true, IsMandatory: true, DefaultValue: nil},
				},
			})
		// The publish-kind response (AgentKindVersionResponse) is version-scoped and does
		// not echo the kind name, so assert on the version it does return and confirm the
		// kind actually landed in the catalog (the name is our own request input).
		Expect(kind.Version).To(Equal(framework.E2ESharedKindVersion))
		Expect(catalogops.KindExists(Client, Cfg.DefaultOrg, framework.E2ESharedKindName)).To(BeTrue(),
			"published kind %q should be present in the catalog", framework.E2ESharedKindName)
		GinkgoWriter.Printf("Kind published: %s@%s\n", framework.E2ESharedKindName, framework.E2ESharedKindVersion)
	})

	It("creates a new agent from the published kind", func() {
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			Request: framework.NewAgentFromKindRequest(
				fromKindAgent,
				framework.E2ESharedKindName,
				framework.E2ESharedKindVersion,
				map[string]string{"OPENAI_API_KEY": Cfg.OpenAIAPIKey},
			),
		})
		Expect(ag.Name).To(Equal(fromKindAgent))
		GinkgoWriter.Printf("Agent from kind created: %s\n", fromKindAgent)
	})

	It("deploys the kind-based agent in the default environment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   fromKindAgent,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, projectName, fromKindAgent, Cfg.DefaultEnv, 10*time.Minute)
		GinkgoWriter.Printf("Agent from kind deployed and ready in %s\n", Cfg.DefaultEnv)
	})

	It("returns a chat response from the kind-based agent in the default environment", func() {
		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, projectName, fromKindAgent, Cfg.DefaultEnv)
		endpointURL = deployment.FirstEndpointURL(endpoints)
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL should not be empty")

		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, projectName, fromKindAgent, Cfg.DefaultEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-catalog-default-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		apiKey = apiKeyResp.ApiKey
		Expect(apiKey).NotTo(BeEmpty(), "agent API key should not be empty")

		GinkgoWriter.Printf("Invoking agent from kind at: %s\n", endpointURL)
		agentops.InvokeAgentEndpoint(fmt.Sprintf("%s/chat", endpointURL), invokeReq, apiKey)
	})

	It("promotes the kind-based agent to the second environment", func() {
		useSource := false
		resp := agentops.PromoteAgent(Default, Client, Cfg.DefaultOrg, projectName, fromKindAgent,
			framework.PromoteAgentRequest{
				SourceEnvironment:      Cfg.DefaultEnv,
				TargetEnvironment:      envName,
				UseConfigFromSourceEnv: &useSource,
				Env: []framework.EnvironmentVariable{
					{Key: "OPENAI_API_KEY", Value: Cfg.OpenAIAPIKey, IsSensitive: true},
				},
			})
		Expect(resp.TargetEnvironment).To(Equal(envName))
		GinkgoWriter.Printf("Agent promoted: %s -> %s\n", Cfg.DefaultEnv, envName)
	})

	It("deploys the kind-based agent in the second environment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projectName,
			AgentName:   fromKindAgent,
			Environment: envName,
			Timeout:     5 * time.Minute,
		})
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, projectName, fromKindAgent, envName, 10*time.Minute)
		GinkgoWriter.Printf("Agent from kind deployed and ready in %s\n", envName)
	})

	It("returns a chat response from the kind-based agent in the second environment", func() {
		gwops.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, envName, 3*time.Minute)

		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, projectName, fromKindAgent, envName)
		endpointURL = deployment.FirstEndpointURL(endpoints)
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL in new env should not be empty")

		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, projectName, fromKindAgent, envName,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-catalog-promo-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		apiKey = apiKeyResp.ApiKey
		Expect(apiKey).NotTo(BeEmpty(), "agent API key in new env should not be empty")

		GinkgoWriter.Printf("Invoking promoted agent from kind at: %s\n", endpointURL)
		agentops.InvokeAgentEndpoint(fmt.Sprintf("%s/chat", endpointURL), invokeReq, apiKey)
	})

	It("captures traces for the default-environment invocation", func() {
		traces := traceops.WaitForTraces(Client, &traceops.WaitForTracesParams{
			Organization: Cfg.DefaultOrg,
			Project:      projectName,
			Agent:        fromKindAgent,
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
			Agent:        fromKindAgent,
			Environment:  envName,
			Timeout:      2 * time.Minute,
		})
		Expect(traces.Traces).NotTo(BeEmpty(), "expected at least one trace after invocation in new env")
		GinkgoWriter.Printf("Traces in %s: %d found\n", envName, len(traces.Traces))
	})
})
