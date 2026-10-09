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
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// plainAgentIdentityKeys / balAgentIdentityKeys are the two AgentID env var
// name sets: the default, and the one a Ballerina agent receives instead when
// AgentIDAsBalConfigurables is enabled for an environment.
func plainAgentIdentityKeys() []string {
	return []string{
		client.EnvVarAgentIDClientID,
		client.EnvVarAgentIDClientSecret,
		client.EnvVarAgentIDTokenEndpoint,
		client.EnvVarAgentIDScopes,
	}
}

func balAgentIdentityKeys() []string {
	return []string{
		client.BalConfigVarAgentIDClientID,
		client.BalConfigVarAgentIDClientSecret,
		client.BalConfigVarAgentIDTokenEndpoint,
		client.BalConfigVarAgentIDScopes,
	}
}

func agentIdentityKeysOf(vars []client.EnvVar) []string {
	keys := make([]string, 0, len(vars))
	for _, ev := range vars {
		keys = append(keys, ev.Key)
	}
	return keys
}

// newBalConfigurablesIdentityService builds the injection service with the
// environment's saved AgentIDAsBalConfigurables set to asBal.
func newBalConfigurablesIdentityService(oc *clientmocks.OpenChoreoClientMock, asBal bool) AgentIdentityInjectionService {
	return NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil), noMCPConfigRepo(),
		envConfigRepoReturning(&models.AgentConfig{AgentIDAsBalConfigurables: asBal}), noMCPProxyScopeRepo(), oc, "1h", discardLogger())
}

func TestAgentIdentityEnvVarsAs_RenamesBothWaysKeepingValues(t *testing.T) {
	secretRef := &client.EnvVarValueFrom{SecretKeyRef: &client.SecretKeyRef{Name: "ref", Key: "k"}}
	plain := []client.EnvVar{
		{Key: client.EnvVarAgentIDClientID, Value: "client-abc"},
		{Key: client.EnvVarAgentIDClientSecret, ValueFrom: secretRef},
	}

	bal := agentIdentityEnvVarsAs(plain, true)
	assert.Equal(t, []string{client.BalConfigVarAgentIDClientID, client.BalConfigVarAgentIDClientSecret}, agentIdentityKeysOf(bal))
	assert.Equal(t, "client-abc", bal[0].Value)
	assert.Same(t, secretRef, bal[1].ValueFrom, "the client secret must keep pointing at the same secret")
	assert.Equal(t, []string{client.BalConfigVarAgentIDClientID, client.BalConfigVarAgentIDClientSecret}, agentIdentityKeysOf(agentIdentityEnvVarsAs(bal, true)),
		"already-prefixed names must not be prefixed twice")

	assert.Equal(t, agentIdentityKeysOf(plain), agentIdentityKeysOf(agentIdentityEnvVarsAs(bal, false)))
	assert.Equal(t, client.EnvVarAgentIDClientID, plain[0].Key, "the input slice must not be mutated")
	assert.Nil(t, agentIdentityEnvVarsAs(nil, true), "nothing to inject stays nothing to inject")
}

func TestOtherAgentIdentityEnvVarKeys_IsTheOtherNameSet(t *testing.T) {
	plain := agentIdentityEnvVarsAs([]client.EnvVar{
		{Key: client.EnvVarAgentIDClientID},
		{Key: client.EnvVarAgentIDClientSecret},
		{Key: client.EnvVarAgentIDTokenEndpoint},
		{Key: client.EnvVarAgentIDScopes},
	}, false)
	assert.ElementsMatch(t, balAgentIdentityKeys(), otherAgentIdentityEnvVarKeys(plain))
	assert.ElementsMatch(t, plainAgentIdentityKeys(), otherAgentIdentityEnvVarKeys(agentIdentityEnvVarsAs(plain, true)))
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_NameSetFollowsSavedSetting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		asBal    bool
		wantKeys []string
	}{
		{"off: AMP_AGENTID_* only", false, plainAgentIdentityKeys()},
		{"on: BAL_CONFIG_VAR_AMPAGENTID* only", true, balAgentIdentityKeys()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newBalConfigurablesIdentityService(injectableOCClient(), tc.asBal)

			vars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.wantKeys, agentIdentityKeysOf(vars))
			for _, ev := range vars {
				if ev.Key == client.BalConfigVarAgentIDClientSecret || ev.Key == client.EnvVarAgentIDClientSecret {
					require.NotNil(t, ev.ValueFrom)
					assert.Equal(t, testIdentitySecretRefName(), ev.ValueFrom.SecretKeyRef.Name)
				}
			}
		})
	}
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_NoSavedConfigUsesPlainNames(t *testing.T) {
	svc := newTestIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil), injectableOCClient())

	vars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	assert.ElementsMatch(t, plainAgentIdentityKeys(), agentIdentityKeysOf(vars))
}

