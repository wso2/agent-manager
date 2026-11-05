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
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// cpManagedProvisioningStub reports CP_MANAGED, the way the real provisioning
// service does once SetProjectAgentIdentityProvider ran.
type cpManagedProvisioningStub struct {
	*provisionForEnvIfMissingStub
}

func (*cpManagedProvisioningStub) IdentityManagementMode() thundersvc.IdentityManagementMode {
	return thundersvc.IdentityManagementCPManaged
}

// recordingProvisioning counts the AgentID provisioning kick-offs a promotion
// makes. It reports no mode, like an implementation without project AgentID
// support.
type recordingProvisioning struct {
	*stubAgentThunderProvisioning
	mu             sync.Mutex
	provisionCalls int
}

func (p *recordingProvisioning) ProvisionForEnvironmentIfMissing(context.Context, string, string, string, string, models.AgentProvisioningType, string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provisionCalls++
	return false, nil
}

func (p *recordingProvisioning) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.provisionCalls
}

type cpManagedRecordingProvisioning struct {
	*recordingProvisioning
}

func (*cpManagedRecordingProvisioning) IdentityManagementMode() thundersvc.IdentityManagementMode {
	return thundersvc.IdentityManagementCPManaged
}

// legacyModeProvisioningStub reports an explicit LEGACY_DIRECT mode.
type legacyModeProvisioningStub struct {
	*provisionForEnvIfMissingStub
}

func (*legacyModeProvisioningStub) IdentityManagementMode() thundersvc.IdentityManagementMode {
	return thundersvc.IdentityManagementLegacyDirect
}

// cpManagedPromoteFixture is promoteAgentTestFixture in CP_MANAGED mode with the
// target's binding in the given state (nil means no row). It returns the
// service, the env overrides PromoteComponent received, the environments
// ReconcileForEnvironment was called for, and a log reader.
func cpManagedPromoteFixture(
	t *testing.T, tgtIdentityEnvVars []client.EnvVar, state *AgentThunderBindingState,
) (s *agentManagerService, overrides func() []client.EnvVar, reconciled func() []string, logs func() string) {
	t.Helper()
	s, _ = promoteAgentTestFixture(t, tgtIdentityEnvVars, nil)
	stubBindingState(t, s, state, nil)
	s.agentThunderProvisioning = &cpManagedProvisioningStub{s.agentThunderProvisioning.(*provisionForEnvIfMissingStub)}
	logs = captureLogs(s)

	var mu sync.Mutex
	var captured []client.EnvVar
	oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	oc.PromoteComponentFunc = func(_ context.Context, _, _, _, _, _ string, envOverrides []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
		mu.Lock()
		defer mu.Unlock()
		captured = append([]client.EnvVar(nil), envOverrides...)
		return nil
	}
	var reconciledEnvs []string
	s.agentIdentityInjection.(*agentIdentityInjectorStub).ReconcileForEnvironmentFunc = func(_ context.Context, _, _, _, envName string) error {
		mu.Lock()
		defer mu.Unlock()
		reconciledEnvs = append(reconciledEnvs, envName)
		return nil
	}
	overrides = func() []client.EnvVar {
		mu.Lock()
		defer mu.Unlock()
		return captured
	}
	reconciled = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return reconciledEnvs
	}
	return s, overrides, reconciled, logs
}

func promoteToStagingForIdentity(t *testing.T, s *agentManagerService) error {
	t.Helper()
	return s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})
}

