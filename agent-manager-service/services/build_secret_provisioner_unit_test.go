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
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type buildSecretProvisionerStub struct {
	putSourceFunc         func(context.Context, string, string, string, spec.GitHubAppSource) error
	hasSourceFunc         func(context.Context, string, string, string) (bool, error)
	deleteSourceFunc      func(context.Context, string, string, string) error
	ensureBuildSecretFunc func(context.Context, string, string, string, string) error
}

func (s *buildSecretProvisionerStub) PutSource(ctx context.Context, ouID, projectName, componentName string, source spec.GitHubAppSource) error {
	if s.putSourceFunc == nil {
		return nil
	}
	return s.putSourceFunc(ctx, ouID, projectName, componentName, source)
}

func (s *buildSecretProvisionerStub) HasSource(ctx context.Context, ouID, projectName, componentName string) (bool, error) {
	if s.hasSourceFunc == nil {
		return false, nil
	}
	return s.hasSourceFunc(ctx, ouID, projectName, componentName)
}

func (s *buildSecretProvisionerStub) DeleteSource(ctx context.Context, ouID, projectName, componentName string) error {
	if s.deleteSourceFunc == nil {
		return nil
	}
	return s.deleteSourceFunc(ctx, ouID, projectName, componentName)
}

func (s *buildSecretProvisionerStub) EnsureBuildSecret(ctx context.Context, ouID, projectName, componentName, workflowRunName string) error {
	if s.ensureBuildSecretFunc == nil {
		return nil
	}
	return s.ensureBuildSecretFunc(ctx, ouID, projectName, componentName, workflowRunName)
}

func TestPrepareBuild_NilProvisionerPreservesDefaultPath(t *testing.T) {
	s := &agentManagerService{}

	name, err := s.prepareBuild(context.Background(), "ou-1", "proj-1", "agent-1")

	require.NoError(t, err)
	assert.Empty(t, name)
}

func TestPrepareBuild_UnboundComponentPreservesDefaultPath(t *testing.T) {
	s := &agentManagerService{buildSecretProvisioner: &buildSecretProvisionerStub{
		hasSourceFunc: func(context.Context, string, string, string) (bool, error) { return false, nil },
	}}

	name, err := s.prepareBuild(context.Background(), "ou-1", "proj-1", "agent-1")

	require.NoError(t, err)
	assert.Empty(t, name)
}

func TestPrepareBuild_ProvisionsExactGeneratedRunName(t *testing.T) {
	var gotOU, gotProject, gotComponent, gotRunName string
	s := &agentManagerService{buildSecretProvisioner: &buildSecretProvisionerStub{
		hasSourceFunc: func(context.Context, string, string, string) (bool, error) { return true, nil },
		ensureBuildSecretFunc: func(_ context.Context, ouID, projectName, componentName, workflowRunName string) error {
			gotOU, gotProject, gotComponent, gotRunName = ouID, projectName, componentName, workflowRunName
			return nil
		},
	}}

	name, err := s.prepareBuild(context.Background(), "ou-1", "proj-1", "agent-1")

	require.NoError(t, err)
	assert.NotEmpty(t, name)
	assert.Equal(t, name, gotRunName)
	assert.Equal(t, "ou-1", gotOU)
	assert.Equal(t, "proj-1", gotProject)
	assert.Equal(t, "agent-1", gotComponent)
}

func TestPrepareBuild_PropagatesMintError(t *testing.T) {
	mintErr := errors.New("mint failed")
	s := &agentManagerService{buildSecretProvisioner: &buildSecretProvisionerStub{
		hasSourceFunc: func(context.Context, string, string, string) (bool, error) { return true, nil },
		ensureBuildSecretFunc: func(context.Context, string, string, string, string) error {
			return mintErr
		},
	}}

	name, err := s.prepareBuild(context.Background(), "ou-1", "proj-1", "agent-1")

	assert.Empty(t, name)
	assert.ErrorIs(t, err, mintErr)
}

