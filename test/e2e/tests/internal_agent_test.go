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

package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
	traceops "github.com/wso2/agent-manager/test/e2e/operations/trace"
)

// loadTestData reads a JSON file from the testdata directory and unmarshals it into dest.
func loadTestData(relPath string, dest any) {
	data, err := os.ReadFile(filepath.Join("testdata", relPath))
	Expect(err).NotTo(HaveOccurred(), "failed to read testdata file: %s", relPath)
	Expect(json.Unmarshal(data, dest)).To(Succeed(), "failed to unmarshal testdata file: %s", relPath)
}

// injectEnvVars populates environment variable values in Configurations from the provided map.
func injectEnvVars(cfg *framework.Configurations, envVars map[string]string) {
	if cfg == nil {
		return
	}
	for i := range cfg.Env {
		val, ok := envVars[cfg.Env[i].Key]
		Expect(ok && val != "").To(BeTrue(), "config value for %s must be set", cfg.Env[i].Key)
		cfg.Env[i].Value = val
	}
}

var _ = Describe("Internal Chat Agent Lifecycle", Ordered, func() {
	var (
		projName  string
		agentName string
		buildName string
		envVars   map[string]string

		createProjReq framework.CreateProjectRequest
		createReq     framework.CreateAgentRequest
		invokeReq     json.RawMessage
	)

	BeforeAll(func() {
		suffix := uuid.New().String()[:8]
		projName = e2eProjectPrefix + suffix
		agentName = "e2e-chat-" + suffix

		envVars = map[string]string{
			"TAVILY_API_KEY": Cfg.TavilyAPIKey,
			"OPENAI_API_KEY": Cfg.OpenAIAPIKey,
		}

		loadTestData("internal-chat-agent/create_project.json", &createProjReq)
		createProjReq.Name = projName

		loadTestData("internal-chat-agent/create_agent.json", &createReq)
		createReq.Name = agentName
		injectEnvVars(createReq.Configurations, envVars)

		loadTestData("internal-chat-agent/invoke_request.json", &invokeReq)
	})

	It("should create a project", func() {
		By("Creating e2e project")
		proj := project.CreateProject(Default, Client, &project.CreateProjectParams{
			OrgName: Cfg.DefaultOrg,
			Request: createProjReq,
		})
		framework.ExpectJSONMatch(Default, "internal-chat-agent/expected_create_project.json", proj)
		GinkgoWriter.Printf("Project: %s\n", projName)
	})

	It("should create an internal chat agent", func() {
		By("Creating internal chat agent")
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		framework.ExpectJSONMatch(Default, "internal-chat-agent/expected_create_agent.json", ag)
		GinkgoWriter.Printf("Agent: %s (type: %s/%s)\n", agentName, ag.AgentType.Type, ag.AgentType.SubType)
	})

	It("should complete the build", func() {
		By("Waiting for build to succeed")
		buildName = build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projName,
			AgentName:   agentName,
			Timeout:     20 * time.Minute,
		})
		Expect(buildName).NotTo(BeEmpty())
		GinkgoWriter.Printf("Build: %s\n", buildName)
	})

	It("should have build logs available", func() {
		By("Verifying build logs")
		logs := build.GetBuildLogs(Default, Client, Cfg.DefaultOrg, projName, agentName, buildName)
		Expect(logs.Logs).NotTo(BeEmpty(), "expected build logs to be available")
		GinkgoWriter.Printf("Log entries: %d\n", len(logs.Logs))
	})

	It("should deploy the agent", func() {
		By("Waiting for deployment to become active")
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})
		GinkgoWriter.Printf("Environment: %s\n", Cfg.DefaultEnv)
	})

	It("should become ready", func() {
		By("Waiting for agent readiness via runtime logs")
		agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			SearchText:  "Uvicorn running on",
			Timeout:     10 * time.Minute,
		})
	})

	It("should respond to invocation", func() {
		By("Getting agent endpoints")
		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, projName, agentName, Cfg.DefaultEnv)
		Expect(endpoints).NotTo(BeEmpty(), "expected at least one endpoint")

		var endpointURL string
		for _, ep := range endpoints {
			endpointURL = ep.URL
			break
		}
		Expect(endpointURL).NotTo(BeEmpty(), "endpoint URL should not be empty")
		endpointURL = endpointURL + "/chat"
		GinkgoWriter.Printf("Endpoint: %s\n", endpointURL)

		By("Invoking agent endpoint")
		agentops.InvokeAgentEndpoint(endpointURL, invokeReq)
	})

	It("should have metrics available", func() {
		By("Verifying agent metrics")
		metrics := agentops.GetMetrics(Default, Client, Cfg.DefaultOrg, projName, agentName, Cfg.DefaultEnv)
		Expect(metrics.CPUUsage).NotTo(BeEmpty(), "expected CPU usage metrics")
		Expect(metrics.Memory).NotTo(BeEmpty(), "expected memory metrics")
		GinkgoWriter.Printf("CPU points: %d, Memory points: %d\n", len(metrics.CPUUsage), len(metrics.Memory))
	})

	It("should have traces available", func() {
		By("Verifying traces")
		traces := traceops.WaitForTraces(Client, &traceops.WaitForTracesParams{
			Organization: Cfg.DefaultOrg,
			Project:      projName,
			Agent:        agentName,
			Environment:  Cfg.DefaultEnv,
			Timeout:      2 * time.Minute,
		})
		Expect(traces.Traces).NotTo(BeEmpty(), "expected at least one trace after agent invocation")
		GinkgoWriter.Printf("Traces: %d found\n", len(traces.Traces))
	})
})