func sortedIdentityKeys() []string {
	keys := make([]string, 0, len(agentIdentityEnvVarKeySet))
	for key := range agentIdentityEnvVarKeySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// requireOnlyNeutralIdentityOverrides asserts every identity key is overridden
// by an empty plain value, and that nothing of the lowest environment's real
// credential is carried.
func requireOnlyNeutralIdentityOverrides(t *testing.T, overrides []client.EnvVar) {
	t.Helper()
	var keys []string
	for _, override := range overrides {
		if !agentIdentityEnvVarKeySet[override.Key] {
			continue
		}
		keys = append(keys, override.Key)
		assert.Empty(t, override.Value, "%s must be an empty placeholder", override.Key)
		assert.Nil(t, override.ValueFrom, "%s must not reference a secret", override.Key)
		assert.NotEqual(t, "dev-client-id", override.Value, "the lowest environment's credential must never travel")
	}
	sort.Strings(keys)
	require.Equal(t, sortedIdentityKeys(), keys, "every identity key needs its own placeholder, or the missing one is inherited")
}

func TestPromoteAgent_CPManaged_PromotesWithoutTargetIdentityAndNeutralizesTheCredentials(t *testing.T) {
	cases := map[string]struct {
		state      *AgentThunderBindingState
		wantStatus string
	}{
		"binding missing":     {state: nil, wantStatus: "bindingStatus=missing"},
		"binding pending":     {state: &AgentThunderBindingState{Status: models.AgentThunderStatusPending}, wantStatus: "bindingStatus=pending"},
		"binding in progress": {state: &AgentThunderBindingState{Status: models.AgentThunderStatusInProgress}, wantStatus: "bindingStatus=in_progress"},
		"binding failed": {
			state:      &AgentThunderBindingState{Status: models.AgentThunderStatusFailed, LastError: "provider refused"},
			wantStatus: "bindingStatus=failed",
		},
		"completed without secret": {
			state:      &AgentThunderBindingState{Status: models.AgentThunderStatusCompleted, HasSecret: false},
			wantStatus: `bindingStatus="completed without secret"`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, overrides, reconciled, logs := cpManagedPromoteFixture(t, nil, tc.state)

			require.NoError(t, promoteToStagingForIdentity(t, s), "an AgentID problem must never fail the workload promotion")

			captured := overrides()
			require.NotNil(t, captured, "PromoteComponent must have been called")
			requireOnlyNeutralIdentityOverrides(t, captured)
			assert.Equal(t, []string{"staging"}, reconciled(), "a binding that completed meanwhile must be injected right away")

			out := logs()
			assert.Contains(t, out, "level=WARN")
			assert.Contains(t, out, "without the target environment's own AgentID")
			assert.Contains(t, out, "targetEnvironment=staging")
			assert.Contains(t, out, tc.wantStatus)
			assert.Contains(t, out, "agentName=my-agent")
			assert.Contains(t, out, "projectName=proj1")
			assert.Contains(t, out, "ouID=acme")
			assert.NotContains(t, out, "dev-client-id")
		})
	}
}

func TestPromoteAgent_CPManaged_LegacyModeStillBlocksWithTheSameInput(t *testing.T) {
	for name, wrap := range map[string]func(*provisionForEnvIfMissingStub) AgentThunderProvisioningService{
		"explicit LEGACY_DIRECT": func(p *provisionForEnvIfMissingStub) AgentThunderProvisioningService {
			return &legacyModeProvisioningStub{p}
		},
		"a provisioning service that does not report a mode": func(p *provisionForEnvIfMissingStub) AgentThunderProvisioningService {
			return p
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, promoteCalled := promoteAgentTestFixture(t, nil, nil)
			s.agentThunderProvisioning = wrap(s.agentThunderProvisioning.(*provisionForEnvIfMissingStub))

			err := promoteToStagingForIdentity(t, s)

			ve := requireBriefPromotionBlock(t, err)
			assert.Contains(t, ve.Message, "still being provisioned")
			assert.False(t, *promoteCalled)
		})
	}
}

func TestPromoteAgent_CPManaged_LowestEnvironmentWithoutCredentialAddsNoPlaceholders(t *testing.T) {
	s, overrides, reconciled, logs := cpManagedPromoteFixture(t, nil, nil)
	// Nothing was ever injected, in any environment: nothing to neutralize.
	s.agentIdentityInjection.(*agentIdentityInjectorStub).EnvVarsForEnvironmentFunc = func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
		return nil, nil
	}

	require.NoError(t, promoteToStagingForIdentity(t, s))

	for _, override := range overrides() {
		assert.False(t, agentIdentityEnvVarKeySet[override.Key], "unexpected identity override %s", override.Key)
	}
	assert.Empty(t, reconciled())
	assert.NotContains(t, logs(), "without the target environment's own AgentID")
}