// A failed settings read must not be guessed: the wrong name set either stops a
// Ballerina program from starting or withholds its credentials.
func TestAgentIdentityInjection_EnvVarsForEnvironment_SettingReadErrorPropagates(t *testing.T) {
	readErr := errors.New("db down")
	envConfigRepo := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentConfig, error) { return nil, readErr },
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil), noMCPConfigRepo(),
		envConfigRepo, noMCPProxyScopeRepo(), injectableOCClient(), "1h", discardLogger())

	_, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	assert.ErrorIs(t, err, readErr)
}

func TestAgentIdentityInjection_InjectForEnvironment_RemovesOtherNameSetInSameWrite(t *testing.T) {
	for _, tc := range []struct {
		name       string
		asBal      bool
		wantAdded  []string
		wantRemove []string
	}{
		{"on: adds prefixed, removes plain", true, balAgentIdentityKeys(), plainAgentIdentityKeys()},
		{"off: adds plain, removes prefixed", false, plainAgentIdentityKeys(), balAgentIdentityKeys()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var removed []string
			var added []client.EnvVar
			oc := injectableOCClient()
			oc.ReplaceReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, envName string, keysToRemove []string, envVars []client.EnvVar) error {
				assert.Equal(t, testIdentityEnv, envName)
				removed, added = keysToRemove, envVars
				return nil
			}
			svc := newBalConfigurablesIdentityService(oc, tc.asBal)

			require.NoError(t, svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
			assert.ElementsMatch(t, tc.wantAdded, agentIdentityKeysOf(added))
			assert.ElementsMatch(t, tc.wantRemove, removed)
		})
	}
}

// liveIdentityEnvVars is what GetComponentConfigurations reports for a
// workload carrying the given identity keys (values as completedInternalBinding
// would produce, empty scope list).
func liveIdentityEnvVars(keys []string) []models.EnvVars {
	vars := make([]models.EnvVars, 0, len(keys))
	for _, k := range keys {
		vars = append(vars, models.EnvVars{Key: k, Value: ""})
	}
	return vars
}

func TestAgentIdentityInjection_ReconcileForEnvironment_SwitchedOn_SwapsNameSetsAndCleansWorkload(t *testing.T) {
	var replaceRemoved []string
	var replaceAdded []client.EnvVar
	var workloadRemoved []string
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		// Still on the plain names from before the setting was switched on.
		return liveIdentityEnvVars(plainAgentIdentityKeys()), nil
	}
	oc.ReplaceReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, keysToRemove []string, envVars []client.EnvVar) error {
		replaceRemoved, replaceAdded = keysToRemove, envVars
		return nil
	}
	oc.RemoveWorkloadEnvVarsFunc = func(_ context.Context, _, agentName string, keys []string) error {
		assert.Equal(t, testIdentityAgent, agentName)
		workloadRemoved = keys
		return nil
	}
	svc := newBalConfigurablesIdentityService(oc, true)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.ElementsMatch(t, balAgentIdentityKeys(), agentIdentityKeysOf(replaceAdded))
	assert.ElementsMatch(t, plainAgentIdentityKeys(), replaceRemoved)
	assert.ElementsMatch(t, plainAgentIdentityKeys(), workloadRemoved,
		"stale names live at Workload level would no longer be shadowed by the override, so they must go too")
}

func TestAgentIdentityInjection_ReconcileForEnvironment_SwitchedOff_RemovesPrefixedNames(t *testing.T) {
	var replaceRemoved []string
	var replaceAdded []client.EnvVar
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		return liveIdentityEnvVars(balAgentIdentityKeys()), nil
	}
	oc.ReplaceReleaseBindingEnvVarsFunc = func(_ context.Context, _, _, _, _ string, keysToRemove []string, envVars []client.EnvVar) error {
		replaceRemoved, replaceAdded = keysToRemove, envVars
		return nil
	}
	oc.RemoveWorkloadEnvVarsFunc = func(_ context.Context, _, _ string, _ []string) error { return nil }
	svc := newBalConfigurablesIdentityService(oc, false)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.ElementsMatch(t, plainAgentIdentityKeys(), agentIdentityKeysOf(replaceAdded))
	assert.ElementsMatch(t, balAgentIdentityKeys(), replaceRemoved,
		"a leftover BAL_CONFIG_VAR_* var with no configurable stops a Ballerina program from starting")
}

