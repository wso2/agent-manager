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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// projectAgentIdentitySecretProviderStub is a provider that can also set secrets.
type projectAgentIdentitySecretProviderStub struct {
	projectAgentIdentityProviderStub
	setSecret func(context.Context, ProjectAgentIdentitySecretRequest) error
}

func (s projectAgentIdentitySecretProviderStub) SetSecret(ctx context.Context, request ProjectAgentIdentitySecretRequest) error {
	return s.setSecret(ctx, request)
}

type cpSecretCtxKey struct{}

// cpSecretFixture wires a CP_MANAGED provisioning service with a fake identity
// platform, secret manager and binding repository, and records every call in
// order so tests can assert the sequence.
type cpSecretFixture struct {
	t        *testing.T
	events   []string
	requests []ProjectAgentIdentitySecretRequest
	stored   []map[string]string
	refPaths []string

	setSecretErr error
	createErr    error
	deleteErr    error
	// secretRefPath is the binding's stored SecretReference name before the operation.
	secretRefPath string
	provisioning  models.AgentProvisioningType
	ctxPropagated bool

	svc AgentThunderProvisioningService
}

func newCPSecretFixture(t *testing.T, ownership models.AgentProvisioningType) *cpSecretFixture {
	t.Helper()
	return &cpSecretFixture{t: t, provisioning: ownership, secretRefPath: "existing-ref"}
}

func (f *cpSecretFixture) repo() *repomocks.AgentThunderClientRepositoryMock {
	return &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, ouID, project, agent, env string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), OUID: ouID, ProjectName: project, AgentName: agent, EnvironmentName: env,
				ThunderAgentID: "agent-id-" + env, ThunderClientID: "client-" + env,
				ProvisioningType: f.provisioning, SecretRefPath: f.secretRefPath,
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, path string) error {
			f.events = append(f.events, "update-ref")
			f.refPaths = append(f.refPaths, path)
			return nil
		},
	}
}

func (f *cpSecretFixture) store() *clientmocks.SecretManagementClientMock {
	return &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			f.events = append(f.events, "store")
			if f.createErr != nil {
				return "", f.createErr
			}
			f.stored = append(f.stored, data)
			return "secret-reference", nil
		},
		DeleteSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, string) error {
			f.events = append(f.events, "delete")
			return f.deleteErr
		},
	}
}

// directClientMustNotBeUsed fails the test if the legacy per-environment
// Thunder client is resolved.
func (f *cpSecretFixture) directClientMustNotBeUsed() *clientmocks.EnvThunderResolverMock {
	return &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(context.Context, string, string, string) (thundersvc.ThunderClient, error) {
			f.t.Error("the legacy per-environment Thunder client must not be used for a CP_MANAGED environment")
			return nil, errors.New("direct client must not be used")
		},
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			f.t.Error("the legacy per-environment identity client must not be used for a CP_MANAGED environment")
			return nil, errors.New("direct client must not be used")
		},
	}
}

func (f *cpSecretFixture) provider() ProjectAgentIdentityProvider {
	return projectAgentIdentitySecretProviderStub{
		setSecret: func(ctx context.Context, request ProjectAgentIdentitySecretRequest) error {
			f.events = append(f.events, "set-secret")
			f.requests = append(f.requests, request)
			f.ctxPropagated = ctx.Value(cpSecretCtxKey{}) == "marker"
			return f.setSecretErr
		},
	}
}

// build creates the service. A nil provider leaves it without a secret provider.
func (f *cpSecretFixture) build(provider ProjectAgentIdentityProvider) *cpSecretFixture {
	f.t.Helper()
	svc := NewAgentThunderProvisioningService(f.repo(), f.directClientMustNotBeUsed(), f.store(), discardLogger())
	require.NoError(f.t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(
		thundersvc.IdentityManagementCPManaged, provider,
	))
	f.svc = svc
	return f
}

