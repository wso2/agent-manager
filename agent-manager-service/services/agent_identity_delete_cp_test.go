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
	"log/slog"
	"strings"
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

// projectAgentIdentityDeletionProviderStub is a provider that can also delete AgentIDs.
type projectAgentIdentityDeletionProviderStub struct {
	projectAgentIdentityProviderStub
	deleteIdentity func(context.Context, ProjectAgentIdentityDeleteRequest) error
}

func (s projectAgentIdentityDeletionProviderStub) DeleteAgentIdentity(ctx context.Context, request ProjectAgentIdentityDeleteRequest) error {
	return s.deleteIdentity(ctx, request)
}

// cpDeleteFixture wires a provisioning service over three bindings of one agent
// (source "development", promoted "stage" and "production") and records every
// external call in order.
type cpDeleteFixture struct {
	t         *testing.T
	events    []string
	requests  []ProjectAgentIdentityDeleteRequest
	deleteErr map[string]error
	// legacyDeleted records AgentIDs deleted through the legacy client.
	legacyDeleted []string
	seed          []models.AgentThunderClient
	// logs receives the service's log output.
	logs bytes.Buffer
}

func newCPDeleteFixture(t *testing.T, ownership models.AgentProvisioningType) *cpDeleteFixture {
	t.Helper()
	binding := func(env, source string) models.AgentThunderClient {
		return models.AgentThunderClient{
			ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
			EnvironmentName: env, PromotionSource: source, ProvisioningType: ownership,
			ThunderAgentID: "agent-id-" + env, ThunderClientID: "client-" + env, SecretRefPath: "ref-" + env,
		}
	}
	return &cpDeleteFixture{
		t:         t,
		deleteErr: map[string]error{},
		// The source is deliberately not first, to prove ordering is by role.
		seed: []models.AgentThunderClient{
			binding("stage", "development"),
			binding("development", ""),
			binding("production", "stage"),
		},
	}
}

func (f *cpDeleteFixture) repo() *repomocks.AgentThunderClientRepositoryMock {
	return &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(context.Context, string, string, string) ([]models.AgentThunderClient, error) {
			return f.seed, nil
		},
		GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
			for _, b := range f.seed {
				if b.EnvironmentName == env {
					return &b, nil
				}
			}
			return nil, errors.New("not found")
		},
		UpdateSecretRefFunc: func(context.Context, uuid.UUID, string) error { return nil },
		DeleteByIDsFunc: func(context.Context, []uuid.UUID) error {
			f.events = append(f.events, "db-delete")
			return nil
		},
	}
}

func (f *cpDeleteFixture) store() *clientmocks.SecretManagementClientMock {
	return &clientmocks.SecretManagementClientMock{
		DeleteSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ string) error {
			f.events = append(f.events, "secret-delete:"+location.EnvironmentName)
			return nil
		},
	}
}

// failIfUsedResolver fails the test if the legacy per-environment client is resolved.
func (f *cpDeleteFixture) failIfUsedResolver() *clientmocks.EnvThunderResolverMock {
	return &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(context.Context, string, string, string) (thundersvc.ThunderClient, error) {
			f.t.Error("the legacy per-environment Thunder client must not be used for a CP_MANAGED environment")
			return nil, errors.New("direct client must not be used")
		},
	}
}