func TestAgentIdentityInjection_ReconcileForEnvironment_PrefixedInSync_DoesNotWrite(t *testing.T) {
	oc := injectableOCClient()
	oc.GetComponentConfigurationsFunc = func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
		return liveIdentityEnvVars(balAgentIdentityKeys()), nil
	}
	// ReplaceReleaseBindingEnvVarsFunc / RemoveWorkloadEnvVarsFunc left nil — any
	// write would panic, proving an in-sync workload is never rolled.
	svc := newBalConfigurablesIdentityService(oc, true)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestIdentityEnvVarsInSync_ComparesScopesUnderPrefixedName(t *testing.T) {
	desired := []client.EnvVar{{Key: client.BalConfigVarAgentIDScopes, Value: "tickets:read"}}

	assert.True(t, identityEnvVarsInSync(desired, []models.EnvVars{{Key: client.BalConfigVarAgentIDScopes, Value: "tickets:read"}}))
	assert.False(t, identityEnvVarsInSync(desired, []models.EnvVars{{Key: client.BalConfigVarAgentIDScopes, Value: ""}}),
		"a drifted scope list must re-inject under the prefixed name too")
	assert.False(t, identityEnvVarsInSync(desired, []models.EnvVars{
		{Key: client.BalConfigVarAgentIDScopes, Value: "tickets:read"},
		{Key: client.EnvVarAgentIDClientID, Value: "client-abc"},
	}), "the other name set still being live is out of sync")
}

func TestResolveAgentIDAsBalConfigurables(t *testing.T) {
	on, off := true, false
	saved := func(v bool) *models.AgentConfig { return &models.AgentConfig{AgentIDAsBalConfigurables: v} }
	for _, tc := range []struct {
		name        string
		isBallerina bool
		existing    *models.AgentConfig
		requested   *bool
		want        bool
		wantErr     bool
	}{
		{"nothing saved or requested: off", true, nil, nil, false, false},
		{"request wins over saved", true, saved(true), &off, false, false},
		{"request turns it on", true, saved(false), &on, true, false},
		{"omitted request keeps saved value", true, saved(true), nil, true, false},
		{"non-Ballerina request to enable is rejected", false, nil, &on, false, true},
		{"non-Ballerina request to disable is fine", false, nil, &off, false, false},
		{"saved value never applies to a non-Ballerina agent", false, saved(true), nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAgentIDAsBalConfigurables(tc.isBallerina, tc.existing, tc.requested)
			if tc.wantErr {
				assert.ErrorIs(t, err, utils.ErrInvalidInput)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// deploySettingsAgentIDHarness runs UpdateAgentDeploySettings for a Ballerina
// API agent whose saved config has AgentIDAsBalConfigurables=saved, recording
// what was persisted and which environments were re-injected.
func deploySettingsAgentIDHarness(t *testing.T, saved bool, language string, requested *bool) (*models.AgentConfig, []string, error) {
	t.Helper()
	var persisted *models.AgentConfig
	var reconciled []string
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Type:  models.AgentType{Type: string(utils.AgentTypeAPI)},
				Build: &models.Build{Type: "buildpack", Buildpack: &models.BuildpackConfig{Language: language}},
			}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: "env-uuid"}, nil
		},
		UpdateReleaseBindingTraitConfigsFunc: func(_ context.Context, _, _, _ string, _ map[string]interface{}, _ map[string]interface{}) error {
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
			return &models.AgentConfig{AgentIDAsBalConfigurables: saved}, nil
		},
		UpsertFunc: func(_ context.Context, cfg *models.AgentConfig) error {
			persisted = cfg
			return nil
		},
	}
	injector := &agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(_ context.Context, _, _, _, envName string) error {
			reconciled = append(reconciled, envName)
			return nil
		},
	}
	s := &agentManagerService{
		ocClient:               ocClient,
		artifactRepo:           artifactRepo,
		agentConfigRepo:        agentConfigRepo,
		agentIdentityInjection: injector,
		logger:                 discardLogger(),
	}
	err := s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.UpdateAgentDeploySettingsRequest{
		EnvironmentName:                 "dev",
		AgentIdAsBallerinaConfigurables: requested,
	})
	return persisted, reconciled, err
}

func TestUpdateAgentDeploySettings_AgentIDAsBalConfigurables(t *testing.T) {
	on, off := true, false
	ballerina := string(utils.LanguageBallerina)

	t.Run("turning it on persists and re-injects only that environment", func(t *testing.T) {
		persisted, reconciled, err := deploySettingsAgentIDHarness(t, false, ballerina, &on)
		require.NoError(t, err)
		require.NotNil(t, persisted)
		assert.True(t, persisted.AgentIDAsBalConfigurables)
		assert.Equal(t, []string{"dev"}, reconciled)
	})
	t.Run("turning it off persists and re-injects", func(t *testing.T) {
		persisted, reconciled, err := deploySettingsAgentIDHarness(t, true, ballerina, &off)
		require.NoError(t, err)
		assert.False(t, persisted.AgentIDAsBalConfigurables)
		assert.Equal(t, []string{"dev"}, reconciled)
	})
	t.Run("an unrelated settings edit keeps the saved value and rolls nothing", func(t *testing.T) {
		persisted, reconciled, err := deploySettingsAgentIDHarness(t, true, ballerina, nil)
		require.NoError(t, err)
		assert.True(t, persisted.AgentIDAsBalConfigurables, "an upsert that omits the field must not reset it")
		assert.Empty(t, reconciled)
	})
	t.Run("rejected for a non-Ballerina agent before anything is written", func(t *testing.T) {
		persisted, reconciled, err := deploySettingsAgentIDHarness(t, false, string(utils.LanguagePython), &on)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Nil(t, persisted)
		assert.Empty(t, reconciled)
	})
}