func (f *cpSecretFixture) ctx() context.Context {
	return context.WithValue(context.Background(), cpSecretCtxKey{}, "marker")
}

func TestRegenerateSecretCPManagedInternalSetsThenStoresTheSameSecret(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.build(f.provider())

	ownership, clientID, secret, err := f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeInternal, ownership)
	assert.Equal(t, "client-stage", clientID)
	require.NotEmpty(t, secret)
	assert.Equal(t, []string{"set-secret", "store", "update-ref"}, f.events,
		"the identity platform must accept the secret before it is stored")
	require.Len(t, f.requests, 1)
	assert.Equal(t, ProjectAgentIdentitySecretRequest{
		OrganizationID: "org-a", ProjectID: "project-a", ProjectName: "project-a", Environment: "stage",
		AgentName: "agent-a", AgentID: "agent-id-stage", ClientID: "client-stage", ClientSecret: secret,
	}, f.requests[0])
	require.Len(t, f.stored, 1)
	assert.Equal(t, secret, f.stored[0][thundersvc.AgentSecretKeyClientSecret], "the stored secret is the returned one")
	assert.Equal(t, "client-stage", f.stored[0][thundersvc.AgentSecretKeyClientID])
	assert.Equal(t, []string{"secret-reference"}, f.refPaths, "the returned SecretReference name is persisted")
	assert.True(t, f.ctxPropagated, "the caller's context must reach the provider")
}

func TestRegenerateSecretCPManagedExternalReturnsSecretWithoutStoring(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeExternal)
	f.build(f.provider())

	ownership, _, secret, err := f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeExternal, ownership)
	require.NotEmpty(t, secret)
	assert.Equal(t, []string{"set-secret"}, f.events, "an external agent secret is set on the platform and never stored")
	assert.Equal(t, secret, f.requests[0].ClientSecret)
}

func TestRegenerateSecretCPManagedProviderFailureStoresAndReturnsNothing(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.setSecretErr = errors.New("identity platform rejected the secret")
	f.build(f.provider())

	_, _, secret, err := f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.Error(t, err)
	assert.ErrorIs(t, err, f.setSecretErr)
	assert.Empty(t, secret)
	assert.Equal(t, []string{"set-secret"}, f.events, "nothing is stored and the secret reference is unchanged")
	assert.NotContains(t, err.Error(), f.requests[0].ClientSecret, "errors must not contain the secret")
}

func TestRegenerateSecretCPManagedStoreFailureAfterSetSecretErrors(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.createErr = errors.New("secret manager unavailable")
	f.build(f.provider())

	_, _, secret, err := f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.Error(t, err)
	assert.ErrorIs(t, err, f.createErr)
	assert.Empty(t, secret, "a secret that was not stored is never returned")
	assert.Equal(t, []string{"set-secret", "store"}, f.events, "the secret reference must not change")
	assert.Empty(t, f.refPaths)

	// A retry regenerates a new secret rather than reusing the failed one.
	f.createErr = nil
	_, _, retried, err := f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")
	require.NoError(t, err)
	require.Len(t, f.requests, 2)
	assert.NotEqual(t, f.requests[0].ClientSecret, retried)
	assert.Equal(t, retried, f.requests[1].ClientSecret)
}

func TestRegenerateSecretCPManagedWithoutSecretProviderFailsClosed(t *testing.T) {
	// A provider that cannot set secrets, and a service with no provider at all.
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.build(projectAgentIdentityProviderStub{})

	_, _, secret, err := f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.ErrorIs(t, err, errProjectAgentIdentitySecretUnavailable)
	assert.Empty(t, secret)
	assert.Empty(t, f.events, "nothing is stored and the direct client is not used (the resolver fails the test if it is)")

	_, err = f.svc.RevokeSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")
	require.ErrorIs(t, err, errProjectAgentIdentitySecretUnavailable)
	assert.Empty(t, f.events, "revoke must not delete anything when it cannot invalidate the secret")
}

