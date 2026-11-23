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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// projectAgentIdentityDeletionProviderStub is a provider that can also delete AgentIDs.
type projectAgentIdentityDeletionProviderStub struct {
	projectAgentIdentityProviderStub
	deleteIdentity func(context.Context, ProjectAgentIdentityDeleteRequest) error
	presence       func(context.Context, ProjectAgentIdentityDeleteRequest) (bool, error)
}

func (s projectAgentIdentityDeletionProviderStub) DeleteAgentIdentity(ctx context.Context, request ProjectAgentIdentityDeleteRequest) error {
	return s.deleteIdentity(ctx, request)
}

func (s projectAgentIdentityDeletionProviderStub) AgentIdentityExists(ctx context.Context, request ProjectAgentIdentityDeleteRequest) (bool, error) {
	if s.presence == nil {
		return true, nil
	}
	return s.presence(ctx, request)
}

// cpDeleteFixture wires a provisioning service over three bindings of one agent
// (source "development", promoted "stage" and "production") and records every
// external call in order.
type cpDeleteFixture struct {
	t              *testing.T
	events         []string
	requests       []ProjectAgentIdentityDeleteRequest
	deleteErr      map[string]error
	secretErr      map[string]error
	outsidePresent map[string]bool
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
		secretErr: map[string]error{},
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
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		UpdateAfterAttemptFunc: func(_ context.Context, id uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			for i := range f.seed {
				if f.seed[i].ID == id {
					f.seed[i].Status = fields.Status
				}
			}
			return nil
		},
		UpdateSecretRefFunc: func(_ context.Context, id uuid.UUID, value string) error {
			for i := range f.seed {
				if f.seed[i].ID == id {
					f.seed[i].SecretRefPath = value
				}
			}
			return nil
		},
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
			return f.secretErr[location.EnvironmentName]
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
		presence: func(_ context.Context, request ProjectAgentIdentityDeleteRequest) (bool, error) {
			value, ok := f.outsidePresent[request.Environment]
			return !ok || value, nil
		},
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
	svc := NewAgentThunderProvisioningService(f.repo(), resolver, f.store(), nil, slog.New(slog.NewTextHandler(&f.logs, nil)))
	require.NoError(f.t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(mode, provider))
	if mode == thundersvc.IdentityManagementCPManaged {
		svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "development", []ProjectIdentityEnvironment{{Name: "development", PromotesTo: []string{"stage"}}, {Name: "stage", PromotesTo: []string{"production"}}, {Name: "production"}}, nil
		}))
	}
	return svc
}

// renameSeedEnvironment renames the recorded binding of one environment, as if the environment had
// since been removed from the pipeline.
func (f *cpDeleteFixture) renameSeedEnvironment(from, to string) {
	f.t.Helper()
	for i := range f.seed {
		if f.seed[i].EnvironmentName == from {
			f.seed[i].EnvironmentName = to
			return
		}
	}
	f.t.Fatalf("no seeded binding for environment %q", from)
}

func (f *cpDeleteFixture) providerEnvs() []string {
	envs := make([]string, 0, len(f.requests))
	for _, r := range f.requests {
		envs = append(envs, r.Environment)
	}
	return envs
}

func TestDeleteAllBindingsCPManagedDeletesEveryEnvironmentBeforeBindings(t *testing.T) {
	for _, ownership := range []models.AgentProvisioningType{models.AgentProvisioningTypeInternal, models.AgentProvisioningTypeExternal} {
		t.Run(string(ownership), func(t *testing.T) {
			f := newCPDeleteFixture(t, ownership)
			svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
			svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
			require.Len(t, f.requests, 3)
			require.Equal(t, "development", f.requests[0].Environment)
			require.ElementsMatch(t, []string{"development", "stage", "production"}, f.providerEnvs())
			for _, r := range f.requests {
				assert.Equal(t, r.Environment == "development", r.Source)
				if r.Environment == "stage" {
					require.Equal(t, "development", r.SourceEnvironment)
				}
				if r.Environment == "production" {
					require.Equal(t, "stage", r.SourceEnvironment)
				}
				assert.Equal(t, "org-a", r.OrganizationID)
				assert.Equal(t, "project-a", r.ProjectID)
				assert.Equal(t, "project-a", r.ProjectName)
				assert.Equal(t, "agent-a", r.AgentName)
				assert.Equal(t, "agent-id-"+r.Environment, r.AgentID)
			}
			assert.Equal(t, "db-delete", f.events[len(f.events)-1])
			if ownership == models.AgentProvisioningTypeExternal {
				require.Len(t, f.events, 4)
			}
		})
	}
}

