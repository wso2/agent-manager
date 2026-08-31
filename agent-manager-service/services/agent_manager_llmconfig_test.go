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

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// spyConfigService records the request passed to Create and stubs the system-managed env var
// resolvers; the embedded interface panics on any other method.
type spyConfigService struct {
	AgentConfigurationService
	lastReq        models.CreateAgentModelConfigRequest
	systemEnvVars  []client.EnvVar
	systemEnvVarsE error
	systemEnvKeys  map[string]bool
}

func (s *spyConfigService) Create(_ context.Context, _, _, _ string,
	req models.CreateAgentModelConfigRequest, _ string,
) (*models.AgentModelConfigResponse, error) {
	s.lastReq = req
	return &models.AgentModelConfigResponse{}, nil
}

func (s *spyConfigService) BuildSystemManagedEnvVarsFromConfig(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) {
	return s.systemEnvVars, s.systemEnvVarsE
}

func (s *spyConfigService) ListSystemManagedEnvVarKeys(_ context.Context, _, _, _, _ string) (map[string]bool, error) {
	return s.systemEnvKeys, nil
}

func (s *spyConfigService) ListAgentLLMConfigSecretReferences(_ context.Context, _, _, _ string) (map[string]struct{}, error) {
	return map[string]struct{}{}, nil
}

func TestCreateAgentLLMConfigs_KeysUnderFirstEnv(t *testing.T) {
	spy := &spyConfigService{}
	s := &agentManagerService{agentConfigurationService: spy}

	req := &spec.CreateAgentRequest{
		Name:        "my-agent",
		ModelConfig: []spec.ModelConfigRequest{{ProviderName: "openai"}},
	}

	err := s.createAgentLLMConfigs(context.Background(), "org", "proj", "Development", req)
	require.NoError(t, err)

	require.Len(t, spy.lastReq.EnvMappings, 1, "exactly one env mapping")
	got, ok := spy.lastReq.EnvMappings["Development"]
	require.True(t, ok, "config must be keyed under firstEnv")
	require.Equal(t, "openai", got.ProviderName)
}

// TestMergeKindWorkloadSystemEnvVars_InjectsLLMEnvVars verifies that for a kind-sourced agent
// with an LLM configuration, the resolved system-managed LLM env vars are appended to the
// user-supplied env vars that get baked into the Workload CR. Regression test for the bug where
// LLM provider keys were written to the (unused) Component workflow params instead of the Workload.
func TestMergeKindWorkloadSystemEnvVars_InjectsLLMEnvVars(t *testing.T) {
	llmVars := []client.EnvVar{
		{Key: "OPENAI_BASE_URL", Value: "https://gw/openai"},
		{Key: "OPENAI_API_KEY", ValueFrom: &client.EnvVarValueFrom{
			SecretKeyRef: &client.SecretKeyRef{Name: "secret-ref", Key: "api-key"},
		}},
	}
	spy := &spyConfigService{systemEnvVars: llmVars}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars)
	require.NoError(t, err)
	require.Equal(t, append(append([]client.EnvVar{}, userVars...), llmVars...), got,
		"user env vars must be preserved and LLM env vars appended")
}

// TestMergeKindWorkloadSystemEnvVars_InjectsMCPEnvVars verifies that a kind-sourced agent
// configured with ONLY an MCP connection (no LLM provider) still gets its system-managed MCP env
// vars baked into the Workload CR. Regression test for the bug where the injection was gated on
// the presence of a model config, so an MCP-only agent deployed with no MCP URL or API key in its
// container and failed on every tool call.
func TestMergeKindWorkloadSystemEnvVars_InjectsMCPEnvVars(t *testing.T) {
	mcpVars := []client.EnvVar{
		{Key: "MY_AGENT_MCP_1_URL", Value: "https://gw/default/booking/mcp"},
		{Key: "MY_AGENT_MCP_1_API_KEY", ValueFrom: &client.EnvVarValueFrom{
			SecretKeyRef: &client.SecretKeyRef{Name: "secret-ref", Key: "api-key"},
		}},
	}
	spy := &spyConfigService{systemEnvVars: mcpVars}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars)
	require.NoError(t, err)
	require.Equal(t, append(append([]client.EnvVar{}, userVars...), mcpVars...), got,
		"user env vars must be preserved and MCP env vars appended")
}

// TestMergeKindWorkloadSystemEnvVars_NoSystemConfig verifies that an agent whose configs yield no
// system-managed vars gets its user env vars back unchanged. The resolver is still consulted — it
// is the authority on which configs exist — and reports the empty result itself.
func TestMergeKindWorkloadSystemEnvVars_NoSystemConfig(t *testing.T) {
	spy := &spyConfigService{systemEnvVars: nil}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars)
	require.NoError(t, err)
	require.Equal(t, userVars, got)
}