func TestPromoteAgent_CPManaged_ReadyTargetIdentityIsInjectedAsBefore(t *testing.T) {
	ready := []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "staging-client-id"}}
	s, overrides, reconciled, _ := cpManagedPromoteFixture(t, ready, nil)

	require.NoError(t, promoteToStagingForIdentity(t, s))

	var identity []client.EnvVar
	for _, override := range overrides() {
		if agentIdentityEnvVarKeySet[override.Key] {
			identity = append(identity, override)
		}
	}
	assert.Equal(t, ready, identity, "a ready identity travels as is, with no placeholders")
	assert.Empty(t, reconciled(), "nothing was neutralized, so there is nothing to reconcile")
}

func TestPromoteAgent_CPManaged_ReconcileFailureDoesNotFailThePromotion(t *testing.T) {
	s, _, _, logs := cpManagedPromoteFixture(t, nil, nil)
	s.agentIdentityInjection.(*agentIdentityInjectorStub).ReconcileForEnvironmentFunc = func(context.Context, string, string, string, string) error {
		return errors.New("openchoreo unavailable")
	}

	require.NoError(t, promoteToStagingForIdentity(t, s))
	assert.Contains(t, logs(), "Failed to reconcile AgentID credentials after promoting with placeholders")
}

func TestPromoteAgent_CPManaged_StateUnreadableStillPromotes(t *testing.T) {
	s, overrides, _, logs := cpManagedPromoteFixture(t, nil, nil)
	stub := s.agentThunderProvisioning.(*cpManagedProvisioningStub)
	stub.GetBindingStateFunc = func(context.Context, string, string, string, string) (*AgentThunderBindingState, error) {
		return nil, errors.New("database unavailable")
	}

	require.NoError(t, promoteToStagingForIdentity(t, s), "the status is only for the log; it must not block")
	requireOnlyNeutralIdentityOverrides(t, overrides())
	assert.Contains(t, logs(), "bindingStatus=unknown")
}

func TestPromoteAgent_CPManaged_PlaceholdersDropTheSourceEnvironmentsCredentialsToo(t *testing.T) {
	// useConfigFromSourceEnv clones the source overrides, which may hold the
	// source's real credentials; they must be stripped and replaced by the
	// placeholders.
	s, overrides, _, _ := cpManagedPromoteFixture(t, nil, nil)
	oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	oc.GetSourceEnvWorkloadOverridesFunc = func(context.Context, string, string, string) ([]client.EnvVar, []client.FileVar, error) {
		return []client.EnvVar{
			{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"},
			{Key: client.EnvVarAgentIDClientSecret, ValueFrom: &client.EnvVarValueFrom{SecretKeyRef: &client.SecretKeyRef{Name: "dev-secret", Key: "client_secret"}}},
			{Key: "USER_SETTING", Value: "kept"},
		}, nil, nil
	}
	useSource := true

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev", TargetEnvironment: "staging", UseConfigFromSourceEnv: &useSource,
	})

	require.NoError(t, err)
	captured := overrides()
	requireOnlyNeutralIdentityOverrides(t, captured)
	var user []client.EnvVar
	for _, override := range captured {
		if override.Key == "USER_SETTING" {
			user = append(user, override)
		}
	}
	assert.Len(t, user, 1)
}

func TestNeutralAgentIdentityEnvVarsCoverEveryIdentityKey(t *testing.T) {
	placeholders := neutralAgentIdentityEnvVars()
	var keys []string
	for _, placeholder := range placeholders {
		keys = append(keys, placeholder.Key)
		assert.Empty(t, placeholder.Value)
		assert.Nil(t, placeholder.ValueFrom)
	}
	assert.Equal(t, sortedIdentityKeys(), keys, "stable order, one placeholder per identity key")
}