func TestDeleteAllBindingsCPManagedRetainsRecordsOnAnyEnvironmentFailure(t *testing.T) {
	for _, env := range []string{"development", "stage", "production"} {
		t.Run(env, func(t *testing.T) {
			f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
			f.deleteErr[env] = errors.New("environment unavailable")
			svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
			svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
			switch env {
			case "development":
				require.Equal(t, []string{"development"}, f.providerEnvs(), "promoted deletion requires successful source removal")
			case "stage":
				require.Equal(t, []string{"development", "stage"}, f.providerEnvs(), "a child must wait for its predecessor's removal")
			default:
				require.Equal(t, []string{"development", "stage", "production"}, f.providerEnvs())
			}
			require.NotContains(t, f.events, "db-delete")
			require.NotContains(t, f.events, "secret-delete:development")
			require.Contains(t, f.logs.String(), "retaining agent identity bindings for retry")
		})
	}
}

func TestDeleteAllBindingsCPManagedRecoversUnrecordedPromotedID(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	for i := range f.seed {
		if f.seed[i].EnvironmentName == "stage" {
			f.seed[i].ThunderAgentID = ""
		}
	}
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
	for _, request := range f.requests {
		if request.Environment == "stage" {
			require.Equal(t, "agent-id-development", request.AgentID)
			require.False(t, request.Source)
		}
	}
	require.Len(t, f.requests, 3)
}

func TestDeleteAgentWithIdentityCleanupKeepsComponentUntilAllEnvironmentsSucceed(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()).(ProjectAgentIdentityCleanup)
	f.deleteErr["stage"] = errors.New("stage unavailable")
	componentDeletes := 0
	removeComponent := func(context.Context) error {
		componentDeletes++
		f.events = append(f.events, "component-delete")
		return nil
	}
	handled, _, err := svc.DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", removeComponent)
	require.True(t, handled)
	require.Error(t, err)
	require.Zero(t, componentDeletes)
	require.NotContains(t, f.events, "db-delete")
	delete(f.deleteErr, "stage")
	handled, _, err = svc.DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", removeComponent)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, 1, componentDeletes)
	require.Equal(t, "db-delete", f.events[len(f.events)-1])
	require.Greater(t, len(f.events), 7)
}

func TestDeleteAgentWithIdentityCleanupRetainsBindingsWhenComponentDeleteFails(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()).(ProjectAgentIdentityCleanup)
	_, _, err := svc.DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", func(context.Context) error { return errors.New("component unavailable") })
	require.ErrorContains(t, err, "component unavailable")
	require.NotContains(t, f.events, "db-delete")
}

func TestDeleteAgentWithIdentityCleanupLegacyDoesNotInterceptDeletion(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	svc := f.service(f.legacyResolver(), thundersvc.IdentityManagementLegacyDirect, nil).(ProjectAgentIdentityCleanup)
	handled, _, err := svc.DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", func(context.Context) error { t.Fatal("legacy deletion must remain in the caller"); return nil })
	require.NoError(t, err)
	require.False(t, handled)
	require.Empty(t, f.events)
}

