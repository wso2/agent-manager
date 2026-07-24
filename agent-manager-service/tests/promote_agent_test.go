//go:build integration

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testPromoteOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

// pipelineWithPath returns a mock GetProjectDeploymentPipeline func that allows source -> target.
func pipelineWithPath(source, target string) func(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error) {
	return func(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error) {
		return &models.DeploymentPipelineResponse{
			Name:        "test-pipeline",
			DisplayName: "test-pipeline",
			OrgName:     namespaceName,
			CreatedAt:   time.Now(),
			PromotionPaths: []models.PromotionPath{
				{
					SourceEnvironmentRef:  source,
					TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: target}},
				},
			},
		}, nil
	}
}

// envVarValue returns the value of the env var with the given key, or "" if absent.
func envVarValue(envVars []client.EnvVar, key string) string {
	for _, ev := range envVars {
		if ev.Key == key {
			return ev.Value
		}
	}
	return ""
}

// seedReadyAgentIdentityRow inserts a COMPLETED AgentID binding for
// (org, project, agent, env) directly into the real test database, so
// agentIdentityInjection.EnvVarsForEnvironment (which PromoteAgent now calls
// and hard-blocks on if the target's identity isn't ready — see
// agent_manager.go's PromoteAgent) treats this environment's identity as
// ready. These promotion tests exercise env-var-override merging, not
// AgentID provisioning itself, so this stands in for "the agent's identity
// already finished provisioning before promotion was attempted" without
// needing the caller to actually run the full async provisioning flow.
//
// Call this ONCE per (org, project, agent, env) — every t.Run subtest in
// TestPromoteAgent shares the same agentName, and the table has a unique
// constraint on (org, project, agent, env), so seeding it more than once for
// the same tuple fails with a duplicate-key error.
func seedReadyAgentIdentityRow(t *testing.T, orgName, projectName, agentName, envName string) {
	t.Helper()
	binding := &models.AgentThunderClient{
		OUID:             orgName,
		ProjectName:      projectName,
		AgentName:        agentName,
		EnvironmentName:  envName,
		ProvisioningType: models.AgentProvisioningTypeInternal,
		Status:           models.AgentThunderStatusCompleted,
		ThunderAgentID:   "test-thunder-agent-" + uuid.New().String()[:8],
		ThunderClientID:  "test-client-" + uuid.New().String()[:8],
		SecretRefPath:    "agent-thunder-clients/" + orgName + "/" + projectName + "/" + envName + "/" + agentName,
	}
	require.NoError(t, db.DB(context.Background()).Create(binding).Error)
}

// stubReadyAgentIdentitySecretReference stubs the OpenChoreo mock's
// SecretReference methods for the "not found yet, create it" path —
// EnvVarsForEnvironment calls these once per invocation, and each subtest
// constructs its own fresh ocClient mock (unlike the DB row, which is
// shared), so this must be called once per subtest.
func stubReadyAgentIdentitySecretReference(ocClient *clientmocks.OpenChoreoClientMock) {
	ocClient.GetSecretReferenceFunc = func(_ context.Context, _, _ string) (*client.SecretReferenceInfo, error) {
		return nil, utils.ErrNotFound
	}
	ocClient.CreateSecretReferenceFunc = func(_ context.Context, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
		return &client.SecretReferenceInfo{Name: req.Name}, nil
	}
}

