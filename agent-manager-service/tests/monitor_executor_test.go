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
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// realEvaluators returns a realistic set of evaluators spanning all levels (trace, agent, llm)
// with varied config shapes, including arrays and nested booleans.
func realEvaluators() []models.MonitorEvaluator {
	return []models.MonitorEvaluator{
		{Identifier: "latency_performance", DisplayName: "Latency Check", Config: map[string]interface{}{"level": "trace", "max_latency_ms": float64(3000), "use_task_constraint": false}},
		{Identifier: "iteration_efficiency", DisplayName: "Iteration Count", Config: map[string]interface{}{"level": "trace", "max_iterations": float64(5), "use_context_constraint": false}},
		{Identifier: "token_efficiency", DisplayName: "Token Efficiency", Config: map[string]interface{}{"level": "trace", "max_tokens": float64(4000), "use_context_constraint": false}},
		{Identifier: "content_coverage", DisplayName: "Required Content", Config: map[string]interface{}{"level": "trace", "required_strings": []interface{}{"hello"}}},
		{Identifier: "content_safety", DisplayName: "Prohibited Content", Config: map[string]interface{}{
			"level":                  "trace",
			"case_sensitive":         false,
			"prohibited_strings":     []interface{}{"internal error", "stack trace", "debug:", "hotels"},
			"use_context_prohibited": false,
		}},
		{Identifier: "length_compliance", DisplayName: "Answer Length", Config: map[string]interface{}{"level": "trace", "max_length": float64(5000), "min_length": float64(10)}},
		{Identifier: "latency_performance", DisplayName: "Agent Latency", Config: map[string]interface{}{"level": "agent", "max_latency_ms": float64(5000), "use_task_constraint": true}},
		{Identifier: "latency_performance", DisplayName: "Span Latency", Config: map[string]interface{}{"level": "llm", "max_latency_ms": float64(1000), "use_task_constraint": true}},
	}
}

// seedMonitor creates a monitor row in the DB that satisfies FK constraints for monitor_runs.
func seedMonitor(t *testing.T) *models.Monitor {
	t.Helper()
	gdb := db.DB(context.Background())

	evaluators := realEvaluators()
	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            "exec-test-" + uuid.New().String()[:8],
		DisplayName:     "Executor Test Monitor",
		Type:            models.MonitorTypePast,
		OrgName:         "test-org",
		ProjectName:     "test-project",
		AgentName:       "test-agent",
		AgentID:         "00000000-0000-0000-0000-000000000001",
		EnvironmentName: "default",
		EnvironmentID:   "00000000-0000-0000-0000-000000000002",
		Evaluators:      evaluators,
		SamplingRate:    0.8,
	}
	require.NoError(t, gdb.Create(monitor).Error)
	// GORM's serializer:json may mutate the slice in-place after Create; restore with a fresh copy
	monitor.Evaluators = realEvaluators()

	t.Cleanup(func() {
		gdb.Where("monitor_id = ?", monitor.ID).Delete(&models.MonitorRun{})
		gdb.Delete(monitor)
	})

	return monitor
}