func TestDeleteAllBindingsCPManagedRequiresDeletionProvider(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	svc.(*agentThunderProvisioningService).projectIdentityDeletionProvider = nil
	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
	require.Empty(t, f.events)
	require.Contains(t, f.logs.String(), "project AgentID deletion provider is not configured")
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

func TestDeleteAllBindingsCPManagedIncludesPipelineEnvironmentsWithoutBindings(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	f.seed = f.seed[1:2]
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
		return "development", []ProjectIdentityEnvironment{{Name: "development", PromotesTo: []string{"stage"}}, {Name: "stage", PromotesTo: []string{"production"}}, {Name: "production"}}, nil
	}))
	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
	require.Equal(t, []string{"development", "stage", "production"}, f.providerEnvs())
	for _, request := range f.requests {
		require.Equal(t, "agent-id-development", request.AgentID)
	}
}

func TestDeleteAllBindingsCPManagedPipelineFailureDoesNotPartiallyDelete(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(sourceOnlyReader("", errors.New("pipeline unavailable")))
	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
	require.Empty(t, f.events)
	require.Contains(t, f.logs.String(), "pipeline unavailable")
}

type deletionAgentKindStub struct{ AgentKindService }

func (deletionAgentKindStub) HasKindsSourcedFrom(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func TestDeleteAgentCPManagedReturnsFailureWithoutDeletingComponent(t *testing.T) {
	shrinkIdentityDeleteRetryDelay(t)
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	f.deleteErr["stage"] = errors.New("stage unavailable")
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(context.Context, string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{}, nil
		},
		GetWorkloadSecretRefNamesFunc: func(context.Context, string, string, string) ([]string, error) { return nil, nil },
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{}, nil
		},
		DeleteComponentFunc: func(context.Context, string, string, string) error {
			t.Fatal("component must remain when identity cleanup fails")
			return nil
		},
	}
	manager := &agentManagerService{
		ocClient:                 oc,
		agentKindService:         deletionAgentKindStub{},
		logger:                   discardLogger(),
		agentThunderProvisioning: f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()),
	}
	_, err := manager.DeleteAgent(auditableCtx(t), "org-a", "project-a", "agent-a")
	require.ErrorIs(t, err, utils.ErrServiceUnavailable)
	require.Empty(t, oc.DeleteComponentCalls())
	require.NotContains(t, f.events, "db-delete")
}

func TestDeleteAllBindingsCPManagedWarnsAboutEnvironmentRemovedFromPipeline(t *testing.T) {
	for _, ownership := range []models.AgentProvisioningType{models.AgentProvisioningTypeInternal, models.AgentProvisioningTypeExternal} {
		t.Run(string(ownership), func(t *testing.T) {
			f := newCPDeleteFixture(t, ownership)
			for i := range f.seed {
				switch f.seed[i].EnvironmentName {
				case "development":
					f.seed[i].EnvironmentName = "x"
				case "stage":
					f.seed[i].EnvironmentName = "y"
					f.seed[i].PromotionSource = "x"
				case "production":
					f.seed[i].EnvironmentName = "z"
					f.seed[i].PromotionSource = "y"
				}
			}
			svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
			svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
				return "x", []ProjectIdentityEnvironment{{Name: "x", PromotesTo: []string{"z"}}, {Name: "z"}}, nil
			}))
			concrete := svc.(*agentThunderProvisioningService)
			steps, err := concrete.projectPromotionSteps(context.Background(), "org-a", "project-a")
			require.NoError(t, err)
			require.Equal(t, []promotionStep{{Environment: "x"}, {Environment: "z", Source: "x"}}, steps)
			_, err = concrete.writePromotionBindings(context.Background(), repositories.AgentRef{
				OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", ProvisioningType: ownership,
			}, steps, "")
			require.NoError(t, err)
			require.NotContains(t, f.events, "db-delete", "pipeline reconciliation must retain the old y binding")

			componentDeletes := 0
			handled, outside, err := concrete.DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", func(context.Context) error {
				componentDeletes++
				f.events = append(f.events, "component-delete")
				return nil
			})
			require.NoError(t, err)
			require.True(t, handled)
			require.Equal(t, []string{"y"}, outside, "the removed environment is reported, not blocked on")
			require.Equal(t, []string{"x", "z"}, f.providerEnvs(), "the pipeline is cleaned up first")
			require.True(t, f.requests[0].Source)
			require.Equal(t, "x", f.requests[1].SourceEnvironment, "use the current predecessor, not the removed environment")
			require.Equal(t, 1, componentDeletes)
			require.Equal(t, "db-delete", f.events[len(f.events)-1])
		})
	}
}

