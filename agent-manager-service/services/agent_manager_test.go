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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// stubAgentThunderProvisioning implements AgentThunderProvisioningService by
// embedding the (nil) interface and overriding only RegenerateFunc — any
// other method call panics on the nil embed, which is fine since tests using
// this stub never call them.
type stubAgentThunderProvisioning struct {
	AgentThunderProvisioningService
	RegenerateFunc       func(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error)
	RevokeFunc           func(ctx context.Context, orgName, projectName, agentName, envName string) (string, error)
	GetBindingStateFunc  func(ctx context.Context, orgName, projectName, agentName, envName string) (*AgentThunderBindingState, error)
	GetAgentRolesFunc    func(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error)
	GetAgentGroupsFunc   func(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error)
	GetIdentityViewsFunc func(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error)
}

func (s *stubAgentThunderProvisioning) GetBindingState(ctx context.Context, orgName, projectName, agentName, envName string) (*AgentThunderBindingState, error) {
	return s.GetBindingStateFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) RegenerateSecret(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error) {
	return s.RegenerateFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) RevokeSecret(ctx context.Context, orgName, projectName, agentName, envName string) (string, error) {
	return s.RevokeFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetAgentRoles(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error) {
	return s.GetAgentRolesFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetAgentGroups(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error) {
	return s.GetAgentGroupsFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetIdentityViews(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error) {
	return s.GetIdentityViewsFunc(ctx, ouID, projectName, agentName)
}

// HealSecretRef is a no-op override: no test using this stub exercises the
// reconciler's startup heal pass, but leaving it unimplemented would panic on
// the nil embedded interface if that ever changes.
func (s *stubAgentThunderProvisioning) HealSecretRef(ctx context.Context, binding models.AgentThunderClient) error {
	return nil
}

func TestValidateInstrumentationVersion_UsesCatalog(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validateInstrumentationVersion("0.2.1"); err != nil {
		t.Errorf("0.2.1 should be valid: %v", err)
	}
	err := s.validateInstrumentationVersion("9.9.9")
	if err == nil {
		t.Fatal("9.9.9 should be invalid")
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("error %q should mention 9.9.9", err)
	}
}

func TestValidatePythonInstrumentationPair(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
			{Version: "0.4.0", PythonVersions: []string{"3.12", "3.13"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validatePythonInstrumentationPair("3.11", "0.2.1"); err != nil {
		t.Errorf("3.11 + 0.2.1 should be valid: %v", err)
	}
	err := s.validatePythonInstrumentationPair("3.13", "0.2.1")
	if err == nil {
		t.Fatal("3.13 + 0.2.1 should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should mention both python and instrumentation versions", err)
	}
}

func TestValidateEffectivePair_FallsBackToDefault(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// nil requested version means "use platform default", which is 0.2.1.
	if err := s.validateEffectivePythonInstrumentationPair("3.11", nil); err != nil {
		t.Errorf("3.11 + default(0.2.1) should be valid: %v", err)
	}
	err := s.validateEffectivePythonInstrumentationPair("3.13", nil)
	if err == nil {
		t.Fatal("3.13 + default(0.2.1) should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should name the resolved default version, not just nil", err)
	}
}

func TestBuildpackPythonVersion_Normalises(t *testing.T) {
	mk := func(lang string, version *string) *spec.Build {
		b := spec.BuildpackBuildAsBuild(&spec.BuildpackBuild{
			Buildpack: spec.BuildpackConfig{
				Language:        lang,
				LanguageVersion: version,
			},
		})
		return &b
	}
	strPtr := func(s string) *string { return &s }

	cases := []struct {
		name string
		in   *spec.Build
		want string
	}{
		{"bare minor", mk("python", strPtr("3.11")), "3.11"},
		{"with patch", mk("python", strPtr("3.11.4")), "3.11"},
		{"with x", mk("python", strPtr("3.11.x")), "3.11"},
		{"leading whitespace", mk("python", strPtr("  3.11  ")), "3.11"},
		{"whitespace only", mk("python", strPtr("   ")), ""},
		{"empty", mk("python", strPtr("")), ""},
		{"capital P language", mk("Python", strPtr("3.11")), ""},
		{"non python language", mk("nodejs", strPtr("20")), ""},
		{"single component", mk("python", strPtr("3")), ""},
		{"nil version", mk("python", nil), ""},
		{"nil build", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildpackPythonVersion(tc.in)
			if got != tc.want {
				t.Errorf("buildpackPythonVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateEffectivePair_NoPythonIsNoOp(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// Empty python means the agent isn't a python-buildpack build.
	if err := s.validateEffectivePythonInstrumentationPair("", nil); err != nil {
		t.Errorf("empty python should be a no-op: %v", err)
	}
}

func TestNormalizePythonMinor(t *testing.T) {
	cases := map[string]string{
		"3.11":     "3.11",
		"3.11.4":   "3.11",
		"3.11.x":   "3.11",
		"  3.11  ": "3.11",
		"3":        "",
		"":         "",
		"   ":      "",
	}
	for in, want := range cases {
		if got := normalizePythonMinor(in); got != want {
			t.Errorf("normalizePythonMinor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveInstrumentationImageOverride(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
			{Version: "0.4.0", PythonVersions: []string{"3.12", "3.13"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	strPtr := func(s string) *string { return &s }
	s := &agentManagerService{logger: discardLogger()}

	t.Run("non-python echoes existing pin, no image", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(false, "3.11", strPtr("0.4.0"), strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want existing 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty for non-python", image)
		}
	})

	t.Run("request override validates and wins", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("0.2.1"), strPtr("0.4.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want requested 0.2.1", version)
		}
		if !strings.HasSuffix(image, "0.2.1-python3.11") {
			t.Errorf("image = %q, want suffix 0.2.1-python3.11", image)
		}
	})

	t.Run("unknown request version is rejected", func(t *testing.T) {
		_, _, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("9.9.9"), nil)
		if !errors.Is(err, utils.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("python-incompatible request version is rejected", func(t *testing.T) {
		// 0.4.0 supports 3.12/3.13, not 3.11.
		_, _, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("0.4.0"), nil)
		if !errors.Is(err, utils.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("no request preserves existing pin as image", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if !strings.HasSuffix(image, "0.2.1-python3.11") {
			t.Errorf("image = %q, want suffix 0.2.1-python3.11", image)
		}
	})

	t.Run("no request and no existing pin yields no override", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version != nil {
			t.Errorf("version = %v, want nil", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty", image)
		}
	})

	t.Run("existing pin incompatible with current python keeps version but skips image", func(t *testing.T) {
		// Pin 0.2.1 supports 3.10/3.11; the agent's Python is now 3.13. Building
		// the image would yield a nonexistent 0.2.1-python3.13 tag, so the
		// override is skipped (empty image) while the DB version is preserved.
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.13", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty (incompatible pin, component default kept)", image)
		}
	})

	t.Run("existing pin with unparseable python keeps component default", func(t *testing.T) {
		// No request override + bad language version: don't fail the redeploy,
		// just skip the per-env image override.
		version, image, err := s.resolveInstrumentationImageOverride(true, "notaversion", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty (component default kept)", image)
		}
	})
}

func TestRegenerateAgentIdentitySecret_ExternalAgent_ReturnsSecret(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeExternal, "client-abc", "fresh-secret-xyz", nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, "dev", resp.EnvironmentName)
	assert.Equal(t, models.AgentProvisioningTypeExternal, resp.ProvisioningType)
	assert.Equal(t, "client-abc", resp.ClientID)
	assert.Equal(t, "fresh-secret-xyz", resp.ClientSecret,
		"an External agent must get its freshly regenerated secret back")
	assert.Equal(t, models.AgentRegenerateSecretStatus, resp.Status)
	assert.Empty(t, resp.WorkloadRefreshWarning, "external agents have no workload to refresh")
}

func TestRegenerateAgentIdentitySecret_InternalAgent_AlsoReturnsSecret(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeInternal, "client-def", "fresh-secret-internal", nil
		},
	}
	refreshed := 0
	injector := &agentIdentityInjectorStub{
		RefreshAfterRotationFunc: func(_ context.Context, orgName, _, _, envName string) error {
			refreshed++
			assert.Equal(t, "acme", orgName)
			assert.Equal(t, "dev", envName)
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, logger: discardLogger()}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeInternal, resp.ProvisioningType)
	assert.Equal(t, "fresh-secret-internal", resp.ClientSecret,
		"an Internal agent must ALSO get its freshly regenerated secret back — regenerate is not the "+
			"one-time-claim endpoint, withholding it here would just force a second call to see it")
	assert.Equal(t, models.AgentRegenerateSecretStatus, resp.Status)
	assert.Equal(t, 1, refreshed, "internal rotation must refresh the workload's injected credential")
	assert.Empty(t, resp.WorkloadRefreshWarning, "a successful refresh must not surface a warning")
}

// TestRegenerateAgentIdentitySecret_InternalAgent_RefreshFailureDoesNotFailRotation
// guards two things at once: a failed workload refresh must not fail the
// request (the Thunder rotation already succeeded), AND it must not be a
// silent failure — the pod keeps serving the now-invalidated old secret until
// a later deploy/promote/rotation, so the caller needs a way to know that
// happened instead of just a log line only an operator would see.
func TestRegenerateAgentIdentitySecret_InternalAgent_RefreshFailureDoesNotFailRotation(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeInternal, "client-def", "fresh-secret-internal", nil
		},
	}
	injector := &agentIdentityInjectorStub{
		RefreshAfterRotationFunc: func(_ context.Context, _, _, _, _ string) error {
			return errors.New("workload refresh failed")
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, logger: discardLogger()}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err, "the rotation already happened in Thunder — a failed workload refresh must not fail the request")
	assert.Equal(t, "fresh-secret-internal", resp.ClientSecret)
	assert.NotEmpty(t, resp.WorkloadRefreshWarning,
		"a failed workload refresh must be surfaced to the caller, not just logged")
}

func TestRevokeAgentIdentitySecret_LowestEnv_RemovesWorkloadLevelVars(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(_ context.Context, _, _, _, _ string) (string, error) { return "client-abc", nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
	}
	var gotIncludeWorkload *bool
	injector := &agentIdentityInjectorStub{
		RemoveForEnvironmentFunc: func(_ context.Context, _, _, _, envName string, includeWorkloadLevel bool) error {
			assert.Equal(t, "dev", envName)
			gotIncludeWorkload = &includeWorkloadLevel
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, ocClient: ocClient, logger: discardLogger()}

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, "client-abc", resp.ClientID)
	require.NotNil(t, gotIncludeWorkload)
	assert.True(t, *gotIncludeWorkload, "revoking the LOWEST environment's credential must also strip the shared workload-level vars")
	assert.Empty(t, resp.WorkloadRefreshWarning, "a clean revoke with a resolvable pipeline must not carry a warning")
}

func TestRevokeAgentIdentitySecret_NonLowestEnv_KeepsWorkloadLevelVars(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(_ context.Context, _, _, _, _ string) (string, error) { return "client-abc", nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
	}
	var gotIncludeWorkload *bool
	injector := &agentIdentityInjectorStub{
		RemoveForEnvironmentFunc: func(_ context.Context, _, _, _, envName string, includeWorkloadLevel bool) error {
			assert.Equal(t, "staging", envName)
			gotIncludeWorkload = &includeWorkloadLevel
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, ocClient: ocClient, logger: discardLogger()}

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	require.NotNil(t, gotIncludeWorkload)
	assert.False(t, *gotIncludeWorkload, "revoking a NON-lowest environment must never strip the lowest environment's shared workload-level vars")
	assert.Empty(t, resp.WorkloadRefreshWarning, "a clean revoke with a resolvable pipeline must not carry a warning")
}

func TestRevokeAgentIdentitySecret_PipelineLookupFails_StillRevokesConservatively(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(_ context.Context, _, _, _, _ string) (string, error) { return "client-abc", nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return nil, errors.New("pipeline unavailable")
		},
	}
	var gotIncludeWorkload *bool
	injector := &agentIdentityInjectorStub{
		RemoveForEnvironmentFunc: func(_ context.Context, _, _, _, _ string, includeWorkloadLevel bool) error {
			gotIncludeWorkload = &includeWorkloadLevel
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, ocClient: ocClient, logger: discardLogger()}

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err, "revoke already succeeded in Thunder — cleanup problems must not fail the request")
	assert.Equal(t, "client-abc", resp.ClientID)
	require.NotNil(t, gotIncludeWorkload)
	assert.False(t, *gotIncludeWorkload, "with an unknown pipeline, be conservative and leave workload-level vars alone")
	assert.NotEmpty(t, resp.WorkloadRefreshWarning,
		"an unresolvable pipeline means it's genuinely unknown whether this environment needed workload-level "+
			"cleanup too — that must be surfaced to the caller, not reported as a plain, silent success")
}

// TestDeployAgent_IdentityInjectionError_AbortsDeploy guards that a failure
// building the AgentID env vars stops the deploy entirely rather than
// deploying without credentials: Deploy() replaces every workload env var, so
// a deploy that proceeded here would permanently drop the agent's
// credentials until some later operation happened to re-inject them.
func TestDeployAgent_IdentityInjectionError_AbortsDeploy(t *testing.T) {
	boom := errors.New("secret backend unavailable")
	deployCalled := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return nil, nil
		},
		// Not blocked, so the deploy reaches the identity injection this test is about.
		GetComponentReconcileBlockFunc: func(context.Context, string, string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // nil block is the "not blocked" signal this API defines
		},
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error { return nil },
		DeployFunc: func(context.Context, string, string, string, client.DeployRequest) error {
			deployCalled = true
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
			return nil, boom
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentIdentityInjection: injector, logger: discardLogger()}

	_, err := s.DeployAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{ImageId: "registry.example.com/my-agent:v1"})

	require.Error(t, err, "a failure building AgentID env vars must abort the deploy, not proceed without credentials")
	assert.False(t, deployCalled, "the OpenChoreo Deploy call must never happen once identity env vars failed to build")
}

// TestUpdateAgentConfigurations_IdentityInjectionError_AbortsUpdate guards
// the same contract as TestDeployAgent_IdentityInjectionError_AbortsDeploy for
// the other call site that replaces an environment's entire env var set: a
// failure building the AgentID env vars must abort before the override
// rewrite runs, or the rewrite would silently drop the agent's credentials.
func TestUpdateAgentConfigurations_IdentityInjectionError_AbortsUpdate(t *testing.T) {
	boom := errors.New("secret backend unavailable")
	overridesReplaced := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name}, nil
		},
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return nil, nil
		},
		EnsureReleaseAndBindingFunc: func(context.Context, string, string, string, string, []client.EnvVar, []client.FileVar) error {
			overridesReplaced = true
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
			return nil, boom
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentIdentityInjection: injector, logger: discardLogger()}

	err := s.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.UpdateAgentConfigurationsRequest{EnvironmentName: "dev"})

	require.Error(t, err, "a failure building AgentID env vars must abort the update, not proceed without credentials")
	assert.False(t, overridesReplaced, "the env var override rewrite must never happen once identity env vars failed to build")
}

// TestUpdateAgentConfigurations_RejectsUnownedSecretRef guards against the
// secretRef ownership bypass: a caller who can read one agent's config (and
// so learns its real secretRef, e.g. "victim-agent-default-secrets") must not
// be able to wire that same secretRef into a DIFFERENT agent's config as a
// claimed "system-managed" reference for a key that agent doesn't yet own.
// Before the fix, processEnvVars trusted any client-supplied secretRef for a
// key outside the target agent's own secret without checking it against that
// agent's actual server-side configuration — so the attacker's env var would
// resolve, at the workload, straight to the victim's secret value.
func TestUpdateAgentConfigurations_RejectsUnownedSecretRef(t *testing.T) {
	overridesReplaced := false
	const attackerAgent = "scout-agent"
	const victimSecretRef = "victim-agent-default-secrets"

	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		// scout-agent's own configuration has never contained OPENAI_API_KEY under
		// any secretRef — there is nothing here for the attacker's claim to match.
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return nil, nil
		},
		EnsureReleaseAndBindingFunc: func(context.Context, string, string, string, string, []client.EnvVar, []client.FileVar) error {
			overridesReplaced = true
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
			return nil, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentIdentityInjection: injector, logger: discardLogger()}

	attackerEnv := spec.EnvironmentVariable{Key: "OPENAI_API_KEY"}
	attackerEnv.SetIsSensitive(true)
	attackerEnv.SetValue("")
	attackerEnv.SetSecretRef(victimSecretRef) // recovered from victim-agent's own config-read response

	err := s.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", attackerAgent,
		&spec.UpdateAgentConfigurationsRequest{
			EnvironmentName: "dev",
			Env:             []spec.EnvironmentVariable{attackerEnv},
		})

	require.Error(t, err, "wiring another agent's secretRef into this agent's config must be rejected")
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "the rejection must be classified as an invalid-input error, not a generic failure")
	assert.False(t, overridesReplaced, "the victim's secretRef must never reach the workload override rewrite for an unrelated agent")
}