func TestIdentityEnvVarsInSyncTreatsPlaceholdersAsMissing(t *testing.T) {
	desired := []client.EnvVar{
		{Key: client.EnvVarAgentIDClientID, Value: "client-abc"},
		{Key: client.EnvVarAgentIDClientSecret, ValueFrom: &client.EnvVarValueFrom{SecretKeyRef: &client.SecretKeyRef{Name: "ref", Key: "client_secret"}}},
		{Key: client.EnvVarAgentIDTokenEndpoint, Value: "http://thunder/token"},
		{Key: client.EnvVarAgentIDScopes, Value: ""},
	}
	real := []models.EnvVars{
		{Key: client.EnvVarAgentIDClientID, Value: "client-abc"},
		{Key: client.EnvVarAgentIDClientSecret, IsSensitive: true, SecretRef: "ref", SecretKey: "client_secret"},
		{Key: client.EnvVarAgentIDTokenEndpoint, Value: "http://thunder/token"},
		{Key: client.EnvVarAgentIDScopes, Value: ""},
	}
	placeholders := []models.EnvVars{
		{Key: client.EnvVarAgentIDClientID},
		{Key: client.EnvVarAgentIDClientSecret},
		{Key: client.EnvVarAgentIDTokenEndpoint},
		{Key: client.EnvVarAgentIDScopes},
	}

	assert.True(t, identityEnvVarsInSync(desired, real), "real credentials present")
	assert.False(t, identityEnvVarsInSync(desired, placeholders), "placeholders must be replaced by the real values")
	for i := range placeholders {
		oneReal := append([]models.EnvVars(nil), placeholders...)
		oneReal[i] = real[i]
		if real[i].Key == client.EnvVarAgentIDScopes {
			// An empty desired scope list is satisfied by an empty placeholder.
			assert.False(t, identityEnvVarsInSync(desired, oneReal), "the other three are still placeholders")
			continue
		}
		assert.False(t, identityEnvVarsInSync(desired, oneReal), "a single placeholder is enough to be out of sync (%s)", real[i].Key)
	}

	placeholderDesired := neutralAgentIdentityEnvVars()
	assert.True(t, identityEnvVarsInSync(placeholderDesired, placeholders), "a placeholder against a placeholder is in sync")
	assert.True(t, identityEnvVarsInSync(placeholderDesired, real), "real values satisfy a placeholder desire")

	scoped := append([]client.EnvVar(nil), desired...)
	scoped[3] = client.EnvVar{Key: client.EnvVarAgentIDScopes, Value: "tickets:read"}
	scopedPlaceholders := append([]models.EnvVars(nil), real...)
	scopedPlaceholders[3] = models.EnvVars{Key: client.EnvVarAgentIDScopes}
	assert.False(t, identityEnvVarsInSync(scoped, scopedPlaceholders), "an empty scope list is not the desired one")
}

// TestPromotionPlaceholdersAreReplacedByTheRealCredentialsOnCompletion proves
// the whole path: PromoteAgent writes placeholders while the target binding is
// pending, the binding then completes, and the completion hook
// (reconcileWorkloadInjection) rewrites the target's ReleaseBinding with the
// real values and rolls the workload, exactly once.
func TestPromotionPlaceholdersAreReplacedByTheRealCredentialsOnCompletion(t *testing.T) {
	// 1. Promote while the target binding is pending. The lowest environment's
	// real credential lives in the shared Workload CR.
	s, overrides, _, _ := cpManagedPromoteFixture(t, nil, &AgentThunderBindingState{Status: models.AgentThunderStatusPending})
	require.NoError(t, promoteToStagingForIdentity(t, s))
	placeholders := overrides()
	requireOnlyNeutralIdentityOverrides(t, placeholders)

	// 2. A fake data plane: the shared Workload holds dev's real credential and
	// the staging ReleaseBinding holds exactly what PromoteComponent wrote.
	workload := []models.EnvVars{
		{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"},
		{Key: client.EnvVarAgentIDClientSecret, IsSensitive: true, SecretRef: "dev-secret", SecretKey: "client_secret"},
		{Key: client.EnvVarAgentIDTokenEndpoint, Value: "http://dev-thunder/token"},
		{Key: client.EnvVarAgentIDScopes, Value: ""},
	}
	releaseBinding := map[string]client.EnvVar{}
	for _, override := range placeholders {
		releaseBinding[override.Key] = override
	}
	var mu sync.Mutex
	rolls := 0
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		mu.Lock()
		defer mu.Unlock()
		merged := map[string]models.EnvVars{}
		for _, ev := range workload {
			merged[ev.Key] = ev
		}
		for key, override := range releaseBinding { // overrides win, as in OpenChoreo
			entry := models.EnvVars{Key: key, Value: override.Value}
			if override.ValueFrom != nil && override.ValueFrom.SecretKeyRef != nil {
				entry.IsSensitive = true
				entry.SecretRef = override.ValueFrom.SecretKeyRef.Name
				entry.SecretKey = override.ValueFrom.SecretKeyRef.Key
			}
			merged[key] = entry
		}
		result := make([]models.EnvVars, 0, len(merged))
		for _, entry := range merged {
			result = append(result, entry)
		}
		return result, nil
	}
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
		mu.Lock()
		defer mu.Unlock()
		rolls++
		for _, envVar := range envVars {
			releaseBinding[envVar.Key] = envVar
		}
		return nil
	}

	// 3. The target binding completes; its completion hook runs.
	binding := completedInternalBinding()
	binding.PromotionSource = "dev"
	injector := newTestIdentityInjectionService(identityRepoReturning(binding, nil), oc)
	reconcileWorkloadInjection(context.Background(), injector, *binding, discardLogger())

	require.Equal(t, 1, rolls, "the placeholders must be replaced, with one rollout")
	assert.Equal(t, "client-abc", releaseBinding[client.EnvVarAgentIDClientID].Value)
	secret := releaseBinding[client.EnvVarAgentIDClientSecret]
	require.NotNil(t, secret.ValueFrom)
	assert.Equal(t, testIdentitySecretRefName(), secret.ValueFrom.SecretKeyRef.Name)
	assert.NotEmpty(t, releaseBinding[client.EnvVarAgentIDTokenEndpoint].Value)
	assert.NotContains(t, releaseBinding[client.EnvVarAgentIDTokenEndpoint].Value, "dev-thunder")

	// 4. The next sweep finds it in sync and does not roll the pod again.
	reconcileWorkloadInjection(context.Background(), injector, *binding, discardLogger())
	assert.Equal(t, 1, rolls, "an already injected workload must not be rolled again")
}