// legacyResolver resolves a legacy client that records the AgentIDs it deletes.
func (f *cpDeleteFixture) legacyResolver() *clientmocks.EnvThunderResolverMock {
	tc := fakeThunderClientMock()
	tc.DeleteAgentIdentityFunc = func(_ context.Context, agentID string) (bool, error) {
		f.events = append(f.events, "legacy-delete:"+agentID)
		f.legacyDeleted = append(f.legacyDeleted, agentID)
		return true, nil
	}
	return &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(context.Context, string, string, string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
}

func (f *cpDeleteFixture) deletionProvider() ProjectAgentIdentityProvider {
	return projectAgentIdentityDeletionProviderStub{
		deleteIdentity: func(_ context.Context, request ProjectAgentIdentityDeleteRequest) error {
			f.events = append(f.events, "provider-delete:"+request.Environment)
			f.requests = append(f.requests, request)
			return f.deleteErr[request.Environment]
		},
	}
}

func (f *cpDeleteFixture) service(
	resolver *clientmocks.EnvThunderResolverMock, mode thundersvc.IdentityManagementMode, provider ProjectAgentIdentityProvider,
) AgentThunderProvisioningService {
	f.t.Helper()
	svc := NewAgentThunderProvisioningService(f.repo(), resolver, f.store(), slog.New(slog.NewTextHandler(&f.logs, nil)))
	require.NoError(f.t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(mode, provider))
	return svc
}

func (f *cpDeleteFixture) providerEnvs() []string {
	envs := make([]string, 0, len(f.requests))
	for _, r := range f.requests {
		envs = append(envs, r.Environment)
	}
	return envs
}

func TestDeleteAllBindingsCPManagedDeletesSourceFirstThenPromotedInOrder(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	require.Equal(t, []string{"development", "stage", "production"}, f.providerEnvs())
	for _, r := range f.requests {
		assert.Equal(t, r.Environment == "development", r.Source, "only the source request is flagged Source: %s", r.Environment)
		assert.Equal(t, "org-a", r.OrganizationID)
		assert.Equal(t, "project-a", r.ProjectID)
		assert.Equal(t, "project-a", r.ProjectName)
		assert.Equal(t, "agent-a", r.AgentName)
		assert.Equal(t, "agent-id-"+r.Environment, r.AgentID)
		assert.Equal(t, "client-"+r.Environment, r.ClientID)
	}
	assert.Equal(t, "db-delete", f.events[0], "rows are deleted before any external call")
}

func TestDeleteAllBindingsCPManagedSourceFailureSkipsPromotedDeletions(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	f.deleteErr["development"] = errors.New("control plane unavailable")
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())

	require.NotPanics(t, func() { svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a") })

	assert.Equal(t, []string{"development"}, f.providerEnvs(), "no promoted AgentID is removed while the source one remains")
	assert.Contains(t, f.events, "secret-delete:stage", "secret cleanup still runs for the skipped environments")
	assert.Contains(t, f.events, "secret-delete:production")

	// One line names every AgentID left behind: the source and both skipped ones.
	logs := f.logs.String()
	assert.Equal(t, 1, strings.Count(logs, "need manual cleanup"), "exactly one leftover line per agent")
	assert.Contains(t, logs, "level=ERROR")
	for _, leftover := range []string{"development=agent-id-development", "stage=agent-id-stage", "production=agent-id-production"} {
		assert.Contains(t, logs, leftover)
	}
}

func TestDeleteAllBindingsCPManagedLogsPromotedDeletionFailureAsLeftover(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	f.deleteErr["stage"] = errors.New("stage unavailable")
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	logs := f.logs.String()
	assert.Equal(t, 1, strings.Count(logs, "need manual cleanup"))
	assert.Contains(t, logs, "level=ERROR")
	assert.Contains(t, logs, "stage=agent-id-stage")
}

func TestDeleteAllBindingsCPManagedPromotedFailureDoesNotStopTheRest(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	f.deleteErr["stage"] = errors.New("stage unavailable")
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	assert.Equal(t, []string{"development", "stage", "production"}, f.providerEnvs())
}

func TestDeleteAllBindingsCPManagedWithoutDeletionProviderFailsClosedButCleansSecrets(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	// SetProjectAgentIdentityProvider rejects a provider that cannot delete, so
	// this exercises the defensive branch by clearing the capability afterwards.
	svc.(*agentThunderProvisioningService).projectIdentityDeletionProvider = nil

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	assert.Empty(t, f.requests)
	assert.Empty(t, f.legacyDeleted)
	assert.ElementsMatch(t,
		[]string{"db-delete", "secret-delete:development", "secret-delete:stage", "secret-delete:production"}, f.events,
		"stored credentials are still deleted even though the AgentIDs are left behind")
	assert.Equal(t, 1, strings.Count(f.logs.String(), "no project AgentID deletion provider is configured"))
	assert.Contains(t, f.logs.String(), "level=ERROR")
}

func TestDeleteAllBindingsCPManagedSkipsBindingsWithoutAnAgentID(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	for i := range f.seed {
		if f.seed[i].EnvironmentName == "stage" {
			f.seed[i].ThunderAgentID = ""
			f.seed[i].ThunderClientID = ""
		}
	}
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	assert.Equal(t, []string{"development", "production"}, f.providerEnvs())
	assert.Contains(t, f.events, "secret-delete:stage", "the credential of a binding that never reached the platform is still cleaned up")
}

func TestDeleteAllBindingsCPManagedExternalAgentsUseTheSameProviderPath(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	assert.Equal(t, []string{"development", "stage", "production"}, f.providerEnvs())
	assert.True(t, f.requests[0].Source)
	for _, e := range f.events {
		assert.NotContains(t, e, "secret-delete", "external agents store no secret, so none is deleted")
	}
}

func TestDeleteAllBindingsLegacyDirectIgnoresADeletionProvider(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	svc := f.service(f.legacyResolver(), thundersvc.IdentityManagementLegacyDirect, f.deletionProvider())

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	assert.Empty(t, f.requests, "the provider is never called in LEGACY_DIRECT mode")
	assert.ElementsMatch(t, []string{"agent-id-stage", "agent-id-development", "agent-id-production"}, f.legacyDeleted)
	assert.Equal(t, "db-delete", f.events[0], "rows are deleted before any external call")
}

func TestDeleteAllBindingsLegacyDirectKeepsSnapshotOrder(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	svc := f.service(f.legacyResolver(), thundersvc.IdentityManagementLegacyDirect, nil)

	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")

	assert.Equal(t, []string{"agent-id-stage", "agent-id-development", "agent-id-production"}, f.legacyDeleted)
}