// stubAgentConfigurationServiceForPromote implements AgentConfigurationService
// by embedding the (nil) interface and overriding only the methods PromoteAgent
// actually calls — any other method call panics on the nil embed, which is fine
// since these tests never exercise them.
type stubAgentConfigurationServiceForPromote struct {
	AgentConfigurationService
	SystemKeysFunc     func(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]bool, error)
	SystemVarsFunc     func(ctx context.Context, agentID, ouID, projectName, environmentName string) ([]client.EnvVar, error)
	SystemConfigsFunc  func(ctx context.Context, agentID, ouID, projectName, environmentName string) ([]SystemManagedConfigRef, error)
	UnresolvedMCPsFunc func(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]struct{}, error)
}

// Defaults to "no configuration to describe", so tests that predate the block's
// per-configuration wording keep the message they were written against.
func (s *stubAgentConfigurationServiceForPromote) ListSystemManagedConfigs(ctx context.Context, agentID, ouID, projectName, environmentName string) ([]SystemManagedConfigRef, error) {
	if s.SystemConfigsFunc == nil {
		return []SystemManagedConfigRef{}, nil
	}
	return s.SystemConfigsFunc(ctx, agentID, ouID, projectName, environmentName)
}

func (s *stubAgentConfigurationServiceForPromote) ListSystemManagedEnvVarKeys(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]bool, error) {
	return s.SystemKeysFunc(ctx, agentID, ouID, projectName, environmentName)
}

func (s *stubAgentConfigurationServiceForPromote) BuildSystemManagedEnvVarsFromConfig(ctx context.Context, agentID, ouID, projectName, environmentName string) ([]client.EnvVar, error) {
	return s.SystemVarsFunc(ctx, agentID, ouID, projectName, environmentName)
}

// Defaults to "every MCP connection resolves", so tests that predate the promotion
// binding check are unaffected by it.
func (s *stubAgentConfigurationServiceForPromote) ListUnresolvedMCPBindings(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]struct{}, error) {
	if s.UnresolvedMCPsFunc == nil {
		return map[string]struct{}{}, nil
	}
	return s.UnresolvedMCPsFunc(ctx, agentID, ouID, projectName, environmentName)
}

// shrinkPromotionIdentityPollForTest overrides the poll interval/budget
// PromoteAgent uses when a target environment's identity isn't ready yet
// (see pollForTargetIdentityReady), so tests exercising the hard-block path
// don't have to wait out the real (multi-second) production budget.
func shrinkPromotionIdentityPollForTest(t *testing.T) {
	t.Helper()
	origInterval, origBudget := promotionIdentityPollInterval, promotionIdentityPollBudget
	promotionIdentityPollInterval = time.Millisecond
	promotionIdentityPollBudget = 5 * time.Millisecond
	t.Cleanup(func() {
		promotionIdentityPollInterval, promotionIdentityPollBudget = origInterval, origBudget
	})
}