// TestReconcileInjectsOverEmptyPlaceholdersOnly pins the placeholder case on its
// own: a release binding override that is an empty placeholder is never treated
// as a credential, even though its key is present.
func TestReconcileInjectsOverEmptyPlaceholdersOnly(t *testing.T) {
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
		return []models.EnvVars{
			{Key: client.EnvVarAgentIDClientID},
			{Key: client.EnvVarAgentIDClientSecret},
			{Key: client.EnvVarAgentIDTokenEndpoint},
			{Key: client.EnvVarAgentIDScopes},
		}, nil
	}
	var injected []client.EnvVar
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
		injected = envVars
		return nil
	}
	svc := newTestIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil), oc)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	require.Len(t, injected, 4)
	assert.Equal(t, "client-abc", injected[0].Value)
}

// promoteWithRecordedIdentityReads promotes dev to staging with the target's AgentID
// not ready and returns how many times the target's identity vars were read and
// how many AgentID kick-offs were made.
func promoteWithRecordedIdentityReads(t *testing.T, cpManaged bool) (targetReads, provisionCalls int, err error) {
	t.Helper()
	s, _ := promoteAgentTestFixture(t, nil, nil)
	recording := &recordingProvisioning{
		stubAgentThunderProvisioning: s.agentThunderProvisioning.(*provisionForEnvIfMissingStub).stubAgentThunderProvisioning,
	}
	if cpManaged {
		s.agentThunderProvisioning = &cpManagedRecordingProvisioning{recording}
		s.agentIdentityInjection.(*agentIdentityInjectorStub).ReconcileForEnvironmentFunc = func(context.Context, string, string, string, string) error {
			return nil
		}
	} else {
		s.agentThunderProvisioning = recording
	}
	injector := s.agentIdentityInjection.(*agentIdentityInjectorStub)
	inner := injector.EnvVarsForEnvironmentFunc
	var reads int
	var mu sync.Mutex
	injector.EnvVarsForEnvironmentFunc = func(ctx context.Context, ou, project, agent, env string) ([]client.EnvVar, error) {
		if env == "staging" {
			mu.Lock()
			reads++
			mu.Unlock()
		}
		return inner(ctx, ou, project, agent, env)
	}

	err = promoteToStagingForIdentity(t, s)

	mu.Lock()
	defer mu.Unlock()
	return reads, recording.calls(), err
}

func TestPromoteAgent_CPManaged_NeverTriggersOrWaitsForAgentIDPromotion(t *testing.T) {
	reads, provisionCalls, err := promoteWithRecordedIdentityReads(t, true)

	require.NoError(t, err)
	assert.Zero(t, provisionCalls, "workload promotion must not start AgentID provisioning")
	assert.Equal(t, 1, reads, "the target identity is read once, with no polling for it to appear")
}

