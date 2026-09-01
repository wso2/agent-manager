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
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// From OpenChoreo 1.2.0 the cell namespace for (project, environment) is owned
// by a ProjectReleaseBinding. A project with no binding gets no namespace, and
// every deployment into that environment fails to apply with
// `namespaces "dp-..." not found`. These tests pin the three places that have
// to create one: project creation, deploy, and promote.

func threeEnvPipeline() *models.DeploymentPipelineResponse {
	return &models.DeploymentPipelineResponse{
		Name: "default",
		PromotionPaths: []models.PromotionPath{
			{
				SourceEnvironmentRef:  "dev",
				TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}},
			},
			{
				SourceEnvironmentRef:  "staging",
				TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "prod"}},
			},
		},
	}
}

func TestCreateProject_EnsuresBindingForEveryPipelineEnvironment(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		CreateProjectFunc: func(context.Context, string, client.CreateProjectRequest) error {
			return nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return threeEnvPipeline(), nil
		},
		EnsureProjectReleaseBindingFunc: func(context.Context, string, string, string) error {
			return nil
		},
	}
	mgr := NewInfraResourceManager(ocClient, discardLogger())

	project, err := mgr.CreateProject(context.Background(), "acme", spec.CreateProjectRequest{
		Name:               "my-project",
		DisplayName:        "My Project",
		DeploymentPipeline: "default",
	})

	require.NoError(t, err)
	require.NotNil(t, project)

	calls := ocClient.EnsureProjectReleaseBindingCalls()
	require.Len(t, calls, 3, "one binding per environment reachable through the pipeline")
	envs := make([]string, 0, len(calls))
	for _, call := range calls {
		assert.Equal(t, "acme", call.OuID)
		assert.Equal(t, "my-project", call.ProjectName)
		envs = append(envs, call.EnvironmentName)
	}
	// dev and staging each appear once even though staging is both a target of
	// the first path and the source of the second.
	assert.Equal(t, []string{"dev", "staging", "prod"}, envs)
}

func TestCreateProject_BindingFailureDoesNotFailProjectCreation(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		CreateProjectFunc: func(context.Context, string, client.CreateProjectRequest) error {
			return nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return threeEnvPipeline(), nil
		},
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, envName string) error {
			if envName == "staging" {
				return errors.New("openchoreo unavailable")
			}
			return nil
		},
	}
	mgr := NewInfraResourceManager(ocClient, discardLogger())

	project, err := mgr.CreateProject(context.Background(), "acme", spec.CreateProjectRequest{
		Name:               "my-project",
		DeploymentPipeline: "default",
	})

	// The project exists in OpenChoreo by this point, and deploy/promote ensure
	// the binding for the environment they target, so a binding failure must not
	// turn a created project into a failed request.
	require.NoError(t, err)
	require.NotNil(t, project)
	assert.Len(t, ocClient.EnsureProjectReleaseBindingCalls(), 3,
		"a failure for one environment must not stop the remaining environments")
}

func TestCreateProject_PipelineLookupFailureSkipsBindings(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		CreateProjectFunc: func(context.Context, string, client.CreateProjectRequest) error {
			return nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return nil, errors.New("pipeline lookup failed")
		},
		// Deliberately nil: a pipeline we could not read must not produce
		// binding calls with a guessed environment name.
	}
	mgr := NewInfraResourceManager(ocClient, discardLogger())

	project, err := mgr.CreateProject(context.Background(), "acme", spec.CreateProjectRequest{
		Name:               "my-project",
		DeploymentPipeline: "default",
	})

	require.NoError(t, err)
	require.NotNil(t, project)
	assert.Empty(t, ocClient.EnsureProjectReleaseBindingCalls())
}