// TestMergeKindWorkloadSystemEnvVars_ResolverError verifies the resolver error is propagated so the
// caller can roll back the partially-created agent rather than deploying without system keys.
func TestMergeKindWorkloadSystemEnvVars_ResolverError(t *testing.T) {
	resolverErr := errors.New("boom")
	spy := &spyConfigService{systemEnvVarsE: resolverErr}
	s := &agentManagerService{agentConfigurationService: spy}

	_, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", nil)
	require.ErrorIs(t, err, resolverErr, "resolver error must stay unwrappable so callers can inspect it")
}

// #1736: vars configured while the first build ran exist only in the DB, so a deploy that
// rebuilds from live cluster state alone would full-replace the overrides without them.
func TestDeployAgent_InjectsSystemEnvVarsAbsentFromClusterState(t *testing.T) {
	llmVars := []client.EnvVar{
		{Key: "OPENAI_BASE_URL", Value: "http://gateway.cluster.local/openai"},
		{Key: "OPENAI_API_KEY", ValueFrom: &client.EnvVarValueFrom{
			SecretKeyRef: &client.SecretKeyRef{Name: "secret-ref", Key: "api-key"},
		}},
	}
	s, _, capturedOverrides := deployAPIAgentMocks(nil)
	s.agentConfigurationService = &spyConfigService{
		systemEnvVars: llmVars,
		systemEnvKeys: map[string]bool{"OPENAI_BASE_URL": true, "OPENAI_API_KEY": true},
	}

	_, err := s.DeployAgent(auditableCtx(t), "acme", "proj1", "my-agent",
		&spec.DeployAgentRequest{ImageId: "registry.example.com/my-agent:v1"})

	require.NoError(t, err)
	require.Equal(t, llmVars, *capturedOverrides, "workload overrides must carry the DB-tracked LLM vars")
}

// Drives GetAgentDeployments for an internal API agent in one environment; liveEnv is what the
// cluster already carries. Captures the env vars any reconcile writes to the binding.
func deploymentsReconcileMocks(liveEnv []models.EnvVars, spy *spyConfigService) (*agentManagerService, *[]client.EnvVar, *int) {
	var captured []client.EnvVar
	writes := 0
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{Name: name, DeploymentPipeline: "default"}, nil
		},
		GetDeploymentsFunc: func(context.Context, string, string, string, string) ([]*models.DeploymentResponse, error) {
			return []*models.DeploymentResponse{{AgentName: "it-help-5", Environment: "default"}}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: string(utils.AgentTypeAPI)},
			}, nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "default"}}, nil
		},
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return liveEnv, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
			captured = envVars
			writes++
			return nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentConfigurationService: spy, logger: discardLogger()}
	return s, &captured, &writes
}

// #1736 creation path: the LLM config is created after the build is triggered and AutoDeploy
// builds the first binding from that build's Workload, so DeployAgent never runs. The
// deployments poll must converge the binding on the DB.
func TestGetAgentDeployments_InjectsSystemEnvVarsMissingFromBinding(t *testing.T) {
	llmVars := []client.EnvVar{
		{Key: "OPENAI_URL", Value: "http://gateway.cluster.local/openai"},
		{Key: "OPENAI_API_KEY", ValueFrom: &client.EnvVarValueFrom{
			SecretKeyRef: &client.SecretKeyRef{Name: "proxy-secrets", Key: "api-key"},
		}},
	}
	spy := &spyConfigService{
		systemEnvVars: llmVars,
		systemEnvKeys: map[string]bool{"OPENAI_URL": true, "OPENAI_API_KEY": true},
	}
	s, captured, writes := deploymentsReconcileMocks([]models.EnvVars{{Key: "OPENAI_API_TEST", Value: "123"}}, spy)

	_, err := s.GetAgentDeployments(context.Background(), "acme", "default", "it-help-5")

	require.NoError(t, err)
	require.Equal(t, 1, *writes, "the binding must be patched exactly once")
	assert.ElementsMatch(t, llmVars, *captured, "both LLM vars must be merged into the binding")
}

// UpdateReleaseBindingEnvVars stamps restartedAt, so an unconditional write would restart the
// pod on every poll.
func TestGetAgentDeployments_SkipsWriteWhenSystemEnvVarsAlreadyPresent(t *testing.T) {
	spy := &spyConfigService{
		systemEnvVars: []client.EnvVar{{Key: "OPENAI_URL", Value: "http://gateway.cluster.local/openai"}},
		systemEnvKeys: map[string]bool{"OPENAI_URL": true},
	}
	s, _, writes := deploymentsReconcileMocks([]models.EnvVars{{Key: "OPENAI_URL", Value: "http://gateway.cluster.local/openai"}}, spy)

	_, err := s.GetAgentDeployments(context.Background(), "acme", "default", "it-help-5")

	require.NoError(t, err)
	assert.Zero(t, *writes, "nothing is missing, so the binding must not be rewritten")
}