func TestDeleteAllBindingsCPManagedSkipsOutsideEnvironmentConfirmedGone(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	f.renameSeedEnvironment("stage", "old-stage")
	f.outsidePresent = map[string]bool{"old-stage": false}
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	_, outside, err := svc.(ProjectAgentIdentityCleanup).DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", nil)
	require.NoError(t, err)
	require.Empty(t, outside)
	require.Equal(t, []string{"development", "stage", "production"}, f.providerEnvs())
}

func TestDeleteAllBindingsCPManagedContinuesCredentialCleanupAfterFailure(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
	f.secretErr["development"] = errors.New("secret store unavailable")
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
	for _, env := range []string{"development", "stage", "production"} {
		require.Contains(t, f.events, "secret-delete:"+env)
	}
	require.NotContains(t, f.events, "db-delete")
	for _, binding := range f.seed {
		require.Equal(t, models.AgentThunderStatusFailed, binding.Status)
		if binding.EnvironmentName == "development" {
			require.Equal(t, "ref-development", binding.SecretRefPath)
		}
	}
	delete(f.secretErr, "development")
	svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
	require.Equal(t, "db-delete", f.events[len(f.events)-1])
}

func TestDeleteAllBindingsCPManagedRetainsLegacyMarkerUntilCleanupSucceeds(t *testing.T) {
	for _, failSecret := range []bool{true, false} {
		t.Run(fmt.Sprint(failSecret), func(t *testing.T) {
			f := newCPDeleteFixture(t, models.AgentProvisioningTypeInternal)
			for i := range f.seed {
				f.seed[i].SecretRefPath = "secret/data/legacy/" + f.seed[i].EnvironmentName
			}
			if failSecret {
				f.secretErr["development"] = errors.New("delete credential failed")
			}
			failLegacy := !failSecret
			legacyCalls := []string{}
			svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
			svc.(WorkloadInjectorSetter).SetWorkloadInjector(&agentIdentityInjectorStub{
				CleanupForEnvironmentFunc: func(_ context.Context, _, _, env string) error {
					legacyCalls = append(legacyCalls, env)
					if env == "development" && failLegacy {
						return errors.New("delete reference failed")
					}
					return nil
				},
			})
			svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
			require.NotContains(t, f.events, "db-delete")
			for _, binding := range f.seed {
				require.Equal(t, models.AgentThunderStatusFailed, binding.Status, "failed bindings are not injectable or automatically provisioned")
				injection := &agentIdentityInjectionService{repo: svc.(*agentThunderProvisioningService).repo}
				injectable, injectErr := injection.injectableBinding(context.Background(), "org-a", "project-a", "agent-a", binding.EnvironmentName)
				require.NoError(t, injectErr)
				require.Nil(t, injectable, "retaining the legacy marker must not allow credential reinjection")
				if binding.EnvironmentName == "development" {
					require.True(t, isLegacySecretRefPath(binding.SecretRefPath))
				}
			}
			delete(f.secretErr, "development")
			failLegacy = false
			svc.DeleteAllBindings(context.Background(), "org-a", "project-a", "agent-a")
			require.Contains(t, legacyCalls, "development")
			require.Equal(t, "db-delete", f.events[len(f.events)-1])
		})
	}
}