// promoteAgentTestFixture builds the minimal set of mocks PromoteAgent needs
// for a non-API-type internal agent (skips the large isAPIAgent branch
// entirely), for a dev -> staging promotion pipeline.
func promoteAgentTestFixture(t *testing.T, tgtIdentityEnvVars []client.EnvVar, tgtIdentityErr error) (*agentManagerService, *bool) {
	t.Helper()
	shrinkPromotionIdentityPollForTest(t)
	promoteCalled := false

	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		GetOrganizationFunc: func(_ context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: orgName}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: "agent-chat"}, // deliberately not agent-api
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		// Default: the component reconciles normally, so the promote pre-flight lets
		// it through. Tests covering the pre-flight override this directly.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		GetEnvironmentFunc:              nonProductionEnvStub(),
		IsDeploymentInProgressFunc:      func(_ context.Context, _, _, _ string) (bool, error) { return false, nil },
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error { return nil },
		PromoteComponentFunc: func(_ context.Context, _, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
			promoteCalled = true
			return nil
		},
	}

	agentConfigSvc := &stubAgentConfigurationServiceForPromote{
		SystemKeysFunc: func(_ context.Context, _, _, _, _ string) (map[string]bool, error) { return map[string]bool{}, nil },
		SystemVarsFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) { return nil, nil },
	}

	identityInjector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, envName string) ([]client.EnvVar, error) {
			if envName == "staging" {
				if tgtIdentityErr != nil {
					return nil, tgtIdentityErr
				}
				return tgtIdentityEnvVars, nil
			}
			if envName == "dev" {
				// dev (the pipeline's lowest environment) already has a real,
				// deployed AgentID credential in every fixture built from this
				// helper — PromoteAgent's leak-safety check reads this to tell
				// "target genuinely not ready yet, must block" apart from
				// "AgentID was never used at all, safe to let through" when
				// tgtIdentityEnvVars comes back empty (see the cross-environment
				// leak fix in PromoteAgent).
				return []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"}}, nil
			}
			t.Fatalf("agentIdentityInjection.EnvVarsForEnvironment called for unexpected environment %q", envName)
			return nil, nil
		},
	}

	provisioningStub := &stubAgentThunderProvisioning{
		// Default: "still provisioning" — matches the not-ready fixtures'
		// expectation that the hard block's message says so. Tests asserting
		// a different state (revoked, failed) override this directly.
		GetBindingStateFunc: func(context.Context, string, string, string, string) (*AgentThunderBindingState, error) {
			return &AgentThunderBindingState{Status: models.AgentThunderStatusPending}, nil
		},
	}
	// ProvisionForEnvironmentIfMissing is called unconditionally before the
	// readiness check — must not panic.
	provisioningStubWithProvision := &provisionForEnvIfMissingStub{stubAgentThunderProvisioning: provisioningStub}

	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: agentConfigSvc,
		agentIdentityInjection:    identityInjector,
		agentThunderProvisioning:  provisioningStubWithProvision,
		logger:                    discardLogger(),
	}
	return s, &promoteCalled
}

// provisionForEnvIfMissingStub adds a no-op ProvisionForEnvironmentIfMissing
// on top of stubAgentThunderProvisioning, since PromoteAgent now calls it
// unconditionally before checking target-identity readiness.
type provisionForEnvIfMissingStub struct {
	*stubAgentThunderProvisioning
}

func (s *provisionForEnvIfMissingStub) ProvisionForEnvironmentIfMissing(_ context.Context, _, _, _, _ string, _ models.AgentProvisioningType, _ string) (bool, error) {
	return false, nil
}

// promoteConfigStub reaches the agent configuration stub a promote fixture was built
// with, so each stubbing helper is the closure it installs and nothing else.
func promoteConfigStub(t *testing.T, s *agentManagerService) *stubAgentConfigurationServiceForPromote {
	t.Helper()
	agentConfigSvc, ok := s.agentConfigurationService.(*stubAgentConfigurationServiceForPromote)
	require.True(t, ok)
	return agentConfigSvc
}

// stubUnresolvedMCPs makes the named connections unresolved in blockedEnv only,
// which is what makes a promotion into that environment break them.
func stubUnresolvedMCPs(t *testing.T, s *agentManagerService, blockedEnv string, names ...string) {
	t.Helper()
	promoteConfigStub(t, s).UnresolvedMCPsFunc = func(_ context.Context, _, _, _, envName string) (map[string]struct{}, error) {
		unresolved := map[string]struct{}{}
		if envName == blockedEnv {
			for _, name := range names {
				unresolved[name] = struct{}{}
			}
		}
		return unresolved, nil
	}
}

// An MCP connection that resolves in the source but not the target would promote with its
// URL and API key injected as empty strings — the agent starts, then fails on every tool
// call. The promotion must be refused instead, before the component is promoted.
func TestPromoteAgent_BlocksWhenMCPConnectionUnresolvableInTarget(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubUnresolvedMCPs(t, s, "staging", "booking")

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "booking", "the message must name the connection that blocks the promotion")
	assert.Contains(t, ve.Reason, "deploy")
	assert.False(t, *promoteCalled,
		"promotion must be refused before PromoteComponent — otherwise the agent is already running with an empty MCP URL by the time this error is returned")
}

// A connection unresolved in BOTH environments is simply not offered anywhere; this
// promotion does not break it, so it must not be blocked.
func TestPromoteAgent_AllowsMCPConnectionUnresolvableInBothEnvironments(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	agentConfigSvc, ok := s.agentConfigurationService.(*stubAgentConfigurationServiceForPromote)
	require.True(t, ok)
	agentConfigSvc.UnresolvedMCPsFunc = func(_ context.Context, _, _, _, _ string) (map[string]struct{}, error) {
		return map[string]struct{}{"booking": {}}, nil
	}

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.NoError(t, err)
	assert.True(t, *promoteCalled)
}

// Not knowing whether the target's MCP connections resolve is not the same as knowing they
// do. A lookup failure must abort the promotion rather than wave it through — otherwise a
// transient database blip is all it takes to ship an agent with an empty MCP URL.
func TestPromoteAgent_BlocksWhenMCPBindingLookupFails(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	agentConfigSvc, ok := s.agentConfigurationService.(*stubAgentConfigurationServiceForPromote)
	require.True(t, ok)
	agentConfigSvc.UnresolvedMCPsFunc = func(_ context.Context, _, _, _, _ string) (map[string]struct{}, error) {
		return nil, errors.New("database unavailable")
	}

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	assert.False(t, *promoteCalled)
}

// Several broken connections are listed in a stable order, so the same rejection produces
// the same message on every run instead of reshuffling with Go's map iteration order.
func TestPromoteAgent_ListsBrokenMCPConnectionsInStableOrder(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubUnresolvedMCPs(t, s, "staging", "payments", "booking")

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "booking, payments")
}

// PromoteComponent's writes land on the Component whether or not OpenChoreo can reconcile
// it, but a blocked component never gets a new release cut for the target environment. The
// promote must be refused up front instead of returning 202 for a no-op.
func TestPromoteAgent_BlocksWhenComponentNotReconcilable(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.GetComponentReconcileBlockFunc = func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
		return &client.ComponentReconcileBlock{
			Reason:  "WorkloadInvalid",
			Message: "workload references a missing secret",
		}, nil
	}

	err := s.PromoteAgent(auditableCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrComponentNotReconcilable)
	assert.Contains(t, err.Error(), "WorkloadInvalid")
	assert.Contains(t, err.Error(), "workload references a missing secret")
	assert.False(t, *promoteCalled,
		"promotion must be refused before PromoteComponent — otherwise the caller gets a success for a promote the controller silently discarded")
}

// The pre-flight is an extra safety net, not a dependency: if the conditions cannot be read,
// a promote that would otherwise succeed must still go through rather than being held hostage
// to a transient OpenChoreo read failure.
func TestPromoteAgent_ReconcileLookupFails_PromotesAnyway(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.GetComponentReconcileBlockFunc = func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
		return nil, errors.New("openchoreo unavailable")
	}

	err := s.PromoteAgent(auditableCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.NoError(t, err)
	assert.True(t, *promoteCalled)
}

// The block is checked before the pipeline is fetched, so a blocked component is rejected
// for the reason that actually applies rather than failing later on unrelated validation.
func TestPromoteAgent_ReconcileBlockCheckedBeforePipelineLookup(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.GetComponentReconcileBlockFunc = func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
		return &client.ComponentReconcileBlock{Reason: "WorkloadInvalid", Message: "bad workload"}, nil
	}
	pipelineCalled := false
	ocMock.GetProjectDeploymentPipelineFunc = func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
		pipelineCalled = true
		return nil, errors.New("pipeline lookup must not be reached")
	}

	err := s.PromoteAgent(auditableCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrComponentNotReconcilable)
	assert.False(t, pipelineCalled)
}

func TestPromoteAgent_BlocksWhenTargetIdentityNotReady(t *testing.T) {
	// Empty, no-error result — exactly what EnvVarsForEnvironment returns
	// when the target's AgentID binding hasn't finished provisioning yet.
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)

	err := s.PromoteAgent(auditableCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "retry once provisioning completes")
	assert.False(t, *promoteCalled,
		"promotion must be blocked BEFORE calling PromoteComponent — otherwise the pod is already promoted with leaked credentials by the time this error is returned")
}

// The block reports what was actually checked — that the configuration has no
// MCP server bound in the target — rather than asserting the server has no
// endpoint there. Only the absent mapping row is observed; the server may well
// be deployed to the target, with just the binding missing, and sending the
// user to look for a missing endpoint would send them after the wrong problem.
func TestPromoteAgent_BlockedMCPPromotion_ReportsTheMissingBindingNotAnUncheckedCause(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubUnresolvedMCPs(t, s, "staging", "booking")

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, `MCP configuration "booking" has no MCP server in "staging"`)
	assert.NotContains(t, ve.Message, "endpoint",
		"whether the MCP server has an endpoint in the target is never checked, so the block must not claim it")
}