func TestPipelineEnvironments(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *models.DeploymentPipelineResponse
		want     []string
	}{
		{"nil pipeline", nil, nil},
		{"no promotion paths", &models.DeploymentPipelineResponse{}, []string{}},
		{"dedupes and preserves order", threeEnvPipeline(), []string{"dev", "staging", "prod"}},
		{
			"single environment with no targets",
			&models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "default"},
			}},
			[]string{"default"},
		},
		{
			"skips empty names",
			&models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: ""}, {Name: "dev"}}},
			}},
			[]string{"dev"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, pipelineEnvironments(tc.pipeline))
		})
	}
}

// recordingProvisioningStub records whether the promote path reached the first
// write to the target environment.
type recordingProvisioningStub struct {
	*stubAgentThunderProvisioning
	provisionCalled *bool
}

func (s *recordingProvisioningStub) ProvisionForEnvironmentIfMissing(_ context.Context, _, _, _, _ string, _ models.AgentProvisioningType, _ string) (bool, error) {
	*s.provisionCalled = true
	return false, nil
}

func TestPromoteAgent_BindingFailureAbortsBeforeAnyTargetWrite(t *testing.T) {
	setRBACEnabledForTier(t, false)
	boom := errors.New("openchoreo unavailable")
	promoteCalled := false
	provisionCalled := false
	configUpserted := false

	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		// The deploy/promote pre-flight reads the component's reconcile conditions;
		// an unblocked component keeps this test on the path it actually covers.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: "agent-chat"},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		GetEnvironmentFunc:         nonProductionEnvStub(),
		IsDeploymentInProgressFunc: func(_ context.Context, _, _, _ string) (bool, error) { return false, nil },
		EnsureProjectReleaseBindingFunc: func(context.Context, string, string, string) error {
			return boom
		},
		PromoteComponentFunc: func(_ context.Context, _, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
			promoteCalled = true
			return nil
		},
	}
	// Nil funcs on purpose: reaching the source/target system-managed key reads
	// would mean the binding check ran too late to be a real guard.
	agentConfigSvc := &stubAgentConfigurationServiceForPromote{}
	provisioning := &recordingProvisioningStub{
		stubAgentThunderProvisioning: &stubAgentThunderProvisioning{},
		provisionCalled:              &provisionCalled,
	}
	agentConfigRepo := &repomocks.AgentConfigRepositoryMock{
		UpsertFunc: func(context.Context, *models.AgentConfig) error {
			configUpserted = true
			return nil
		},
	}
	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: agentConfigSvc,
		agentThunderProvisioning:  provisioning,
		agentConfigRepo:           agentConfigRepo,
		logger:                    discardLogger(),
	}

	err := s.PromoteAgent(auditableCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.ErrorIs(t, err, boom)
	assert.False(t, promoteCalled, "the promote must not run without a namespace to release into")
	assert.False(t, provisionCalled,
		"a promotion that cannot get a namespace must not leave an AgentID binding behind in the target environment")
	assert.False(t, configUpserted,
		"a promotion that cannot get a namespace must not persist agent config for the target environment")
}

func TestDeployAgent_BindingFailureAbortsDeploy(t *testing.T) {
	setRBACEnabledForTier(t, false)
	boom := errors.New("openchoreo unavailable")
	deployCalled := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		// The deploy/promote pre-flight reads the component's reconcile conditions;
		// an unblocked component keeps this test on the path it actually covers.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return threeEnvPipeline(), nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		EnsureProjectReleaseBindingFunc: func(context.Context, string, string, string) error {
			return boom
		},
		DeployFunc: func(context.Context, string, string, string, client.DeployRequest) error {
			deployCalled = true
			return nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	_, err := s.DeployAgent(auditableCtx(t), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{
		ImageId: "registry.example.com/my-agent:v1",
	})

	require.ErrorIs(t, err, boom)
	assert.False(t, deployCalled,
		"without a cell namespace the release binding cannot apply — deploying anyway only fails later and less legibly")

	calls := ocClient.EnsureProjectReleaseBindingCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "dev", calls[0].EnvironmentName, "the deploy target is the pipeline's lowest environment")
}