func TestDeleteAgentCPManagedWaitsForSourcePromotionBindingInsert(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	source := f.seed[1]
	f.seed = []models.AgentThunderClient{source}
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()).(*agentThunderProvisioningService)
	repo := svc.repo.(*repomocks.AgentThunderClientRepositoryMock)
	repo.UpsertFunc = func(_ context.Context, binding *models.AgentThunderClient) error {
		binding.ID = uuid.New()
		f.seed = append(f.seed, *binding)
		return nil
	}
	repo.DeleteByIDsFunc = func(_ context.Context, ids []uuid.UUID) error {
		require.Len(t, ids, 3, "source attempt's newly inserted targets must also be deleted")
		f.seed = nil
		return nil
	}
	release, err := svc.lockAgentIdentity(context.Background(), "org-a", "project-a", "agent-a")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, _, deleteErr := svc.DeleteAgentWithIdentityCleanup(ctx, "org-a", "project-a", "agent-a", func(context.Context) error { return nil })
		done <- deleteErr
	}()
	<-started
	require.Empty(t, repo.FindByAgentCalls(), "deletion must take the agent lock before reading the binding snapshot")
	// The source attempt inserts promotion rows while holding this same lifecycle lock.
	created, err := svc.ensureProjectPromotionBindingsLocked(ctx, repositories.AgentRef{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", ProvisioningType: models.AgentProvisioningTypeExternal}, "")
	require.NoError(t, err)
	release()
	require.NoError(t, <-done)
	require.ElementsMatch(t, []string{"development", "stage", "production"}, f.providerEnvs())
	require.Empty(t, f.seed)
	repo.ClaimForAttemptFunc = func(context.Context, uuid.UUID) (bool, error) { return false, nil }
	require.Len(t, created, 2)
	svc.AttemptProvision(ctx, created[0])
	require.Len(t, repo.ClaimForAttemptCalls(), 1, "a queued target attempt cannot claim a deleted binding")
	require.Len(t, f.requests, 3)
}

func TestProjectPromotionBindingsDoNotRecreateDeletedSource(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()).(*agentThunderProvisioningService)
	f.seed = nil
	created, err := svc.writePromotionBindings(context.Background(), repositories.AgentRef{
		OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "development",
	}, []promotionStep{{Environment: "development"}, {Environment: "stage", Source: "development"}}, "")
	require.NoError(t, err)
	require.Empty(t, created)
	require.Empty(t, svc.repo.(*repomocks.AgentThunderClientRepositoryMock).UpsertCalls())
}

func TestDeleteAllBindingsCPManagedContinuesIndependentPipelineBranches(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider())
	svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
		return "development", []ProjectIdentityEnvironment{{Name: "development", PromotesTo: []string{"stage", "production"}}, {Name: "stage"}, {Name: "production"}}, nil
	}))
	f.deleteErr["stage"] = errors.New("stage unavailable")
	_, _, err := svc.(ProjectAgentIdentityCleanup).DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", nil)
	require.Error(t, err)
	require.ElementsMatch(t, []string{"development", "stage", "production"}, f.providerEnvs())
	for _, request := range f.requests {
		if !request.Source {
			require.Equal(t, "development", request.SourceEnvironment)
		}
	}
	require.Len(t, f.seed, 3)
}

func TestDeleteAllBindingsCPManagedReportsOutsideEnvironmentItCannotInspect(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	f.renameSeedEnvironment("stage", "old-stage")
	provider := projectAgentIdentityDeletionProviderStub{
		presence: func(context.Context, ProjectAgentIdentityDeleteRequest) (bool, error) {
			return false, errors.New("inspection unavailable")
		},
		deleteIdentity: func(_ context.Context, request ProjectAgentIdentityDeleteRequest) error {
			f.requests = append(f.requests, request)
			return nil
		},
	}
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, provider)
	_, outside, err := svc.(ProjectAgentIdentityCleanup).DeleteAgentWithIdentityCleanup(context.Background(), "org-a", "project-a", "agent-a", nil)
	require.NoError(t, err, "an unanswered check must not block deleting the agent")
	require.Equal(t, []string{"old-stage"}, outside, "an unconfirmed environment is reported for manual cleanup")
	require.Equal(t, []string{"development", "stage", "production"}, f.providerEnvs())
}