// "Bind them to an endpoint" named no control the console or the CLI offers.
// Deploying the MCP server to the target is what actually clears the block:
// this configuration is already mapped in the source, so ReconcileMCPBindingsForProxy
// backfills the target mapping as soon as the server lands there.
func TestPromoteAgent_BlockedMCPPromotion_TellsUserToDeployTheMCPServer(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubUnresolvedMCPs(t, s, "staging", "booking")

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Equal(t, `deploy its MCP server to "staging", then promote`, ve.Reason)
}

// One broken configuration is the common case, so the block reads as a sentence
// about it instead of hedging with "configuration(s)" and "their".
func TestPromoteAgent_BlockedMCPPromotion_ReadsAsPluralOnlyWhenSeveralAreBroken(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubUnresolvedMCPs(t, s, "staging", "payments", "booking")

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, `MCP configurations booking, payments have no MCP server in "staging"`)
	assert.Equal(t, `deploy their MCP servers to "staging", then promote`, ve.Reason)
	assert.NotContains(t, renderedUIError(ve), "(s)", "the wording agrees with the count instead of hedging")
}

// The sibling block below shortens a long configuration name, and this one renders
// its names through the same helper, so it must shorten too. A 255-character name is
// legal, and pasting one in whole buries the sentence that says what to do about it.
func TestPromoteAgent_BlockedMCPPromotion_VeryLongConfigName_IsShortenedNotPastedWhole(t *testing.T) {
	longName := "hotel-booking" + strings.Repeat("x", 242)
	require.Len(t, longName, 255, "the fixture must use the longest name the API accepts")

	for _, tc := range []struct {
		name  string
		names []string
	}{
		{"single configuration", []string{longName}},
		{"several configurations", []string{longName, "payments"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
			stubUnresolvedMCPs(t, s, "staging", tc.names...)
			logs := captureLogs(s)

			err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
				SourceEnvironment: "dev",
				TargetEnvironment: "staging",
			})

			ve := requireBriefPromotionBlock(t, err)
			rendered := renderedUIError(ve)
			assert.NotContains(t, rendered, longName, "the whole name must not reach the UI")
			assert.Contains(t, rendered, "hotel-booking", "enough of the name must survive to identify the configuration")
			assert.Contains(t, logs(), longName, "the untruncated name must still reach the log")
		})
	}
}

func TestPromoteAgent_TargetIdentityReady_PromotesWithTargetOnlyCredentials(t *testing.T) {
	targetVars := []client.EnvVar{
		{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"},
	}
	s, _ := promoteAgentTestFixture(t, targetVars, nil)

	var capturedOverrides []client.EnvVar
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.PromoteComponentFunc = func(_ context.Context, _, _, _, _, _ string, envOverrides []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
		capturedOverrides = envOverrides
		return nil
	}

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.NoError(t, err)
	require.NotNil(t, capturedOverrides, "PromoteComponent must actually be called")

	found := false
	for _, ev := range capturedOverrides {
		if ev.Key == "AMP_AGENTID_CLIENT_ID" {
			found = true
			assert.Equal(t, "staging-client-id", ev.Value,
				"the target environment's own identity vars must be the ones actually sent to PromoteComponent")
		}
	}
	assert.True(t, found, "target environment's identity env vars must be present in the promoted overrides")
}

func TestPromoteAgent_IdentityBuildError_AbortsBeforePromoting(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, nil, errors.New("openchoreo unavailable"))

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	assert.False(t, *promoteCalled, "a real error building identity env vars must abort before promoting, not just log a warning")
}

// TestPromoteAgent_KickOffThenRetry_SucceedsOnceTargetIdentityCompletes covers
// promoting to an environment that was added to the pipeline AFTER the agent
// was created, so it has no AgentID binding yet. The pre-promote kick-off
// (ProvisionForEnvironmentIfMissing) starts provisioning, but that Thunder
// call is asynchronous, so the FIRST attempt still hard-blocks; a RETRY of the
// same promote call must succeed once that provisioning attempt finishes —
// proving the pre-promote kick-off alone is sufficient to unblock a
// new-environment promotion, with no dependency on any post-promote step.
func TestPromoteAgent_KickOffThenRetry_SucceedsOnceTargetIdentityCompletes(t *testing.T) {
	shrinkPromotionIdentityPollForTest(t)
	promoteCalled := false
	var capturedOverrides []client.EnvVar
	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetOrganizationFunc: func(_ context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: orgName}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: "agent-chat"},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		// Default: the component reconciles normally, so the promote pre-flight lets
		// it through. Tests covering the pre-flight override this directly.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		IsDeploymentInProgressFunc:      func(_ context.Context, _, _, _ string) (bool, error) { return false, nil },
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error { return nil },
		PromoteComponentFunc: func(_ context.Context, _, _, _, _, _ string, envOverrides []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
			promoteCalled = true
			capturedOverrides = envOverrides
			return nil
		},
	}
	agentConfigSvc := &stubAgentConfigurationServiceForPromote{
		SystemKeysFunc: func(_ context.Context, _, _, _, _ string) (map[string]bool, error) { return map[string]bool{}, nil },
		SystemVarsFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) { return nil, nil },
	}

	// targetReady flips to true once the (simulated) async provisioning
	// attempt kicked off by ProvisionForEnvironmentIfMissing finishes.
	targetReady := false
	identityInjector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, envName string) ([]client.EnvVar, error) {
			if envName == "dev" {
				// dev already has a real, deployed credential — AgentID is
				// actively used for this agent, so the leak-safety check on an
				// empty target result must fall through to the poll/hard-block
				// below rather than waving the promotion through.
				return []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"}}, nil
			}
			require.Equal(t, "staging", envName)
			if !targetReady {
				return nil, nil
			}
			return []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil
		},
	}
	provisioning := &provisionForEnvIfMissingStub{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{
		GetBindingStateFunc: func(context.Context, string, string, string, string) (*AgentThunderBindingState, error) {
			if !targetReady {
				return &AgentThunderBindingState{Status: models.AgentThunderStatusPending}, nil
			}
			return &AgentThunderBindingState{Status: models.AgentThunderStatusCompleted, HasSecret: true}, nil
		},
	}}

	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: agentConfigSvc,
		agentIdentityInjection:    identityInjector,
		agentThunderProvisioning:  provisioning,
		logger:                    discardLogger(),
	}

	req := &spec.PromoteAgentRequest{SourceEnvironment: "dev", TargetEnvironment: "staging"}

	// First attempt: target environment is brand new — kicks off provisioning
	// (ProvisionForEnvironmentIfMissing), but the identity isn't ready yet.
	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", req)
	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "still being provisioned")
	assert.False(t, promoteCalled, "must not promote while the target identity is still provisioning")

	// Simulate the async provisioning attempt completing in the background.
	targetReady = true

	// Retry: the same promote call now succeeds with the target's own creds.
	err = s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", req)
	require.NoError(t, err)
	assert.True(t, promoteCalled, "the retry must succeed once the target identity is ready")

	found := false
	for _, ev := range capturedOverrides {
		if ev.Key == "AMP_AGENTID_CLIENT_ID" {
			found = true
			assert.Equal(t, "staging-client-id", ev.Value)
		}
	}
	assert.True(t, found, "the promoted overrides must carry the target environment's identity once ready")
}

// TestPromoteAgent_PollSucceedsWithinBudget_PromotesOnFirstCall proves the
// bounded poll itself (not just the two-call retry pattern) — a target
// identity that becomes ready a couple of checks into the poll window must
// let the SAME PromoteAgent call succeed, without the caller needing to
// retry at all.
func TestPromoteAgent_PollSucceedsWithinBudget_PromotesOnFirstCall(t *testing.T) {
	shrinkPromotionIdentityPollForTest(t)
	promoteCalled := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetOrganizationFunc: func(_ context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: orgName}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: "agent-chat"},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		// Default: the component reconciles normally, so the promote pre-flight lets
		// it through. Tests covering the pre-flight override this directly.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		IsDeploymentInProgressFunc:      func(_ context.Context, _, _, _ string) (bool, error) { return false, nil },
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error { return nil },
		PromoteComponentFunc: func(_ context.Context, _, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
			promoteCalled = true
			return nil
		},
	}
	agentConfigSvc := &stubAgentConfigurationServiceForPromote{
		SystemKeysFunc: func(_ context.Context, _, _, _, _ string) (map[string]bool, error) { return map[string]bool{}, nil },
		SystemVarsFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) { return nil, nil },
	}

	// checks counts every EnvVarsForEnvironment call: the first (pre-poll) one
	// plus each poll iteration. Ready only from the 3rd check onward, so the
	// poll loop must actually iterate more than once to succeed.
	var checks int32
	identityInjector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) {
			if atomic.AddInt32(&checks, 1) < 3 {
				return nil, nil
			}
			return []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil
		},
	}
	provisioning := &provisionForEnvIfMissingStub{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{}}

	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: agentConfigSvc,
		agentIdentityInjection:    identityInjector,
		agentThunderProvisioning:  provisioning,
		logger:                    discardLogger(),
	}

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.NoError(t, err, "the poll must let this single call succeed once the identity becomes ready within budget")
	assert.True(t, promoteCalled)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&checks), int32(3), "the poll loop must have actually iterated, not just checked once")
}

// TestPromoteAgent_TargetCredentialRevoked_BlocksWithRegenerateMessage proves
// the hard block's error message is state-specific: a revoked credential
// (COMPLETED status, no stored secret) must never tell the caller to just
// retry — retrying promotion can never fix a revoked credential, only an
// explicit regenerate can.
func TestPromoteAgent_TargetCredentialRevoked_BlocksWithRegenerateMessage(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)

	stubBindingState(t, s, &AgentThunderBindingState{Status: models.AgentThunderStatusCompleted, HasSecret: false}, nil)

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "revoked")
	assert.Contains(t, ve.Reason, "regenerate")
	assert.NotContains(t, renderedUIError(ve), "still being provisioned", "a revoked credential must not tell the caller to just retry")
	assert.False(t, *promoteCalled)
}