func TestCleanupGitHubAppSource_DeletesBinding(t *testing.T) {
	var gotOU, gotProject, gotComponent string
	s := &agentManagerService{buildSecretProvisioner: &buildSecretProvisionerStub{
		deleteSourceFunc: func(_ context.Context, ouID, projectName, componentName string) error {
			gotOU, gotProject, gotComponent = ouID, projectName, componentName
			return nil
		},
	}}

	s.cleanupGitHubAppSource(context.Background(), "ou-1", "proj-1", "agent-1")

	assert.Equal(t, "ou-1", gotOU)
	assert.Equal(t, "proj-1", gotProject)
	assert.Equal(t, "agent-1", gotComponent)
}

func TestPrepareGitHubAppSource_RequiresInjectedProvisioner(t *testing.T) {
	req := githubAppCreateRequest()

	err := (&agentManagerService{}).prepareGitHubAppSource(req)

	assert.ErrorIs(t, err, utils.ErrServiceUnavailable)
}

func TestPrepareGitHubAppSource_NormalizesAndClearsPATSecret(t *testing.T) {
	req := githubAppCreateRequest()
	s := &agentManagerService{buildSecretProvisioner: &buildSecretProvisionerStub{}}

	err := s.prepareGitHubAppSource(req)

	require.NoError(t, err)
	require.NotNil(t, req.Provisioning.Repository.SecretRef.Get())
	assert.Empty(t, *req.Provisioning.Repository.SecretRef.Get())
	assert.Equal(t, "main", req.GithubApp.GetBranch())
	assert.Equal(t, "/agents/demo", req.GithubApp.GetAppPath())
	assert.Equal(t, "https://github.com/acme/demo", req.GithubApp.GetRepositoryUrl())
}

func TestPrepareGitHubAppSource_RejectsMismatchedRepository(t *testing.T) {
	req := githubAppCreateRequest()
	req.GithubApp.Repo = "another-repo"
	s := &agentManagerService{buildSecretProvisioner: &buildSecretProvisionerStub{}}

	err := s.prepareGitHubAppSource(req)

	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func githubAppCreateRequest() *spec.CreateAgentRequest {
	return &spec.CreateAgentRequest{
		Provisioning: spec.Provisioning{
			Type: string(utils.InternalAgent),
			Repository: &spec.RepositoryConfig{
				Url:     "https://github.com/acme/demo",
				Branch:  "main",
				AppPath: "/agents/demo",
			},
		},
		GithubApp: &spec.GitHubAppSource{
			InstallationId: 42,
			Owner:          "acme",
			Repo:           "demo",
		},
	}
}

// A GitHub App source clears secretRef to an explicit empty string, which is a
// non-nil pointer. Guarding git-secret validation on "not nil" therefore rejected
// every GitHub App agent with "git secret reference is empty", while public repos
// (nil ref) were unaffected. Each half was covered on its own; nothing pinned the
// two together, so the contradiction survived.
func TestRequiresGitSecretValidation_SkipsGitHubAppClearedSecretRef(t *testing.T) {
	req := githubAppCreateRequest()
	s := &agentManagerService{buildSecretProvisioner: &buildSecretProvisionerStub{}}

	require.NoError(t, s.prepareGitHubAppSource(req))

	// Precondition: the ref really is present-but-empty, not absent.
	require.NotNil(t, req.Provisioning.Repository.SecretRef.Get())
	require.Empty(t, *req.Provisioning.Repository.SecretRef.Get())

	assert.False(t, requiresGitSecretValidation(req.Provisioning.Repository),
		"an explicitly empty secretRef must not be validated as a git secret")
}

func TestRequiresGitSecretValidation_PublicAndPATRepositories(t *testing.T) {
	assert.False(t, requiresGitSecretValidation(nil),
		"no repository means nothing to validate")

	public := &spec.RepositoryConfig{Url: "https://github.com/acme/demo"}
	assert.False(t, requiresGitSecretValidation(public),
		"a public repository leaves secretRef absent")

	pat := &spec.RepositoryConfig{Url: "https://github.com/acme/demo"}
	pat.SetSecretRef("my-git-secret")
	assert.True(t, requiresGitSecretValidation(pat),
		"a named git secret must still be validated")
}