func TestPromoteAgent_LegacyDirectStillTriggersAndPollsForTheTargetIdentity(t *testing.T) {
	reads, provisionCalls, err := promoteWithRecordedIdentityReads(t, false)

	require.Error(t, err, "LEGACY_DIRECT still hard blocks while the identity is not ready")
	assert.Equal(t, 1, provisionCalls, "the kick-off is unchanged")
	assert.Greater(t, reads, 1, "the poll is unchanged")
}

// ---- other writers of a target environment's env overrides ----

type updateConfigRun struct {
	oc         *clientmocks.OpenChoreoClientMock
	replaced   [][]client.EnvVar
	pipelineN  int
	logs       func() string
	updateFunc func(*spec.UpdateAgentConfigurationsRequest) error
}

// updateConfigFixture builds an agentManagerService for UpdateAgentConfigurations
// of staging in a dev -> staging pipeline. dev holds a real credential unless
// lowestHasCredential is false; staging holds stagingVars. mode is nil for a
// provisioning service that reports no mode (LEGACY_DIRECT).
func updateConfigFixture(t *testing.T, cpManaged, lowestHasCredential bool, stagingVars []client.EnvVar) *updateConfigRun {
	t.Helper()
	run := &updateConfigRun{}
	var mu sync.Mutex
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return nil, nil
		},
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			mu.Lock()
			defer mu.Unlock()
			run.pipelineN++
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		ReplaceReleaseBindingWorkloadOverridesFunc: func(_ context.Context, _, _, _ string, envOverrides []client.EnvVar, _ []client.FileVar) error {
			mu.Lock()
			defer mu.Unlock()
			run.replaced = append(run.replaced, envOverrides)
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, env string) ([]client.EnvVar, error) {
			switch {
			case env == "dev" && lowestHasCredential:
				return []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"}}, nil
			case env == "staging":
				return stagingVars, nil
			}
			return nil, nil
		},
	}
	run.oc = oc
	s := &agentManagerService{ocClient: oc, agentIdentityInjection: injector, logger: discardLogger()}
	if cpManaged {
		s.agentThunderProvisioning = &cpManagedProvisioningStub{&provisionForEnvIfMissingStub{&stubAgentThunderProvisioning{}}}
	}
	run.logs = captureLogs(s)
	run.updateFunc = func(req *spec.UpdateAgentConfigurationsRequest) error {
		return s.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", "my-agent", req)
	}
	return run
}

func userEnvRequest(env string) *spec.UpdateAgentConfigurationsRequest {
	return &spec.UpdateAgentConfigurationsRequest{
		EnvironmentName: env,
		Env:             []spec.EnvironmentVariable{{Key: "USER_SETTING", Value: ptrOf("kept")}},
	}
}

func ptrOf[T any](v T) *T { return &v }

func nonIdentity(vars []client.EnvVar) []client.EnvVar {
	var rest []client.EnvVar
	for _, v := range vars {
		if !agentIdentityEnvVarKeySet[v.Key] {
			rest = append(rest, v)
		}
	}
	return rest
}

func TestUpdateAgentConfigurations_CPManaged_KeepsPlaceholdersWhileTheIdentityIsNotReady(t *testing.T) {
	run := updateConfigFixture(t, true, true, nil)

	require.NoError(t, run.updateFunc(userEnvRequest("staging")))

	require.Len(t, run.replaced, 1)
	requireOnlyNeutralIdentityOverrides(t, run.replaced[0])
	require.Len(t, nonIdentity(run.replaced[0]), 1, "the user's variable is still written")
	assert.Contains(t, run.logs(), "Kept empty AgentID placeholders")
	assert.NotContains(t, run.logs(), "dev-client-id")
}

func TestUpdateAgentConfigurations_CPManaged_ReadyIdentityIsWrittenWithoutPlaceholders(t *testing.T) {
	ready := []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "staging-client-id"}}
	run := updateConfigFixture(t, true, true, ready)

	require.NoError(t, run.updateFunc(userEnvRequest("staging")))

	require.Len(t, run.replaced, 1)
	var identity []client.EnvVar
	for _, v := range run.replaced[0] {
		if agentIdentityEnvVarKeySet[v.Key] {
			identity = append(identity, v)
		}
	}
	assert.Equal(t, ready, identity)
	assert.Zero(t, run.pipelineN, "a ready identity needs no pipeline lookup")
}