// TestPromoteAgent_TargetProvisioningFailed_BlocksWithReprovisionMessage
// proves the hard block's error message is state-specific: a permanently
// FAILED binding (retry budget exhausted) must never tell the caller to just
// retry promotion — that will never succeed without re-provisioning.
func TestPromoteAgent_TargetProvisioningFailed_BlocksWithReprovisionMessage(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)

	stubBindingState(t, s, &AgentThunderBindingState{Status: models.AgentThunderStatusFailed, LastError: "thunder unreachable"}, nil)

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "failed")
	assert.Contains(t, ve.Reason, "thunder unreachable")
	assert.Contains(t, ve.Reason, "re-provision")
	assert.NotContains(t, renderedUIError(ve), "still being provisioned", "a permanently failed binding must not tell the caller to just retry")
	assert.False(t, *promoteCalled)
}

// maxPromotionUIErrorLen guards against a blocked promotion regrowing into the
// wall of text it used to be. The console renders a failed request as
// "<message>: <reason>" (see extractServerErrorMessage in the api-client), so
// both halves together are the UI string. The alert wraps rather than
// truncates, so this is a legibility budget rather than a rendering limit —
// the pre-split messages ran past 300 characters.
//
// What the code bounds is the unbounded detail it interpolates: upstream
// failure text and the list of connection names. The environment name is
// echoed verbatim on purpose (the caller needs to know which one blocked), so
// a deliberately long environment name can still push a message past this.
const maxPromotionUIErrorLen = 200

// captureLogs points s at a buffer-backed logger and returns the accumulated
// output, so a test can assert that detail withheld from the UI still reaches
// the operator.
func captureLogs(s *agentManagerService) func() string {
	var buf bytes.Buffer
	s.logger = slog.New(slog.NewTextHandler(&buf, nil))
	return buf.String
}

// stubBindingState makes GetBindingState return a fixed result, selecting which
// arm of the hard block's state switch runs.
func stubBindingState(t *testing.T, s *agentManagerService, state *AgentThunderBindingState, err error) {
	t.Helper()
	stub, ok := s.agentThunderProvisioning.(*provisionForEnvIfMissingStub)
	require.True(t, ok)
	// (nil, nil) is what GetBindingState itself returns for a missing row.
	stub.GetBindingStateFunc = func(context.Context, string, string, string, string) (*AgentThunderBindingState, error) {
		return state, err
	}
}

// renderedUIError is the string the console actually shows for a failed
// request, so assertions about "what the user sees" all measure the same thing.
func renderedUIError(ve *utils.ValidationError) string {
	return ve.Message + ": " + ve.Reason
}

// requireBriefPromotionBlock asserts err is the caller-facing form of a
// blocked promotion: a ValidationError (so the UI gets the short Message
// instead of the whole technical string) that still classifies as invalid
// input, and whose rendered form stays inside the legibility budget.
func requireBriefPromotionBlock(t *testing.T, err error) *utils.ValidationError {
	t.Helper()
	require.Error(t, err)
	require.ErrorIs(t, err, utils.ErrInvalidInput, "the block must still classify as invalid input, or the controller answers 500 instead of 400")
	ve := utils.IsValidationError(err)
	require.NotNil(t, ve, "the block must carry a short UI Message separate from its technical Reason")

	rendered := renderedUIError(ve)
	assert.LessOrEqual(t, utf8.RuneCountInString(rendered), maxPromotionUIErrorLen,
		"the UI string is too lengthy (%d runes): %s", utf8.RuneCountInString(rendered), rendered)
	assert.NotContains(t, rendered, "check GET ", "REST call hints belong in the logs, not the UI")
	return ve
}

// The UI must get a short, actionable sentence when the target environment's
// identity is still provisioning; the rationale for the block (a
// cross-environment credential leak) is operator detail and belongs in the
// log instead.
func TestPromoteAgent_IdentityStillProvisioning_KeepsUIErrorBriefAndLogsDetail(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)
	logs := captureLogs(s)

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "staging", "the message must name the environment that blocked the promotion")
	assert.Contains(t, ve.Message, "still being provisioned")
	assert.NotContains(t, ve.Message, "inherit", "the leak rationale is operator detail, not a UI message")
	assert.Contains(t, logs(), "inherit", "the full rationale must still reach the log")
	assert.Contains(t, logs(), "staging")
	assert.False(t, *promoteCalled,
		"promotion must be blocked BEFORE calling PromoteComponent — otherwise the pod is already promoted with leaked credentials by the time this error is returned")
}

// The same short-message contract for the state the hard block cannot explain
// from a binding row: provisioning was only just triggered, so no row exists
// yet.
func TestPromoteAgent_IdentityBindingMissing_KeepsUIErrorBriefAndSaysRetry(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)
	logs := captureLogs(s)
	stubBindingState(t, s, nil, nil)

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "staging")
	assert.Contains(t, ve.Reason, "just triggered",
		"the missing-row arm must be distinguishable from the still-provisioning arm, which shares its Message")
	assert.Contains(t, logs(), "staging")
	assert.False(t, *promoteCalled)
}

// A repository failure reading the binding row is an operational fault, not a
// caller mistake: GetBindingState reserves (nil, nil) for "no binding row yet"
// and wraps everything else. Reporting it as a validation error would tell the
// user to retry a promotion that a retry cannot fix, and would answer 400 for
// a server-side failure.
func TestPromoteAgent_BindingStateReadFails_ReportsOperationalFailureNotValidation(t *testing.T) {
	readFailure := errors.New("read agent thunder binding state: dial tcp: connection refused")
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)
	logs := captureLogs(s)
	stubBindingState(t, s, nil, readFailure)

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, readFailure, "the underlying failure must stay wrapped so the cause survives to the logs")
	assert.NotErrorIs(t, err, utils.ErrInvalidInput, "a repository failure is not invalid input; classifying it as one answers 400 for a server fault")
	assert.Nil(t, utils.IsValidationError(err), "an operational failure must not be dressed up as a user-facing validation message")
	assert.Contains(t, logs(), "proj1", "the failure must be logged with the project context")
	assert.False(t, *promoteCalled)
}

// Realistic worst case for the MCP block: several long connection names.
// Connection names are user-supplied and unbounded, so
// without shortening the list the UI string grows into the wall of text the
// split exists to prevent — and shortening must drop whole names, never cut one
// mid-name and put a connection that does not exist in front of the user.
func TestPromoteAgent_ManyLongMCPConnectionNames_KeepsUIErrorBriefAndNamesNoPartialConnection(t *testing.T) {
	names := []string{"booking-service", "invoicing-api", "notifications-fanout", "payments-gateway", "search-indexer"}
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "target-client-id"}}, nil)
	stubUnresolvedMCPs(t, s, "staging", names...)
	logs := captureLogs(s)

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "(+", "a shortened list must say how many connections it left out")
	assert.NotContains(t, ve.Message, "…",
		"a list is shortened by dropping whole names, so no name may be cut mid-word")
	for _, name := range names {
		assert.Contains(t, logs(), name, "every blocked connection must reach the log")
	}
	assert.False(t, *promoteCalled)
}

// A Thunder failure message is unbounded — the whole point of the split is
// that it cannot push the UI string back over the limit. The full text must
// still be logged verbatim.
func TestPromoteAgent_ProvisioningFailedWithLongLastError_TruncatesUIReason(t *testing.T) {
	longLastError := "thunder unreachable: " + strings.Repeat("connection refused; ", 30)
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)
	logs := captureLogs(s)
	stubBindingState(t, s, &AgentThunderBindingState{Status: models.AgentThunderStatusFailed, LastError: longLastError}, nil)

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Reason, "thunder unreachable", "the head of the failure is the most useful part to keep")
	assert.Contains(t, logs(), longLastError, "the untruncated failure must reach the log")
	assert.False(t, *promoteCalled)
}

// TestPromoteAgent_ProvisioningDisabled_SkipsIdentityCheckAndPromotes covers a
// deployment mode where AgentID provisioning is not wired in at all
// (app.Options.AgentThunderProvisioning is nil, so agentManagerService's
// agentThunderProvisioning field is nil too — see app.go). agentIdentityInjection
// is NOT nil here: it is wired unconditionally in production (see
// wiring.ProvideAgentIdentityInjectionService), independent of whether AgentID
// provisioning itself is enabled, so it is always a real, callable service —
// just one that finds nothing to inject when no binding has ever been created.
// agentThunderProvisioning is deliberately left nil (not a stub) so a
// regression that calls it outside the disabled-provisioning guard fails with
// a nil-interface panic rather than silently passing. PromoteAgent must not
// hard-block the promotion just because no AgentID binding will ever exist.
func TestPromoteAgent_ProvisioningDisabled_SkipsIdentityCheckAndPromotes(t *testing.T) {
	promoteCalled := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetOrganizationFunc: func(_ context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: orgName}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: "agent-chat"}, // deliberately not agent-api
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		// Default: the component reconciles normally, so the promote pre-flight lets
		// it through. Tests covering the pre-flight override this directly.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		IsDeploymentInProgressFunc:      func(_ context.Context, _, _, _ string) (bool, error) { return false, nil },
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error { return nil },
		PromoteComponentFunc: func(_ context.Context, _, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
			promoteCalled = true
			return nil
		},
	}
	agentConfigSvc := &stubAgentConfigurationServiceForPromote{
		SystemKeysFunc: func(_ context.Context, _, _, _, _ string) (map[string]bool, error) { return map[string]bool{}, nil },
		SystemVarsFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) { return nil, nil },
	}
	identityInjector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) {
			// No binding exists anywhere for this agent — AgentID has genuinely
			// never been used, in dev or staging, so there is nothing that
			// could leak from one environment's pod into another's.
			return nil, nil
		},
	}

	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: agentConfigSvc,
		agentIdentityInjection:    identityInjector,
		logger:                    discardLogger(),
		// agentThunderProvisioning intentionally omitted (nil).
	}

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.NoError(t, err)
	assert.True(t, promoteCalled, "promotion must proceed normally when AgentID has never been used for this agent")
}