func TestPromoteAgent(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])

	// PromoteAgent hard-blocks promotion when the target environment's
	// AgentID identity isn't ready (see agent_manager.go) — seed a completed
	// one for "production" up front so the subtests below, which are
	// exercising env-var-override merging rather than AgentID provisioning,
	// aren't blocked by it. Seeded once (not per-subtest) because the table
	// has a unique constraint on (org, project, agent, env) and every
	// subtest shares this same agentName/org/env combination.
	//
	// Seeded under jwtassertion.MockOUID, not testPromoteOrgName: the mock
	// middleware always mints tokens with OUID=MockOUID regardless of the
	// org handle in the URL path (see its doc comment), and PromoteAgent's
	// identity lookup is keyed by that token OUID — not the path org.
	seedReadyAgentIdentityRow(t, jwtassertion.MockOUID, "my-project", agentName, "production")

	promoteURL := func(org string) string {
		return fmt.Sprintf("/api/v1/orgs/%s/projects/my-project/agents/%s/promote", org, agentName)
	}

	t.Run("promoting along a valid path returns 202", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		stubReadyAgentIdentitySecretReference(ocClient)
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "production")
		// System-managed env var resolution parses the environment UUID, so it must be valid.
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		ocClient.PromoteComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, traitEnvConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error {
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
		require.Len(t, ocClient.PromoteComponentCalls(), 1)
		call := ocClient.PromoteComponentCalls()[0]
		require.Equal(t, "development", call.SourceEnvironment)
		require.Equal(t, "production", call.TargetEnvironment)
	})

	t.Run("with useConfigFromSourceEnv=true clones the source env overrides", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		stubReadyAgentIdentitySecretReference(ocClient)
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "production")
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		// The source env's workload overrides are what get cloned to the target.
		// Also includes a source-environment AgentID client ID — simulating a
		// stale/leaked identity value sitting in the source's own overrides —
		// so this test can prove the clone path strips it rather than carrying
		// it over to the target environment's pod.
		const leakedSourceClientID = "leaked-source-client-id"
		ocClient.GetSourceEnvWorkloadOverridesFunc = func(ctx context.Context, namespaceName, componentName, sourceEnvironment string) ([]client.EnvVar, []client.FileVar, error) {
			return []client.EnvVar{
					{Key: "FROM_SOURCE", Value: "src-value"},
					{Key: client.EnvVarAgentIDClientID, Value: leakedSourceClientID},
				},
				[]client.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "k: v"}}, nil
		}
		var capturedEnv []client.EnvVar
		var capturedFiles []client.FileVar
		ocClient.PromoteComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, traitEnvConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error {
			capturedEnv = envOverrides
			capturedFiles = fileOverrides
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{
			SourceEnvironment:      "development",
			TargetEnvironment:      "production",
			UseConfigFromSourceEnv: boolPtr(true),
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
		// Source overrides must have been read and forwarded to the promote call.
		require.Len(t, ocClient.GetSourceEnvWorkloadOverridesCalls(), 1)
		require.Len(t, ocClient.PromoteComponentCalls(), 1)
		require.Equal(t, "src-value", envVarValue(capturedEnv, "FROM_SOURCE"))
		require.Len(t, capturedFiles, 1)
		require.Equal(t, "config.yaml", capturedFiles[0].Key)
		gotClientID := envVarValue(capturedEnv, client.EnvVarAgentIDClientID)
		require.NotEqual(t, leakedSourceClientID, gotClientID,
			"the clone path must strip a stale AgentID identity value carried in the source environment's own overrides, not forward it to the target")
		require.NotEmpty(t, gotClientID,
			"the target environment's own freshly-injected AgentID client ID must still be present after stripping the source's")
	})

	t.Run("with useConfigFromSourceEnv=false forwards the provided env overrides", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		stubReadyAgentIdentitySecretReference(ocClient)
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "production")
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		var capturedEnv []client.EnvVar
		ocClient.PromoteComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, traitEnvConfigs map[string]interface{}, componentTypeConfigs map[string]interface{}) error {
			capturedEnv = envOverrides
			return nil
		}
		testClients := wiring.TestClients{
			OpenChoreoClient: ocClient,
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		payload := spec.PromoteAgentRequest{
			SourceEnvironment:      "development",
			TargetEnvironment:      "production",
			UseConfigFromSourceEnv: boolPtr(false),
			Env:                    []spec.EnvironmentVariable{{Key: "LOG_LEVEL", Value: stringPtr("debug")}},
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
		// The source env's overrides must NOT have been read in the user-driven branch.
		require.Empty(t, ocClient.GetSourceEnvWorkloadOverridesCalls())
		require.Len(t, ocClient.PromoteComponentCalls(), 1)
		require.Equal(t, "debug", envVarValue(capturedEnv, "LOG_LEVEL"))
	})

	t.Run("returns 400 when sourceEnvironment is missing", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "sourceEnvironment is required")
		require.Empty(t, ocClient.PromoteComponentCalls())
	})

	t.Run("returns 400 when source and target are the same", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "development"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "must be different")
	})

	t.Run("returns 400 when useConfigFromSourceEnv is combined with overrides", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		// PromoteComponent must never be invoked: the request is rejected at validation,
		// so calling it would panic the nil mock func and fail the test.
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{
			SourceEnvironment:      "development",
			TargetEnvironment:      "production",
			UseConfigFromSourceEnv: boolPtr(true),
			Env:                    []spec.EnvironmentVariable{{Key: "FOO", Value: stringPtr("bar")}},
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		var errResp spec.ErrorResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
		require.Equal(t, utils.ErrCodeValidation, errResp.Code)
		require.Equal(t,
			"useConfigFromSourceEnv=true is mutually exclusive with env, files, enableAutoInstrumentation, instrumentationVersion, enableApiKeySecurity, corsConfig, enableOAuthSecurity, oauthConfig, and resilienceTimeoutSeconds",
			errResp.Message)

		// The agent must not have been promoted.
		require.Empty(t, ocClient.PromoteComponentCalls())
	})

	t.Run("returns 500 when the promotion path is not allowed", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		// Pipeline only allows development -> staging, so development -> production is invalid.
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "staging")
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		require.Empty(t, ocClient.PromoteComponentCalls())
	})

	t.Run("returns 404 when the organization is not found", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, jwtassertion.NewMockMiddlewareWithOUID(t, "nonexistent-org"))

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL("nonexistent-org"), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), "Organization not found")
	})

	t.Run("returns 400 when target environment AgentID is not ready", func(t *testing.T) {
		// The hard block only fires when the pipeline's lowest environment
		// ("development") actually has a real credential to leak, so it must
		// be seeded here — otherwise promotion proceeds and panics on the
		// unconfigured PromoteComponentFunc mock below.
		seedReadyAgentIdentityRow(t, jwtassertion.MockOUID, "my-project", agentName, "development")

		ocClient := apitestutils.CreateMockOpenChoreoClient()
		stubReadyAgentIdentitySecretReference(ocClient)
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "unready-env")
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "unready-env"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "is not ready yet")
	})
}
