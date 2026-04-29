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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
	traceops "github.com/wso2/agent-manager/test/e2e/operations/trace"
)

// loadTestData reads a JSON file from the testdata directory and unmarshals it into dest.
func loadTestData(t *testing.T, relPath string, dest any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", relPath))
	require.NoError(t, err, "failed to read testdata file: %s", relPath)
	require.NoError(t, json.Unmarshal(data, dest), "failed to unmarshal testdata file: %s", relPath)
}

// configEnvVars maps runtime config env var keys to their values from the test config.
var configEnvVars map[string]string

// injectEnvVars populates environment variable values in Configurations from the test config.
// Each entry's Key is looked up in the config. The test fails if a required value is missing.
func injectEnvVars(t *testing.T, cfg *framework.Configurations) {
	t.Helper()
	if cfg == nil {
		return
	}
	for i := range cfg.Env {
		val, ok := configEnvVars[cfg.Env[i].Key]
		require.True(t, ok && val != "", fmt.Sprintf("config value for %s must be set", cfg.Env[i].Key))
		cfg.Env[i].Value = val
	}
}

func TestInternalAgentLifecycle(t *testing.T) {
	log := framework.NewStepLogger(t)
	log.TestHeader("Internal Agent Lifecycle")

	configEnvVars = map[string]string{
		"TAVILY_API_KEY": Cfg.TavilyAPIKey,
		"OPENAI_API_KEY": Cfg.OpenAIAPIKey,
	}

	suffix := uuid.New().String()[:8]
	projName := e2eProjectPrefix + suffix
	agentName := "e2e-chat-" + suffix

	// Load request payloads from testdata files.
	var createReq framework.CreateAgentRequest
	loadTestData(t, "internal-chat-agent/create_agent.json", &createReq)
	createReq.Name = agentName

	injectEnvVars(t, createReq.Configurations)

	var invokeReq json.RawMessage
	loadTestData(t, "internal-chat-agent/invoke_request.json", &invokeReq)

	// ---- Step 1: Create Project ----
	log.Begin("Create E2E Project")
	stepStart := time.Now()
	project.CreateProject(t, Client, &project.CreateProjectParams{
		OrgName:            Cfg.DefaultOrg,
		Name:               projName,
		DisplayName:        fmt.Sprintf("E2E Test Project (%s)", time.Now().Format("2006-01-02 15:04:05")),
		Description:        "Auto-created project for e2e integration tests",
		DeploymentPipeline: "default",
	})
	log.Info("Project", projName)
	log.Done("Project created", stepStart)

	// ---- Step 2: Create Agent ----
	log.Begin("Create Internal Chat Agent")
	stepStart = time.Now()
	ag := agentops.CreateAgent(t, Client, &agentops.CreateAgentParams{
		OrgName:     Cfg.DefaultOrg,
		ProjectName: projName,
		Request:     createReq,
	})
	require.Equal(t, agentName, ag.Name)
	log.Info("Agent", agentName)
	log.Info("Type", fmt.Sprintf("%s/%s", ag.AgentType.Type, ag.AgentType.SubType))
	log.Done("Agent created", stepStart)

	// ---- Step 3: Wait for Build ----
	log.Begin("Wait for Build")
	stepStart = time.Now()
	buildName := build.WaitForBuildSuccess(t, Client, &build.WaitForBuildParams{
		OrgName:     Cfg.DefaultOrg,
		ProjectName: projName,
		AgentName:   agentName,
		Timeout:     20 * time.Minute,
	})
	log.Info("Build", buildName)
	log.Done("Build completed", stepStart)

	// ---- Step 4: Verify Build Logs ----
	log.Begin("Verify Build Logs")
	stepStart = time.Now()
	logs := build.GetBuildLogs(t, Client, Cfg.DefaultOrg, projName, agentName, buildName)
	require.NotEmpty(t, logs.Logs, "expected build logs to be available")
	log.Info("Log entries", fmt.Sprintf("%d", len(logs.Logs)))
	log.Done("Build logs available", stepStart)

	// ---- Step 5: Wait for Deployment ----
	log.Begin("Wait for Deployment")
	stepStart = time.Now()
	deployment.WaitForDeployed(t, Client, &deployment.WaitForDeploymentParams{
		OrgName:     Cfg.DefaultOrg,
		ProjectName: projName,
		AgentName:   agentName,
		Environment: Cfg.DefaultEnv,
		Timeout:     10 * time.Minute,
	})
	log.Info("Environment", Cfg.DefaultEnv)
	log.Done("Agent deployed", stepStart)

	// ---- Step 6: Invoke Agent ----
	log.Begin("Invoke Agent Endpoint")
	stepStart = time.Now()
	endpoints := deployment.GetEndpoints(t, Client,
		Cfg.DefaultOrg, projName, agentName, Cfg.DefaultEnv)
	require.NotEmpty(t, endpoints, "expected at least one endpoint")

	var endpointURL string
	for _, ep := range endpoints {
		endpointURL = ep.URL
		break
	}
	require.NotEmpty(t, endpointURL, "endpoint URL should not be empty")
	log.Info("Endpoint", endpointURL)

	agentops.InvokeAgentEndpoint(t, endpointURL, invokeReq)
	log.Done("Agent responded", stepStart)

	// ---- Step 7: Verify Traces ----
	log.Begin("Verify Traces")
	stepStart = time.Now()
	startTime := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	endTime := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)

	traces := framework.Poll(t, "traces to appear", framework.PollConfig{
		Timeout:         2 * time.Minute,
		InitialInterval: 10 * time.Second,
		MaxInterval:     20 * time.Second,
	}, func() (framework.TraceOverviewListResponse, bool, error) {
		result := traceops.ListTraces(t, Client, &traceops.ListTracesParams{
			Namespace:   Cfg.DefaultOrg,
			Project:     projName,
			Component:   agentName,
			Environment: Cfg.DefaultEnv,
			StartTime:   startTime,
			EndTime:     endTime,
			Limit:       10,
		})
		if len(result.Traces) > 0 {
			return result, true, nil
		}
		return result, false, nil
	})

	require.NotEmpty(t, traces.Traces, "expected at least one trace after agent invocation")
	log.Info("Traces", fmt.Sprintf("%d found", len(traces.Traces)))
	log.Done("Traces verified", stepStart)

	log.Summary()
}