// TestExecuteMonitorRun_CRStructure verifies that the WorkflowRun request submitted to
// CreateWorkflowRun has the correct workflow name and parameters.
func TestExecuteMonitorRun_CRStructure(t *testing.T) {
	monitor := seedMonitor(t)

	var capturedReq client.CreateWorkflowRunRequest
	var capturedNamespace string
	mockClient := &clientmocks.OpenChoreoClientMock{
		CreateWorkflowRunFunc: func(ctx context.Context, namespaceName string, req client.CreateWorkflowRunRequest) (*client.WorkflowRunResponse, error) {
			capturedNamespace = namespaceName
			capturedReq = req
			return &client.WorkflowRunResponse{
				Name:         "test-workflow-run-123",
				WorkflowName: req.WorkflowName,
				Status:       "Running",
				OrgName:      namespaceName,
			}, nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()), repositories.NewCustomEvaluatorRepo(db.GetDB()), repositories.NewOrgPublisherCredentialRepo(db.GetDB()), repositories.NewMonitorLLMMappingRepository(db.GetDB()), repositories.NewGatewayRepo(db.GetDB()))

	startTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  startTime,
		EndTime:    endTime,
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// --- Verify namespace ---
	assert.Equal(t, monitor.OrgName, capturedNamespace)

	// --- Verify workflow name ---
	assert.Equal(t, "monitor-evaluation-workflow", capturedReq.WorkflowName)

	// --- Verify parameters ---
	params := capturedReq.Parameters

	// Monitor params
	monitorParams := params["monitor"].(map[string]interface{})
	assert.Equal(t, monitor.Name, monitorParams["name"])
	assert.Equal(t, monitor.DisplayName, monitorParams["displayName"])

	// Agent params
	agentParams := params["agent"].(map[string]interface{})
	assert.Equal(t, monitor.AgentID, agentParams["id"])

	// Environment params
	envParams := params["environment"].(map[string]interface{})
	assert.Equal(t, monitor.EnvironmentID, envParams["id"])

	// Evaluation params
	evalParams := params["evaluation"].(map[string]interface{})
	assert.Equal(t, monitor.SamplingRate, evalParams["samplingRate"])
	assert.Equal(t, "2026-01-15T10:00:00Z", evalParams["traceStart"])
	assert.Equal(t, "2026-01-15T11:00:00Z", evalParams["traceEnd"])

	// Publishing params
	pubParams := params["publishing"].(map[string]interface{})
	assert.Equal(t, monitor.ID.String(), pubParams["monitorId"])
	assert.NotEmpty(t, pubParams["runId"])

	// Verify the run ID in publishing matches the DB record
	assert.Equal(t, result.Run.ID.String(), pubParams["runId"])
}

// TestExecuteMonitorRun_EvaluatorsJSON verifies that the evaluators are serialized as a
// JSON string in the request and that the full evaluator data (identifiers, display names,
// levels, and configs including arrays) round-trips correctly.
func TestExecuteMonitorRun_EvaluatorsJSON(t *testing.T) {
	monitor := seedMonitor(t)

	var capturedReq client.CreateWorkflowRunRequest
	mockClient := &clientmocks.OpenChoreoClientMock{
		CreateWorkflowRunFunc: func(ctx context.Context, namespaceName string, req client.CreateWorkflowRunRequest) (*client.WorkflowRunResponse, error) {
			capturedReq = req
			return &client.WorkflowRunResponse{
				Name:         "test-workflow-run-123",
				WorkflowName: req.WorkflowName,
				Status:       "Running",
				OrgName:      namespaceName,
			}, nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()), repositories.NewCustomEvaluatorRepo(db.GetDB()), repositories.NewOrgPublisherCredentialRepo(db.GetDB()), repositories.NewMonitorLLMMappingRepository(db.GetDB()), repositories.NewGatewayRepo(db.GetDB()))

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Extract the evaluators JSON string from the request parameters
	evalParams := capturedReq.Parameters["evaluation"].(map[string]interface{})

	evaluatorsStr, ok := evalParams["evaluators"].(string)
	require.True(t, ok, "evaluators should be a JSON string")

	// The serialized format merges "level" into config for the amp-evaluation SDK.
	// Deserialize into the eval-job format (no top-level Level field).
	type EvalJobEvaluator struct {
		Identifier  string                 `json:"identifier"`
		DisplayName string                 `json:"displayName"`
		Config      map[string]interface{} `json:"config"`
	}

	var evaluators []EvalJobEvaluator
	require.NoError(t, json.Unmarshal([]byte(evaluatorsStr), &evaluators))
	require.Len(t, evaluators, 8)

	// Verify all levels are represented (level is inside config)
	levels := map[string]int{}
	for _, e := range evaluators {
		lvl, ok := e.Config["level"].(string)
		require.True(t, ok, "config.level should be a string for %s", e.DisplayName)
		levels[lvl]++
	}
	assert.Equal(t, 6, levels["trace"])
	assert.Equal(t, 1, levels["agent"])
	assert.Equal(t, 1, levels["llm"])

	// Verify a specific evaluator with simple config
	latencyCheck := evaluators[0]
	assert.Equal(t, "latency_performance", latencyCheck.Identifier)
	assert.Equal(t, "Latency Check", latencyCheck.DisplayName)
	assert.Equal(t, "trace", latencyCheck.Config["level"])
	assert.Equal(t, float64(3000), latencyCheck.Config["max_latency_ms"])
	assert.Equal(t, false, latencyCheck.Config["use_task_constraint"])

	// Verify evaluator with array config (prohibited_content)
	prohibitedContent := evaluators[4]
	assert.Equal(t, "content_safety", prohibitedContent.Identifier)
	assert.Equal(t, "Prohibited Content", prohibitedContent.DisplayName)
	assert.Equal(t, "trace", prohibitedContent.Config["level"])
	prohibitedStrings, ok := prohibitedContent.Config["prohibited_strings"].([]interface{})
	require.True(t, ok, "prohibited_strings should be an array")
	assert.Len(t, prohibitedStrings, 4)
	assert.Contains(t, prohibitedStrings, "internal error")
	assert.Contains(t, prohibitedStrings, "stack trace")
	assert.Contains(t, prohibitedStrings, "debug:")
	assert.Contains(t, prohibitedStrings, "hotels")

	// Verify same identifier with different display names across levels
	agentLatency := evaluators[6]
	assert.Equal(t, "latency_performance", agentLatency.Identifier)
	assert.Equal(t, "Agent Latency", agentLatency.DisplayName)
	assert.Equal(t, "agent", agentLatency.Config["level"])
	assert.Equal(t, float64(5000), agentLatency.Config["max_latency_ms"])

	spanLatency := evaluators[7]
	assert.Equal(t, "latency_performance", spanLatency.Identifier)
	assert.Equal(t, "Span Latency", spanLatency.DisplayName)
	assert.Equal(t, "llm", spanLatency.Config["level"])
	assert.Equal(t, float64(1000), spanLatency.Config["max_latency_ms"])
}