// TestPromoteAgent_ProvisioningDisabledButLowestEnvHasRealCredential_StillBlocks
// covers the actual cross-environment leak this deployment mode must still
// guard against: a deployment that provisioned real AgentID credentials while
// enabled, then had provisioning disabled afterward (agentThunderProvisioning
// is nil, but agentIdentityInjection — always wired — still finds the lowest
// environment's real, pre-existing credential). DeployAgent's own identity
// injection is not gated on agentThunderProvisioning, so it keeps writing that
// lowest environment's real client_id/client_secret into the shared Workload
// CR regardless. If PromoteAgent let this through without the target's own
// override, the promoted pod would silently inherit that real credential.
func TestPromoteAgent_ProvisioningDisabledButLowestEnvHasRealCredential_StillBlocks(t *testing.T) {
	promoteCalled := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, _ []client.EnvVar, _ []client.FileVar) error {
			return nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetOrganizationFunc: func(_ context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: orgName}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: "agent-chat"},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		// Default: the component reconciles normally, so the promote pre-flight lets
		// it through. Tests covering the pre-flight override this directly.
		GetComponentReconcileBlockFunc: func(_ context.Context, _, _ string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // documented contract: a nil block means "not blocked"
		},
		IsDeploymentInProgressFunc:      func(_ context.Context, _, _, _ string) (bool, error) { return false, nil },
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error { return nil },
		PromoteComponentFunc: func(_ context.Context, _, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
			promoteCalled = true
			return nil
		},
	}
	agentConfigSvc := &stubAgentConfigurationServiceForPromote{
		SystemKeysFunc: func(_ context.Context, _, _, _, _ string) (map[string]bool, error) { return map[string]bool{}, nil },
		SystemVarsFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) { return nil, nil },
	}
	identityInjector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, envName string) ([]client.EnvVar, error) {
			if envName == "dev" {
				// dev was provisioned and deployed while AgentID was still
				// enabled — its real credential is already sitting in the
				// shared Workload CR.
				return []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"}}, nil
			}
			// staging has never been provisioned, and — provisioning being
			// disabled now — never will be automatically.
			return nil, nil
		},
	}

	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: agentConfigSvc,
		agentIdentityInjection:    identityInjector,
		logger:                    discardLogger(),
		// agentThunderProvisioning intentionally omitted (nil): provisioning
		// disabled NOW, even though dev was provisioned earlier while it was on.
	}

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Contains(t, ve.Message, "provisioning is disabled")
	assert.False(t, promoteCalled,
		"promotion must be blocked BEFORE calling PromoteComponent — otherwise the pod is already promoted with the lowest environment's leaked credentials by the time this error is returned")
}

func pipelineWithEnv(envName string) *models.DeploymentPipelineResponse {
	return &models.DeploymentPipelineResponse{
		PromotionPaths: []models.PromotionPath{
			{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: envName}}},
		},
	}
}

func TestGetAgentRoles_EnvironmentInPipeline_DelegatesToProvisioning(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	wantRoles := []thundersvc.ThunderRole{{ID: "role-1", Name: "reader"}}
	stub := &stubAgentThunderProvisioning{
		GetAgentRolesFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderRole, error) {
			return wantRoles, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	roles, err := s.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, wantRoles, roles)
}

// TestGetAgentRoles_EnvironmentNotInPipeline_Errors guards the same visibility
// rule GetAgentIdentity applies: a project only ever sees bindings for
// environments in its own deployment pipeline, even though AgentIDs are
// provisioned across every org-level environment.
func TestGetAgentRoles_EnvironmentNotInPipeline_Errors(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	stub := &stubAgentThunderProvisioning{
		GetAgentRolesFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderRole, error) {
			t.Fatal("must not reach the provisioning layer for an environment outside the project's pipeline")
			return nil, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "prod")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

func TestGetAgentGroups_EnvironmentInPipeline_DelegatesToProvisioning(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	wantGroups := []thundersvc.ThunderGroup{{ID: "group-1", Name: "operators"}}
	stub := &stubAgentThunderProvisioning{
		GetAgentGroupsFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderGroup, error) {
			return wantGroups, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	groups, err := s.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, wantGroups, groups)
}

// TestGetAgentGroups_EnvironmentNotInPipeline_Errors is the groups counterpart
// to TestGetAgentRoles_EnvironmentNotInPipeline_Errors.
func TestGetAgentGroups_EnvironmentNotInPipeline_Errors(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	stub := &stubAgentThunderProvisioning{
		GetAgentGroupsFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderGroup, error) {
			t.Fatal("must not reach the provisioning layer for an environment outside the project's pipeline")
			return nil, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "prod")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

// Labels now live on the OpenChoreo component itself rather than a local
// sidecar table; these tests cover the service-layer wiring that threads
// labels into/out of the OpenChoreo client requests. The label-merge/extract
// logic itself is tested at the client-package level
// (clients/openchoreosvc/client/component_labels_test.go).

func TestToCreateAgentRequestWithSecrets_PassesLabelsThrough(t *testing.T) {
	s := &agentManagerService{}

	t.Run("labels set", func(t *testing.T) {
		labels := map[string]string{"team": "ml"}
		req := &spec.CreateAgentRequest{Name: "agent-1", DisplayName: "Agent 1", Labels: &labels}

		result := s.toCreateAgentRequestWithSecrets(req, "")

		assert.Equal(t, labels, result.Labels)
	})

	t.Run("labels nil", func(t *testing.T) {
		req := &spec.CreateAgentRequest{Name: "agent-1", DisplayName: "Agent 1"}

		result := s.toCreateAgentRequestWithSecrets(req, "")

		assert.Nil(t, result.Labels)
	})
}

func TestUpdateAgentBasicInfo_PassesLabelsThroughToClient(t *testing.T) {
	newLabels := map[string]string{"team": "ml"}
	emptyLabels := map[string]string{}

	testCases := []struct {
		name       string
		reqLabels  *map[string]string
		wantLabels *map[string]string
	}{
		{name: "nil means unchanged", reqLabels: nil, wantLabels: nil},
		{name: "empty map clears user labels", reqLabels: &emptyLabels, wantLabels: &emptyLabels},
		{name: "populated map replaces user labels", reqLabels: &newLabels, wantLabels: &newLabels},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var captured client.UpdateComponentBasicInfoRequest
			ocClient := &clientmocks.OpenChoreoClientMock{
				GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
					return &models.OrganizationResponse{}, nil
				},
				GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
					return &models.ProjectResponse{}, nil
				},
				GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
					return &models.AgentResponse{Name: "agent-1"}, nil
				},
				UpdateComponentBasicInfoFunc: func(_ context.Context, _, _, _ string, req client.UpdateComponentBasicInfoRequest) error {
					captured = req
					return nil
				},
			}
			s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

			_, err := s.UpdateAgentBasicInfo(context.Background(), "acme", "proj1", "agent-1", &spec.UpdateAgentBasicInfoRequest{
				DisplayName: "Agent 1",
				Description: "desc",
				Labels:      tc.reqLabels,
			})

			require.NoError(t, err)
			assert.Equal(t, tc.wantLabels, captured.Labels)
		})
	}
}

func TestGetAgent_LabelsPassThroughUnmodified(t *testing.T) {
	wantLabels := map[string]string{"team": "ml"}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Name: "agent-1", Labels: wantLabels}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return nil, errors.New("no pipeline in this test")
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	agent, err := s.GetAgent(context.Background(), "acme", "proj1", "agent-1")

	require.NoError(t, err)
	assert.Equal(t, wantLabels, agent.Labels)
}