func TestUpdateAgentConfigurations_LegacyDirectIsUnchanged(t *testing.T) {
	run := updateConfigFixture(t, false, true, nil)

	require.NoError(t, run.updateFunc(userEnvRequest("staging")))

	require.Len(t, run.replaced, 1)
	assert.Empty(t, run.replaced[0][len(nonIdentity(run.replaced[0])):], "no identity variables are added")
	assert.Equal(t, nonIdentity(run.replaced[0]), run.replaced[0])
	assert.Zero(t, run.pipelineN, "LEGACY_DIRECT does not even read the pipeline")
}

func TestUpdateAgentConfigurations_CPManaged_NothingToNeutralize(t *testing.T) {
	t.Run("the lowest environment holds no credential", func(t *testing.T) {
		run := updateConfigFixture(t, true, false, nil)
		require.NoError(t, run.updateFunc(userEnvRequest("staging")))
		require.Len(t, run.replaced, 1)
		assert.Equal(t, nonIdentity(run.replaced[0]), run.replaced[0])
	})
	t.Run("the lowest environment writes its own ready credential, never placeholders", func(t *testing.T) {
		run := updateConfigFixture(t, true, true, nil)
		require.NoError(t, run.updateFunc(userEnvRequest("dev")))
		require.Len(t, run.replaced, 1)
		var identity []client.EnvVar
		for _, v := range run.replaced[0] {
			if agentIdentityEnvVarKeySet[v.Key] {
				identity = append(identity, v)
			}
		}
		assert.Equal(t, []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"}}, identity)
		assert.Zero(t, run.pipelineN)
	})
	t.Run("a files-only update leaves the env overrides alone and needs no pipeline", func(t *testing.T) {
		run := updateConfigFixture(t, true, true, nil)
		run.oc.GetProjectDeploymentPipelineFunc = func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			run.pipelineN++
			return nil, errors.New("openchoreo unavailable")
		}
		require.NoError(t, run.updateFunc(&spec.UpdateAgentConfigurationsRequest{
			EnvironmentName: "staging", Files: []spec.FileMount{},
		}))
		require.Len(t, run.replaced, 1)
		assert.Nil(t, run.replaced[0], "nil means leave the existing env overrides, placeholders included")
		assert.Zero(t, run.pipelineN, "a files-only request must not look up the pipeline")
	})
}

func TestUpdateAgentConfigurations_CPManaged_PipelineErrorAbortsBeforeReplacing(t *testing.T) {
	run := updateConfigFixture(t, true, true, nil)
	run.oc.GetProjectDeploymentPipelineFunc = func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
		return nil, errors.New("openchoreo unavailable")
	}

	err := run.updateFunc(userEnvRequest("staging"))

	require.Error(t, err)
	assert.Empty(t, run.replaced, "without knowing the lowest environment the update must not risk dropping the placeholders")
}

// TestUpdateConfigurationPlaceholdersAreReplacedByTheRealCredentialsOnCompletion
// proves the placeholder to real path for the configuration-update writer.
func TestUpdateConfigurationPlaceholdersAreReplacedByTheRealCredentialsOnCompletion(t *testing.T) {
	run := updateConfigFixture(t, true, true, nil)
	require.NoError(t, run.updateFunc(userEnvRequest("staging")))
	require.Len(t, run.replaced, 1)
	requireOnlyNeutralIdentityOverrides(t, run.replaced[0])

	releaseBinding := map[string]client.EnvVar{}
	for _, override := range run.replaced[0] {
		releaseBinding[override.Key] = override
	}
	var mu sync.Mutex
	rolls := 0
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
		mu.Lock()
		defer mu.Unlock()
		// The shared Workload still holds dev's credential; overrides win.
		merged := map[string]models.EnvVars{
			client.EnvVarAgentIDClientID: {Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"},
		}
		for key, override := range releaseBinding {
			entry := models.EnvVars{Key: key, Value: override.Value}
			if override.ValueFrom != nil && override.ValueFrom.SecretKeyRef != nil {
				entry.IsSensitive, entry.SecretRef = true, override.ValueFrom.SecretKeyRef.Name
				entry.SecretKey = override.ValueFrom.SecretKeyRef.Key
			}
			merged[key] = entry
		}
		out := make([]models.EnvVars, 0, len(merged))
		for _, entry := range merged {
			out = append(out, entry)
		}
		return out, nil
	}
	oc.UpdateReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
		mu.Lock()
		defer mu.Unlock()
		rolls++
		for _, envVar := range envVars {
			releaseBinding[envVar.Key] = envVar
		}
		return nil
	}
	binding := completedInternalBinding()
	binding.PromotionSource = "dev"
	injector := newTestIdentityInjectionService(identityRepoReturning(binding, nil), oc)

	reconcileWorkloadInjection(context.Background(), injector, *binding, discardLogger())

	require.Equal(t, 1, rolls)
	assert.Equal(t, "client-abc", releaseBinding[client.EnvVarAgentIDClientID].Value)
	assert.NotNil(t, releaseBinding[client.EnvVarAgentIDClientSecret].ValueFrom)
	assert.Equal(t, "kept", *userValue(run.replaced[0]), "the user's own variable is untouched by the injection")
}