// TestExecuteMonitorRun_DBRecordCreated verifies that a monitor_runs row is created
// in the database with the correct evaluator snapshot and time range.
func TestExecuteMonitorRun_DBRecordCreated(t *testing.T) {
	monitor := seedMonitor(t)

	mockClient := &clientmocks.OpenChoreoClientMock{
		CreateWorkflowRunFunc: func(ctx context.Context, namespaceName string, req client.CreateWorkflowRunRequest) (*client.WorkflowRunResponse, error) {
			return &client.WorkflowRunResponse{
				Name:         "test-workflow-run-123",
				WorkflowName: req.WorkflowName,
				Status:       "Running",
				OrgName:      namespaceName,
			}, nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()), repositories.NewCustomEvaluatorRepo(db.GetDB()), repositories.NewOrgPublisherCredentialRepo(db.GetDB()), repositories.NewMonitorLLMMappingRepository(db.GetDB()), repositories.NewGatewayRepo(db.GetDB()))

	startTime := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	endTime := time.Now().Add(-1 * time.Hour).Truncate(time.Millisecond)

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  startTime,
		EndTime:    endTime,
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify DB record
	var run models.MonitorRun
	require.NoError(t, db.DB(context.Background()).
		Where("id = ?", result.Run.ID).
		First(&run).Error)

	assert.Equal(t, monitor.ID, run.MonitorID)
	assert.Equal(t, models.RunStatusPending, run.Status)
	assert.WithinDuration(t, startTime, run.TraceStart, time.Second)
	assert.WithinDuration(t, endTime, run.TraceEnd, time.Second)

	// Verify evaluators are snapshotted in the run
	require.Len(t, run.Evaluators, 8)
	assert.Equal(t, "Latency Check", run.Evaluators[0].DisplayName)
	assert.Equal(t, "Span Latency", run.Evaluators[7].DisplayName)
}