func TestListAgents_LabelsPassThroughUnmodified(t *testing.T) {
	agents := []*models.AgentResponse{
		{Name: "agent-1", Labels: map[string]string{"env": "prod"}},
		{Name: "agent-2", Labels: map[string]string{"env": "dev"}},
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListComponentsFunc: func(_ context.Context, _, _ string) ([]*models.AgentResponse, error) {
			return agents, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	t.Run("unfiltered returns all agents with their labels intact", func(t *testing.T) {
		result, total, err := s.ListAgents(context.Background(), "acme", "proj1", nil, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(2), total)
		require.Len(t, result, 2)
		assert.Equal(t, map[string]string{"env": "prod"}, result[0].Labels)
	})

	t.Run("filtered narrows to the matching agent", func(t *testing.T) {
		result, total, err := s.ListAgents(context.Background(), "acme", "proj1", map[string]string{"env": "prod"}, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(1), total)
		require.Len(t, result, 1)
		assert.Equal(t, "agent-1", result[0].Name)
	})
}

func int32Ptr(v int32) *int32 {
	return &v
}

// mirrors what buildAPIConfigurationTraitParameters does
// internally when a trait is actually attached. used to inspect
// the parameters a TraitRequest's Opts would produce without needing a real
// OpenChoreo client.
func applyTraitOpts(opts []client.TraitOption) map[string]interface{} {
	params := map[string]interface{}{}
	for _, opt := range opts {
		opt(params)
	}
	return params
}

func findTraitRequest(traits []client.TraitRequest, traitType client.TraitType) *client.TraitRequest {
	for i := range traits {
		if traits[i].TraitType == traitType {
			return &traits[i]
		}
	}
	return nil
}

func TestResolveResilienceTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name      string
		existing  *models.AgentConfig
		requested *int32
		withDef   bool
		want      int32
		wantErr   bool
	}{
		{"nothing set, withDefaults=true falls back to default", nil, nil, true, client.DefaultResilienceTimeoutSeconds, false},
		{"nothing set, withDefaults=false yields zero (no override)", nil, nil, false, 0, false},
		{"existing DB value is used", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, nil, false, 60, false},
		{"request overrides existing DB value", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, int32Ptr(90), false, 90, false},
		{"out-of-bounds request is rejected", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, int32Ptr(10000), false, 0, true},
		{"minimum bound is accepted", nil, int32Ptr(client.MinResilienceTimeoutSeconds), false, client.MinResilienceTimeoutSeconds, false},
		{"maximum bound is accepted", nil, int32Ptr(client.MaxResilienceTimeoutSeconds), false, client.MaxResilienceTimeoutSeconds, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveResilienceTimeoutSeconds(tc.existing, tc.requested, tc.withDef)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, utils.ErrInvalidInput)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// OpenChoreo client and repository mocks needed to drive DeployAgent
func deployAPIAgentMocks(existingConfig *models.AgentConfig) (*agentManagerService, *client.ComponentDeploymentConfigRequest) {
	var capturedDeployConfig client.ComponentDeploymentConfigRequest
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning:   models.Provisioning{Type: string(utils.InternalAgent)},
				Type:           models.AgentType{Type: string(utils.AgentTypeAPI)},
				InputInterface: &models.InputInterface{Port: 8000, BasePath: "/"},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return nil, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: "env-uuid"}, nil
		},
		IsDeploymentInProgressFunc: func(context.Context, string, string, string) (bool, error) {
			return false, nil
		},
		// Deploy cuts the release and writes env vars and file mounts to the environment's
		// ReleaseBinding, leaving the component-wide base alone. ReplaceComponentEnvVars and
		// ReplaceComponentFileMounts are left unstubbed on purpose: a regression that writes the
		// shared base again panics here instead of silently leaking config into every environment.
		EnsureReleaseAndBindingFunc: func(context.Context, string, string, string, string, []client.EnvVar, []client.FileVar) error {
			return nil
		},
		// Not blocked, so the deploy runs to completion.
		GetComponentReconcileBlockFunc: func(context.Context, string, string) (*client.ComponentReconcileBlock, error) {
			return nil, nil //nolint:nilnil // nil block is the "not blocked" signal this API defines
		},
		UpdateComponentDeploymentConfigFunc: func(_ context.Context, _, _, _ string, req client.ComponentDeploymentConfigRequest) error {
			capturedDeployConfig = req
			return nil
		},
		EnsureProjectReleaseBindingFunc: func(_ context.Context, _, _, _ string) error { return nil },
		DeployFunc: func(context.Context, string, string, string, client.DeployRequest) error {
			return nil
		},
		UpdateReleaseBindingTraitConfigsFunc: func(context.Context, string, string, string, map[string]interface{}, map[string]interface{}) error {
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
			return nil, nil
		},
	}
	artifactRepo := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(handle, orgUUID string) (*models.Artifact, error) {
			return &models.Artifact{UUID: uuid.Must(uuid.NewV7()), Handle: handle, Kind: models.KindAgent, OUID: orgUUID}, nil
		},
	}
	agentConfigRepo := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			if existingConfig == nil {
				return nil, repositories.ErrAgentConfigNotFound
			}
			return existingConfig, nil
		},
		UpsertFunc: func(context.Context, *models.AgentConfig) error {
			return nil
		},
	}
	s := &agentManagerService{
		ocClient:               ocClient,
		agentIdentityInjection: injector,
		artifactRepo:           artifactRepo,
		agentConfigRepo:        agentConfigRepo,
		logger:                 discardLogger(),
	}
	return s, &capturedDeployConfig
}

// covers the deploy-time wiring of ResilienceTimeoutSeconds into the
// api-management trait's resilienceTimeout parameter.
func TestDeployAgent_APIAgent_ResilienceTimeout(t *testing.T) {
	tests := []struct {
		name                  string
		existingConfig        *models.AgentConfig
		wantResilienceTimeout string
	}{
		{"persisted value within bounds produces \"<N>s\"", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(45)}, "45s"},
		{"no persisted config falls back to \"30s\"", nil, "30s"},
		{"persisted nil falls back to \"30s\"", &models.AgentConfig{}, "30s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, capturedDeployConfig := deployAPIAgentMocks(tc.existingConfig)

			env, err := s.DeployAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{ImageId: "registry.example.com/my-agent:v1"})

			require.NoError(t, err)
			assert.Equal(t, "dev", env)
			apiTrait := findTraitRequest(capturedDeployConfig.TraitsToAttach, client.TraitAPIManagement)
			require.NotNil(t, apiTrait, "expected an api-management trait to be attached for an API agent deploy")
			params := applyTraitOpts(apiTrait.Opts)
			assert.Equal(t, tc.wantResilienceTimeout, params["resilienceTimeout"])
		})
	}
}

// covers the "editable immediately without redeploy" path:
func TestUpdateAgentDeploySettings_ResilienceTimeout(t *testing.T) {
	tests := []struct {
		name                string
		existingConfig      *models.AgentConfig
		requested           *int32
		wantPersisted       *int32
		wantEnvOverridePush string
	}{
		{"request sets a new value", nil, int32Ptr(90), int32Ptr(90), "90s"},
		{"omitted request keeps existing DB value", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, nil, int32Ptr(60), "60s"},
		{"nothing set omits the override, no key pushed", nil, nil, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var pushedTraitEnvConfigs map[string]interface{}
			var persisted *models.AgentConfig
			ocClient := &clientmocks.OpenChoreoClientMock{
				GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
					return &models.OrganizationResponse{Name: name}, nil
				},
				GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
					return &models.AgentResponse{Type: models.AgentType{Type: string(utils.AgentTypeAPI)}}, nil
				},
				GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
					return &models.EnvironmentResponse{Name: name, UUID: "env-uuid"}, nil
				},
				UpdateReleaseBindingTraitConfigsFunc: func(_ context.Context, _, _, _ string, traitConfigs map[string]interface{}, _ map[string]interface{}) error {
					pushedTraitEnvConfigs = traitConfigs
					return nil
				},
			}
			artifactRepo := &repomocks.ArtifactRepositoryMock{
				GetByHandleFunc: func(handle, orgUUID string) (*models.Artifact, error) {
					return &models.Artifact{UUID: uuid.Must(uuid.NewV7()), Handle: handle, Kind: models.KindAgent, OUID: orgUUID}, nil
				},
			}
			agentConfigRepo := &repomocks.AgentConfigRepositoryMock{
				GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
					if tc.existingConfig == nil {
						return nil, repositories.ErrAgentConfigNotFound
					}
					return tc.existingConfig, nil
				},
				UpsertFunc: func(_ context.Context, cfg *models.AgentConfig) error {
					persisted = cfg
					return nil
				},
			}
			s := &agentManagerService{
				ocClient:        ocClient,
				artifactRepo:    artifactRepo,
				agentConfigRepo: agentConfigRepo,
				logger:          discardLogger(),
			}

			err := s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.UpdateAgentDeploySettingsRequest{
				EnvironmentName:          "dev",
				ResilienceTimeoutSeconds: tc.requested,
			})

			require.NoError(t, err)
			if tc.wantPersisted == nil {
				assert.Nil(t, persisted.ResilienceTimeoutSeconds)
			} else {
				require.NotNil(t, persisted.ResilienceTimeoutSeconds)
				assert.Equal(t, *tc.wantPersisted, *persisted.ResilienceTimeoutSeconds)
			}
			apiTraitCfg, _ := pushedTraitEnvConfigs["my-agent-api-configuration"].(map[string]interface{})
			if tc.wantEnvOverridePush == "" {
				_, present := apiTraitCfg["resilienceTimeout"]
				assert.False(t, present, "expected no resilienceTimeout override to be pushed")
			} else {
				assert.Equal(t, tc.wantEnvOverridePush, apiTraitCfg["resilienceTimeout"])
			}
		})
	}
}