func TestRevokeSecretCPManagedDiscardsASecretSetOnThePlatform(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.build(f.provider())

	clientID, err := f.svc.RevokeSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.NoError(t, err)
	assert.Equal(t, "client-stage", clientID)
	assert.Equal(t, []string{"set-secret", "delete", "update-ref"}, f.events,
		"the platform invalidates the old secret before the stored copy is removed")
	require.Len(t, f.requests, 1)
	assert.NotEmpty(t, f.requests[0].ClientSecret)
	assert.NotEqual(t, clientID, f.requests[0].ClientSecret, "the discarded secret is not returned")
	assert.Empty(t, f.stored, "the discarded secret is never stored")
	assert.Equal(t, []string{""}, f.refPaths, "the secret reference is cleared")
	assert.True(t, f.ctxPropagated)
}

func TestRevokeSecretCPManagedDeleteFailureLeavesPathSetForRetry(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.deleteErr = errors.New("secret manager unavailable")
	f.build(f.provider())

	clientID, err := f.svc.RevokeSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.NoError(t, err, "the platform already invalidated the secret")
	assert.Equal(t, "client-stage", clientID)
	assert.Equal(t, []string{"set-secret", "delete"}, f.events, "the path stays set so a re-revoke retries the delete")
}

func TestRevokeSecretCPManagedProviderFailureDeletesNothing(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.setSecretErr = errors.New("identity platform unavailable")
	f.build(f.provider())

	_, err := f.svc.RevokeSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")

	require.ErrorIs(t, err, f.setSecretErr)
	assert.Equal(t, []string{"set-secret"}, f.events, "the stored credential must stay while the old secret is still valid")
	assert.NotContains(t, err.Error(), f.requests[0].ClientSecret)
}

func TestSecretRotationCPManagedTargetsTheRequestedEnvironmentOnly(t *testing.T) {
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	f.build(f.provider())

	_, _, _, err := f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "development")
	require.NoError(t, err)
	_, _, _, err = f.svc.RegenerateSecret(f.ctx(), "org-a", "project-a", "agent-a", "production")
	require.NoError(t, err)
	_, err = f.svc.RevokeSecret(f.ctx(), "org-a", "project-a", "agent-a", "stage")
	require.NoError(t, err)

	require.Len(t, f.requests, 3)
	for i, env := range []string{"development", "production", "stage"} {
		assert.Equal(t, env, f.requests[i].Environment)
		assert.Equal(t, "agent-id-"+env, f.requests[i].AgentID)
		assert.Equal(t, "client-"+env, f.requests[i].ClientID)
	}
}

func TestSecretRotationLegacyDirectKeepsUsingTheDirectClient(t *testing.T) {
	var rotated, resolved int
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, agentID string) (string, error) {
		rotated++
		assert.Equal(t, "agent-id-stage", agentID)
		return "legacy-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(context.Context, string, string, string) (thundersvc.ThunderClient, error) {
			resolved++
			return tc, nil
		},
	}
	f := newCPSecretFixture(t, models.AgentProvisioningTypeInternal)
	svc := NewAgentThunderProvisioningService(f.repo(), resolver, f.store(), discardLogger())
	// LEGACY_DIRECT drops any provider, even one that can set secrets.
	require.NoError(t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(
		thundersvc.IdentityManagementLegacyDirect, f.provider(),
	))

	_, _, secret, err := svc.RegenerateSecret(context.Background(), "org-a", "project-a", "agent-a", "stage")
	require.NoError(t, err)
	assert.Equal(t, "legacy-secret", secret)
	_, err = svc.RevokeSecret(context.Background(), "org-a", "project-a", "agent-a", "stage")
	require.NoError(t, err)

	assert.Equal(t, 2, rotated)
	assert.Equal(t, 2, resolved)
	assert.Empty(t, f.requests, "the project AgentID provider is never used in LEGACY_DIRECT mode")
}