// TestExecuteMonitorRun_LLMCredentials verifies that when a MonitorLLMMapping and a Gateway
// are seeded for a monitor, the workflow request includes the correct llmProxySecretPath
// and llmApiBase derived from those records.
func TestExecuteMonitorRun_LLMCredentials(t *testing.T) {
	monitor := seedMonitor(t)
	gdb := db.DB(context.Background())

	// Seed an LLMProxy row — Handle is a join-derived field so it is left empty here;
	// buildProxyURL only needs Configuration.Context from this row.
	// session_replication_role disables FK triggers so we don't need to seed artifacts/llm_providers.
	contextPath := "/test-proxy-ctx"
	proxyUUID := uuid.New()
	require.NoError(t, gdb.Exec("SET session_replication_role = 'replica'").Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO llm_proxies (uuid, project_uuid, provider_uuid, status, configuration)
		 VALUES (?, ?, ?, 'deployed', ?)`,
		proxyUUID, uuid.New(), uuid.New(), `{"context":"/test-proxy-ctx"}`,
	).Error)
	require.NoError(t, gdb.Exec("SET session_replication_role = 'origin'").Error)
	proxy := &models.LLMProxy{UUID: proxyUUID}

	// Seed the MonitorLLMMapping linking monitor to proxy.
	mapping := &models.MonitorLLMMapping{
		MonitorID:    monitor.ID,
		LLMProxyUUID: proxyUUID,
	}
	require.NoError(t, gdb.Create(mapping).Error)

	// Seed a Gateway and environment mapping for the monitor's environment.
	gwUUID := uuid.New()
	envUUID, err := uuid.Parse(monitor.EnvironmentID)
	require.NoError(t, err)
	gw := &models.Gateway{
		UUID:                     gwUUID,
		OrganizationName:         monitor.OrgName,
		Name:                     "test-gateway-" + gwUUID.String()[:8],
		DisplayName:              "Test Gateway",
		Vhost:                    "https://gw.example.com",
		IsActive:                 true,
		Properties:               map[string]interface{}{},
		GatewayFunctionalityType: "regular",
	}
	require.NoError(t, gdb.Create(gw).Error)
	require.NoError(t, gdb.Exec(
		"INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)",
		gwUUID, envUUID,
	).Error)

	t.Cleanup(func() {
		gdb.Exec("DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?", gwUUID)
		gdb.Delete(gw)
		gdb.Where("monitor_id = ?", monitor.ID).Delete(&models.MonitorLLMMapping{})
		gdb.Delete(proxy)
	})

	var capturedReq client.CreateWorkflowRunRequest
	mockClient := &clientmocks.OpenChoreoClientMock{
		CreateWorkflowRunFunc: func(ctx context.Context, _ string, req client.CreateWorkflowRunRequest) (*client.WorkflowRunResponse, error) {
			capturedReq = req
			return &client.WorkflowRunResponse{Name: "run-1", WorkflowName: req.WorkflowName, Status: "Running", OrgName: monitor.OrgName}, nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()), repositories.NewCustomEvaluatorRepo(db.GetDB()), repositories.NewOrgPublisherCredentialRepo(db.GetDB()), repositories.NewMonitorLLMMappingRepository(db.GetDB()), repositories.NewGatewayRepo(db.GetDB()))

	_, err = executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)

	evalParams := capturedReq.Parameters["evaluation"].(map[string]interface{})

	// Secret path is deterministic: <orgName>/monitor-<monitorID>/llm-proxy-configs
	expectedSecretPath := monitor.OrgName + "/monitor-" + monitor.ID.String() + "/llm-proxy-configs"
	assert.Equal(t, expectedSecretPath, evalParams["llmProxySecretPath"], "llmProxySecretPath should match composite secret location")

	// Proxy URL is gateway vhost + context path.
	expectedProxyURL := "https://gw.example.com" + contextPath
	assert.Equal(t, expectedProxyURL, evalParams["llmApiBase"], "llmApiBase should be gateway vhost + proxy context path")
}

// TestExecuteMonitorRun_NilEvaluatorsReturnsError verifies that calling ExecuteMonitorRun
// with nil evaluators returns an error immediately.
func TestExecuteMonitorRun_NilEvaluatorsReturnsError(t *testing.T) {
	monitor := seedMonitor(t)

	mockClient := &clientmocks.OpenChoreoClientMock{
		CreateWorkflowRunFunc: func(ctx context.Context, namespaceName string, req client.CreateWorkflowRunRequest) (*client.WorkflowRunResponse, error) {
			t.Fatal("CreateWorkflowRun should not be called with nil evaluators")
			return nil, errors.New("unexpected call")
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()), repositories.NewCustomEvaluatorRepo(db.GetDB()), repositories.NewOrgPublisherCredentialRepo(db.GetDB()), repositories.NewMonitorLLMMappingRepository(db.GetDB()), repositories.NewGatewayRepo(db.GetDB()))

	_, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Evaluators: nil,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evaluators must not be empty")
}

// TestExecuteMonitorRun_PerOrgPublisherCredentials verifies that when per-org
// publisher credentials exist in the DB, buildPublishingParams uses them.
func TestExecuteMonitorRun_PerOrgPublisherCredentials(t *testing.T) {
	monitor := seedMonitor(t)
	gdb := db.DB(context.Background())

	// Seed per-org publisher credentials
	cred := &models.OrgPublisherCredential{
		OrgName:      monitor.OrgName,
		OrgUUID:      "test-ou-uuid",
		ClientID:     "amp-publisher-test-org",
		SecretKVPath: "secret/data/test-org/amp-publisher-test-org",
		SecretKey:    "client-secret",
	}
	require.NoError(t, gdb.Create(cred).Error)
	t.Cleanup(func() {
		gdb.Where("org_name = ?", monitor.OrgName).Delete(&models.OrgPublisherCredential{})
	})

	var capturedReq client.CreateWorkflowRunRequest
	mockClient := &clientmocks.OpenChoreoClientMock{
		CreateWorkflowRunFunc: func(ctx context.Context, namespaceName string, req client.CreateWorkflowRunRequest) (*client.WorkflowRunResponse, error) {
			capturedReq = req
			return &client.WorkflowRunResponse{
				Name:         "test-workflow-run-123",
				WorkflowName: req.WorkflowName,
				Status:       "Running",
				OrgName:      namespaceName,
			}, nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()), repositories.NewCustomEvaluatorRepo(db.GetDB()), repositories.NewOrgPublisherCredentialRepo(db.GetDB()), repositories.NewMonitorLLMMappingRepository(db.GetDB()), repositories.NewGatewayRepo(db.GetDB()))

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	pubParams := capturedReq.Parameters["publishing"].(map[string]interface{})
	assert.Equal(t, "amp-publisher-test-org", pubParams["clientId"])
	assert.Equal(t, "secret/data/test-org/amp-publisher-test-org", pubParams["secretKVPath"])
	assert.Equal(t, "client-secret", pubParams["secretKey"])
}

// TestExecuteMonitorRun_FallbackPublisherCredentials verifies that when no per-org
// publisher credentials exist, buildPublishingParams falls back to static defaults.
func TestExecuteMonitorRun_FallbackPublisherCredentials(t *testing.T) {
	monitor := seedMonitor(t)

	var capturedReq client.CreateWorkflowRunRequest
	mockClient := &clientmocks.OpenChoreoClientMock{
		CreateWorkflowRunFunc: func(ctx context.Context, namespaceName string, req client.CreateWorkflowRunRequest) (*client.WorkflowRunResponse, error) {
			capturedReq = req
			return &client.WorkflowRunResponse{
				Name:         "test-workflow-run-123",
				WorkflowName: req.WorkflowName,
				Status:       "Running",
				OrgName:      namespaceName,
			}, nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()), repositories.NewCustomEvaluatorRepo(db.GetDB()), repositories.NewOrgPublisherCredentialRepo(db.GetDB()), repositories.NewMonitorLLMMappingRepository(db.GetDB()), repositories.NewGatewayRepo(db.GetDB()))

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	pubParams := capturedReq.Parameters["publishing"].(map[string]interface{})
	assert.Equal(t, "amp-publisher-client", pubParams["clientId"])
	assert.Equal(t, "amp-publisher-client-secret", pubParams["secretKVPath"])
	assert.Equal(t, "value", pubParams["secretKey"])
}