// TestUpdateAgentDeploySettings_ResilienceTimeout_OutOfBoundsRejected guards
// against an explicitly-requested out-of-bounds value being silently discarded:
// the call must fail with ErrInvalidInput before it touches the release
// binding or persists anything. UpdateReleaseBindingTraitConfigsFunc and
// UpsertFunc are deliberately left nil so an unwanted call panics the test.
func TestUpdateAgentDeploySettings_ResilienceTimeout_OutOfBoundsRejected(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Type: models.AgentType{Type: string(utils.AgentTypeAPI)}}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: "env-uuid"}, nil
		},
	}
	agentConfigRepo := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return nil, repositories.ErrAgentConfigNotFound
		},
	}
	s := &agentManagerService{
		ocClient:        ocClient,
		agentConfigRepo: agentConfigRepo,
		logger:          discardLogger(),
	}

	err := s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.UpdateAgentDeploySettingsRequest{
		EnvironmentName:          "dev",
		ResilienceTimeoutSeconds: int32Ptr(10000),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestPopulateCreatedBy_NoProvisioning_LeavesUnset(t *testing.T) {
	s := &agentManagerService{logger: discardLogger()}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_NoIdentityClient_LeavesUnset(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			t.Fatal("must not fetch identity views when there is no identity client to resolve a username with")
			return nil, nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: discardLogger()}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_IdentityViewsError_LeavesUnset(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return nil, errors.New("db unavailable")
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient:           &clientmocks.IdentityClientMock{},
		logger:                   discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_NoRequestedBy_LeavesUnset(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{EnvironmentName: "dev"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				t.Fatal("must not resolve a user when no binding recorded a requester")
				return &thundersvc.ThunderUser{}, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_ResolvesUsernameFromAttributes(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{
				{EnvironmentName: "dev", RequestedBy: ""},
				{EnvironmentName: "staging", RequestedBy: "user-123"},
			}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
				assert.Equal(t, "user-123", userID)
				return &thundersvc.ThunderUser{
					ID:         userID,
					Display:    "John Doe",
					Attributes: map[string]any{"username": "john.doe"},
				}, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Equal(t, "john.doe", agent.CreatedBy.Display)
}

func TestPopulateCreatedBy_FallsBackToDisplayWhenNoUsernameAttribute(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
				return &thundersvc.ThunderUser{ID: userID, Display: "John Doe"}, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "John Doe", agent.CreatedBy.Display)
}

func TestPopulateCreatedBy_UserDeleted_KeepsIDOnly(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				return nil, &thundersvc.NotFoundError{Message: "user not found"}
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Empty(t, agent.CreatedBy.Display)
}

func TestPopulateCreatedBy_UserLookupError_KeepsIDOnly(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				return nil, errors.New("thunder unreachable")
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Empty(t, agent.CreatedBy.Display)
}

// A (nil, nil) return isn't something the real Thunder client does, but this is
// best-effort decoration on the GetAgent path: it must degrade to id-only
// rather than panic if any IdentityClient implementation ever behaves that way.
func TestPopulateCreatedBy_NilUserWithoutError_KeepsIDOnly(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				//nolint:nilnil // deliberately exercising a misbehaving implementation
				return nil, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Empty(t, agent.CreatedBy.Display)
}

func TestListOrgAgents_AggregatesAcrossProjects(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
			return []*models.ProjectResponse{
				{Name: "proj1", DisplayName: "Project One"},
				{Name: "proj2", DisplayName: "Project Two"},
			}, nil
		},
		ListComponentsFunc: func(_ context.Context, _ string, projectName string) ([]*models.AgentResponse, error) {
			switch projectName {
			case "proj1":
				return []*models.AgentResponse{{Name: "agent-a", DisplayName: "Agent A", ProjectName: "proj1"}}, nil
			case "proj2":
				return []*models.AgentResponse{{Name: "agent-b", DisplayName: "Agent B", ProjectName: "proj2"}}, nil
			default:
				return nil, fmt.Errorf("unexpected project name %q", projectName)
			}
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	agents, err := s.ListOrgAgents(context.Background(), "acme")

	require.NoError(t, err)
	byName := make(map[string]*models.AgentSummary, len(agents))
	for _, a := range agents {
		byName[a.Name] = a
	}
	require.Contains(t, byName, "agent-a")
	require.Contains(t, byName, "agent-b")
	assert.Equal(t, "Project One", byName["agent-a"].ProjectDisplayName)
	assert.Equal(t, "Project Two", byName["agent-b"].ProjectDisplayName)
}

func TestListOrgAgents_OrganizationNotFound(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return nil, utils.ErrNotFound
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	_, err := s.ListOrgAgents(context.Background(), "acme")

	assert.ErrorIs(t, err, utils.ErrOrganizationNotFound)
}

func TestListOrgAgents_ProjectListFailurePropagates(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
			return nil, errors.New("openchoreo unavailable")
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	_, err := s.ListOrgAgents(context.Background(), "acme")

	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrOrganizationNotFound, "an unrelated openchoreo failure must not be masked as not-found")
}

// stubSystemManagedConfigs gives the agent system-managed configuration in
// configuredEnv and none anywhere else, which is the shape that trips the
// missing-configuration block on a promotion out of configuredEnv.
func stubSystemManagedConfigs(t *testing.T, s *agentManagerService, configuredEnv string, configs ...SystemManagedConfigRef) {
	t.Helper()
	agentConfigSvc := promoteConfigStub(t, s)
	agentConfigSvc.SystemKeysFunc = func(_ context.Context, _, _, _, envName string) (map[string]bool, error) {
		if envName != configuredEnv {
			return map[string]bool{}, nil
		}
		keys := map[string]bool{}
		for _, config := range configs {
			keys[config.Name+"_URL"] = true
		}
		return keys, nil
	}
	agentConfigSvc.SystemConfigsFunc = func(_ context.Context, _, _, _, envName string) ([]SystemManagedConfigRef, error) {
		if envName != configuredEnv {
			return []SystemManagedConfigRef{}, nil
		}
		return configs, nil
	}
}

func mcpConfigRef(name string) SystemManagedConfigRef {
	return SystemManagedConfigRef{Name: name, TypeID: models.AgentConfigTypeIDMCP}
}

func llmConfigRef(name string) SystemManagedConfigRef {
	return SystemManagedConfigRef{Name: name, TypeID: models.AgentConfigTypeIDLLM}
}

// An agent whose only system-managed configuration is an MCP connection used to be
// refused with "no LLM/system configuration", sending the user to look for an LLM
// provider that was never involved. The block must name the configuration that is
// actually missing from the target.
func TestPromoteAgent_TargetMissingMCPOnlyConfig_NamesTheConfigurationNotLLM(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubSystemManagedConfigs(t, s, "dev", mcpConfigRef("booking"))

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Equal(t, `Promotion blocked: MCP configuration "booking" is not connected in "staging"`, ve.Message)
	assert.Equal(t, `connect it in "staging", then promote`, ve.Reason)
	assert.NotContains(t, renderedUIError(ve), "LLM",
		"no LLM provider is involved, so naming one sends the user after a configuration that does not exist")
	assert.False(t, *promoteCalled)
}

// One missing configuration is the common case, so the block reads as a sentence
// about it rather than hedging with "configuration(s)" and "it/them".
func TestPromoteAgent_TargetMissingSeveralMCPConfigs_ReadsAsPlural(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubSystemManagedConfigs(t, s, "dev", mcpConfigRef("payments"), mcpConfigRef("booking"))

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Equal(t, `Promotion blocked: MCP configurations booking, payments are not connected in "staging"`, ve.Message,
		"the names are listed in a stable order, so the same refusal reads the same on every run")
	assert.Equal(t, `connect them in "staging", then promote`, ve.Reason)
	assert.NotContains(t, renderedUIError(ve), "(s)", "the wording agrees with the count instead of hedging")
}

// The same block fires when it really is the LLM provider that the target lacks.
// That wording is correct there and is what callers already handle, so an agent
// with no MCP connection must keep the message and reason it has always produced.
func TestPromoteAgent_TargetMissingLLMConfig_KeepsTheGenericWording(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubSystemManagedConfigs(t, s, "dev", llmConfigRef("openai"))

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Equal(t, `Promotion blocked: no LLM/system configuration in "staging"`, ve.Message)
	assert.Equal(t, `configure system variables in "staging", then promote`, ve.Reason)
}

// An agent missing both kinds is genuinely missing its LLM configuration, so the
// message keeps its wording. The MCP connections are missing too, though, and a
// user who configures only the system variables would be refused all over again —
// so the reason names them.
func TestPromoteAgent_TargetMissingLLMAndMCPConfigs_KeepsMessageAndNamesMCPInReason(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubSystemManagedConfigs(t, s, "dev", llmConfigRef("openai"), mcpConfigRef("booking"))

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Equal(t, `Promotion blocked: no LLM/system configuration in "staging"`, ve.Message,
		"the message stays byte-identical so anything already handling this block keeps working")
	assert.Equal(t, `configure system variables and connect MCP configuration "booking", then promote`, ve.Reason)
}

// The lookup that names the configurations runs only to describe a refusal that has
// already been decided. Failing to describe it must not escalate a 400 the caller can
// act on into a 500 that hides why the promotion stopped.
func TestPromoteAgent_ConfigLookupFailsWhileDescribingBlock_StillRefusesWithGenericWording(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	stubSystemManagedConfigs(t, s, "dev", mcpConfigRef("booking"))
	promoteConfigStub(t, s).SystemConfigsFunc = func(_ context.Context, _, _, _, _ string) ([]SystemManagedConfigRef, error) {
		return nil, errors.New("database unavailable")
	}

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	ve := requireBriefPromotionBlock(t, err)
	assert.Equal(t, `Promotion blocked: no LLM/system configuration in "staging"`, ve.Message)
	assert.False(t, *promoteCalled)
}

// The reason naming the MCP configurations is the longest string this block can
// produce, and it is rendered next to an environment name the user chose. Long names
// and a truncated list together must still fit the console's legibility budget.
func TestPromoteAgent_ManyMCPConfigsAndLongEnvName_StaysWithinUIBudget(t *testing.T) {
	longEnv := "production-eu-west-central-1"
	configs := []SystemManagedConfigRef{
		llmConfigRef("openai"),
		mcpConfigRef("hotel-booking-connection"),
		mcpConfigRef("payments-connection"),
		mcpConfigRef("inventory-connection"),
		mcpConfigRef("notifications-connection"),
	}
	s, _ := promoteAgentTestFixture(t, nil, nil)
	stubSystemManagedConfigs(t, s, "dev", configs...)

	message, reason := s.missingTargetConfigText(context.Background(), "my-agent", "acme", "proj1", "dev", longEnv)

	_ = requireBriefPromotionBlock(t, utils.NewInvalidInputError(message, reason))
}

// Configuration names are accepted up to 255 characters, and this block is the first
// thing that puts one on screen. A name long enough to bury the sentence it sits in
// must be cut down to a recognisable prefix rather than pasted in whole.
func TestPromoteAgent_VeryLongConfigName_IsShortenedNotPastedWhole(t *testing.T) {
	longName := "hotel-booking" + strings.Repeat("x", 242)
	require.Len(t, longName, 255, "the fixture must use the longest name the API accepts")

	for _, tc := range []struct {
		name    string
		configs []SystemManagedConfigRef
	}{
		{"single MCP configuration", []SystemManagedConfigRef{mcpConfigRef(longName)}},
		{"several MCP configurations", []SystemManagedConfigRef{mcpConfigRef(longName), mcpConfigRef("payments")}},
		{"LLM and MCP configurations", []SystemManagedConfigRef{llmConfigRef("openai"), mcpConfigRef(longName)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := promoteAgentTestFixture(t, nil, nil)
			stubSystemManagedConfigs(t, s, "dev", tc.configs...)

			message, reason := s.missingTargetConfigText(context.Background(), "my-agent", "acme", "proj1", "dev", "staging")

			rendered := renderedUIError(utils.NewInvalidInputError(message, reason))
			assert.NotContains(t, rendered, longName, "the whole name must not reach the UI")
			assert.Contains(t, rendered, "hotel-booking", "enough of the name must survive to identify the configuration")
			assert.LessOrEqual(t, utf8.RuneCountInString(rendered), maxPromotionUIErrorLen,
				"a single name must not be able to blow the message past the budget: %s", rendered)
		})
	}
}