// promoteBallerinaAgentHarness promotes a Ballerina API agent from dev (whose
// saved AgentIDAsBalConfigurables is sourceSetting) to staging, returning the
// env overrides sent to PromoteComponent and the target config persisted.
func promoteBallerinaAgentHarness(t *testing.T, sourceSetting bool, requested *bool) ([]client.EnvVar, *models.AgentConfig) {
	t.Helper()
	s, promoted := promoteAgentTestFixture(t, []client.EnvVar{
		{Key: client.EnvVarAgentIDClientID, Value: "staging-client-id"},
		{Key: client.EnvVarAgentIDScopes, Value: ""},
	}, nil)
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.GetComponentFunc = func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         "agent-uuid",
			Name:         name,
			Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
			Type:         models.AgentType{Type: string(utils.AgentTypeAPI), SubType: string(utils.AgentSubTypeA2A)},
			Build:        &models.Build{Type: "buildpack", Buildpack: &models.BuildpackConfig{Language: string(utils.LanguageBallerina)}},
		}, nil
	}
	stagingUUID := uuid.New()
	ocMock.GetEnvironmentFunc = func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: envName, UUID: stagingUUID.String()}, nil
	}
	var overrides []client.EnvVar
	ocMock.PromoteComponentFunc = func(_ context.Context, _, _, _, _, _ string, envOverrides []client.EnvVar, _ []client.FileVar, _, _ map[string]interface{}) error {
		*promoted = true
		overrides = envOverrides
		return nil
	}
	var upserted *models.AgentConfig
	s.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, envName string) (*models.AgentConfig, error) {
			require.Equal(t, "dev", envName, "promote resolves settings from the source environment")
			return &models.AgentConfig{AgentIDAsBalConfigurables: sourceSetting}, nil
		},
		UpsertFunc: func(_ context.Context, cfg *models.AgentConfig) error {
			upserted = cfg
			return nil
		},
	}
	s.artifactRepo = &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(_, _ string) (*models.Artifact, error) {
			return &models.Artifact{UUID: uuid.New(), Kind: models.KindAgent}, nil
		},
	}
	s.a2aPublicationRepo = &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(_ context.Context, _ *models.A2APublication) error { return nil },
	}

	require.NoError(t, s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment:               "dev",
		TargetEnvironment:               "staging",
		AgentIdAsBallerinaConfigurables: requested,
	}))
	require.True(t, *promoted)
	return overrides, upserted
}

func overrideValue(vars []client.EnvVar, key string) (string, bool) {
	for _, ev := range vars {
		if ev.Key == key {
			return ev.Value, true
		}
	}
	return "", false
}

// By default the setting travels with the promoted code: the target inherits
// the source environment's value, and gets ITS OWN credentials under the
// prefixed names — never the source's.
func TestPromoteAgent_AgentIDAsBalConfigurables_InheritsFromSourceEnvironment(t *testing.T) {
	overrides, upserted := promoteBallerinaAgentHarness(t, true, nil)

	value, ok := overrideValue(overrides, client.BalConfigVarAgentIDClientID)
	require.True(t, ok, "the target must get the prefixed names")
	assert.Equal(t, "staging-client-id", value, "the target's own credential, not the source's")
	_, plain := overrideValue(overrides, client.EnvVarAgentIDClientID)
	assert.False(t, plain, "only one name set may be injected")

	require.NotNil(t, upserted)
	assert.Equal(t, "staging", upserted.EnvironmentName)
	assert.True(t, upserted.AgentIDAsBalConfigurables, "the inherited value is saved as the target's own")
}

func TestPromoteAgent_AgentIDAsBalConfigurables_RequestOverridesSource(t *testing.T) {
	off := false
	overrides, upserted := promoteBallerinaAgentHarness(t, true, &off)

	value, ok := overrideValue(overrides, client.EnvVarAgentIDClientID)
	require.True(t, ok)
	assert.Equal(t, "staging-client-id", value)
	_, prefixed := overrideValue(overrides, client.BalConfigVarAgentIDClientID)
	assert.False(t, prefixed)
	require.NotNil(t, upserted)
	assert.False(t, upserted.AgentIDAsBalConfigurables)
}
