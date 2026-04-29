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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
)

func TestExternalAgentLifecycle(t *testing.T) {
	t.Parallel()

	log := framework.NewStepLogger(t, "external")
	log.TestHeader("External Agent Lifecycle")

	suffix := uuid.New().String()[:8]
	projName := e2eProjectPrefix + suffix
	agentName := "e2e-external-" + suffix

	// Load request payloads from testdata files.
	var createProjReq framework.CreateProjectRequest
	loadTestData(t, "external-agent/create_project.json", &createProjReq)
	createProjReq.Name = projName

	var createReq framework.CreateAgentRequest
	loadTestData(t, "external-agent/create_agent.json", &createReq)
	createReq.Name = agentName

	// ---- Step 1: Create Project ----
	log.Begin("Create E2E Project")
	stepStart := time.Now()
	proj := project.CreateProject(t, Client, &project.CreateProjectParams{
		OrgName: Cfg.DefaultOrg,
		Request: createProjReq,
	})
	framework.AssertJSONMatch(t, "external-agent/expected_create_project.json", proj)
	log.Info("Project", projName)
	log.Done("Project created", stepStart)

	// ---- Step 2: Create External Agent ----
	log.Begin("Create External Agent")
	stepStart = time.Now()
	ag := agentops.CreateAgent(t, Client, &agentops.CreateAgentParams{
		OrgName:     Cfg.DefaultOrg,
		ProjectName: projName,
		Request:     createReq,
	})
	require.Equal(t, agentName, ag.Name)
	framework.AssertJSONMatch(t, "external-agent/expected_create_agent.json", ag)
	log.Info("Agent", agentName)
	log.Info("Type", ag.AgentType.Type+"/"+ag.AgentType.SubType)
	log.Done("Agent created", stepStart)

	// ---- Step 3: Generate Agent Token ----
	log.Begin("Generate Agent Token")
	stepStart = time.Now()
	tokenResp := agentops.GenerateAgentToken(t, Client, Cfg.DefaultOrg, projName, agentName, "1h")
	require.NotEmpty(t, tokenResp.Token, "expected non-empty agent token")
	log.Info("Token type", tokenResp.TokenType)
	log.Done("Token generated", stepStart)

	log.Summary()
}