func userValue(vars []client.EnvVar) *string {
	for _, v := range vars {
		if v.Key == "USER_SETTING" {
			value := v.Value
			return &value
		}
	}
	return nil
}

// ---- revoke ----

func revokeFixture(t *testing.T, cpManaged, lowestHasCredential bool) (s *agentManagerService, updates func() [][]client.EnvVar, order func() []string) {
	t.Helper()
	var mu sync.Mutex
	var updated [][]client.EnvVar
	var steps []string
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, env string, envVars []client.EnvVar) error {
			mu.Lock()
			defer mu.Unlock()
			steps = append(steps, "placeholders:"+env)
			updated = append(updated, envVars)
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		RemoveForEnvironmentFunc: func(_ context.Context, _, _, _, env string, _ bool) error {
			mu.Lock()
			defer mu.Unlock()
			steps = append(steps, "remove:"+env)
			return nil
		},
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, env string) ([]client.EnvVar, error) {
			if env == "dev" && lowestHasCredential {
				return []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "dev-client-id"}}, nil
			}
			return nil, nil
		},
	}
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(context.Context, string, string, string, string) (string, error) { return "client-abc", nil },
	}
	var provisioning AgentThunderProvisioningService = stub
	if cpManaged {
		provisioning = &cpManagedProvisioningStub{&provisionForEnvIfMissingStub{stub}}
	}
	s = &agentManagerService{agentThunderProvisioning: provisioning, agentIdentityInjection: injector, ocClient: oc, logger: discardLogger()}
	updates = func() [][]client.EnvVar {
		mu.Lock()
		defer mu.Unlock()
		return updated
	}
	order = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return steps
	}
	return s, updates, order
}

func TestRevokeAgentIdentitySecret_CPManagedNonLowestKeepsPlaceholders(t *testing.T) {
	s, updates, order := revokeFixture(t, true, true)

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Empty(t, resp.WorkloadRefreshWarning)
	assert.Equal(t, []string{"remove:staging", "placeholders:staging"}, order(), "placeholders are written after the removal")
	require.Len(t, updates(), 1)
	requireOnlyNeutralIdentityOverrides(t, updates()[0])
}

func TestRevokeAgentIdentitySecret_PlaceholderCasesThatStayUnchanged(t *testing.T) {
	cases := map[string]struct {
		cpManaged, lowestHasCredential bool
		env                            string
	}{
		"LEGACY_DIRECT non-lowest":   {false, true, "staging"},
		"CP_MANAGED lowest":          {true, true, "dev"},
		"CP_MANAGED lowest has none": {true, false, "staging"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, updates, _ := revokeFixture(t, tc.cpManaged, tc.lowestHasCredential)
			_, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", tc.env)
			require.NoError(t, err)
			assert.Empty(t, updates())
		})
	}
}

func TestRevokeAgentIdentitySecret_PlaceholderWriteFailureIsWarnedNotFailed(t *testing.T) {
	s, _, _ := revokeFixture(t, true, true)
	s.ocClient.(*clientmocks.OpenChoreoClientMock).UpdateReleaseBindingEnvVarsFunc = func(context.Context, string, string, string, string, []client.EnvVar) error {
		return utils.ErrNotFound
	}

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err, "the revoke itself succeeded")
	assert.NotEmpty(t, resp.WorkloadRefreshWarning)
}
