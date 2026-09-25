//
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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func TestMergeAgentAPIKeySecretRef(t *testing.T) {
	envInjKey := "myagent-" + string(TraitEnvInjection)

	t.Run("nil existing returns incoming unchanged", func(t *testing.T) {
		incoming := map[string]interface{}{"other-trait": map[string]interface{}{"x": 1}}
		got := mergeAgentAPIKeySecretRef(nil, incoming, "myagent")
		assert.Equal(t, incoming, got)
	})

	t.Run("existing has no env-injection entry returns incoming unchanged", func(t *testing.T) {
		existing := map[string]interface{}{"other-trait": map[string]interface{}{}}
		incoming := map[string]interface{}{}
		got := mergeAgentAPIKeySecretRef(&existing, incoming, "myagent")
		assert.Empty(t, got)
	})

	t.Run("existing entry has empty ref returns incoming unchanged", func(t *testing.T) {
		existing := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": ""}}
		incoming := map[string]interface{}{}
		got := mergeAgentAPIKeySecretRef(&existing, incoming, "myagent")
		_, present := got[envInjKey]
		assert.False(t, present)
	})

	t.Run("incoming already sets its own ref is left untouched", func(t *testing.T) {
		existing := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": "old/path"}}
		incoming := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": "new/path"}}
		got := mergeAgentAPIKeySecretRef(&existing, incoming, "myagent")
		entry := got[envInjKey].(map[string]interface{})
		assert.Equal(t, "new/path", entry["agentApiKeySecretRef"])
	})

	t.Run("preserves ref and property into nil incoming", func(t *testing.T) {
		existing := map[string]interface{}{
			envInjKey: map[string]interface{}{"agentApiKeySecretRef": "org/proj/prod/agent/key", "agentApiKeySecretProperty": "api-key"},
		}
		got := mergeAgentAPIKeySecretRef(&existing, nil, "myagent")
		entry := got[envInjKey].(map[string]interface{})
		assert.Equal(t, "org/proj/prod/agent/key", entry["agentApiKeySecretRef"])
		assert.Equal(t, "api-key", entry["agentApiKeySecretProperty"])
	})

	t.Run("preserves ref into incoming entry without clobbering other fields", func(t *testing.T) {
		existing := map[string]interface{}{
			envInjKey: map[string]interface{}{"agentApiKeySecretRef": "org/proj/prod/agent/key"},
		}
		incoming := map[string]interface{}{envInjKey: map[string]interface{}{"envInjectionEnabled": true}}
		got := mergeAgentAPIKeySecretRef(&existing, incoming, "myagent")
		entry := got[envInjKey].(map[string]interface{})
		assert.Equal(t, true, entry["envInjectionEnabled"])
		assert.Equal(t, "org/proj/prod/agent/key", entry["agentApiKeySecretRef"])
	})

	t.Run("incoming ref without a property is left as-is, not backfilled from a different existing secret", func(t *testing.T) {
		existing := map[string]interface{}{
			envInjKey: map[string]interface{}{"agentApiKeySecretRef": "org/proj/prod/agent/old-key", "agentApiKeySecretProperty": "api-key"},
		}
		incoming := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": "org/proj/prod/agent/new-key"}}
		got := mergeAgentAPIKeySecretRef(&existing, incoming, "myagent")
		entry := got[envInjKey].(map[string]interface{})
		assert.Equal(t, "org/proj/prod/agent/new-key", entry["agentApiKeySecretRef"])
		_, present := entry["agentApiKeySecretProperty"]
		assert.False(t, present, "property must come from whoever set the new ref, never spliced from a different secret")
	})

	t.Run("a retry with a fresh existing ref is not poisoned by a prior attempt's merge", func(t *testing.T) {
		// Simulates retryReleaseBindingUpdate re-invoking its callback after a conflict: the
		// caller's incoming map must stay usable for a second merge against a newly-fetched
		// existing, not carry forward the first attempt's result.
		incoming := map[string]interface{}{}
		firstAttemptExisting := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": "stale/ref"}}
		mergeAgentAPIKeySecretRef(&firstAttemptExisting, incoming, "myagent")

		concurrentlyRotatedExisting := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": "fresh/ref"}}
		got := mergeAgentAPIKeySecretRef(&concurrentlyRotatedExisting, incoming, "myagent")

		entry := got[envInjKey].(map[string]interface{})
		assert.Equal(t, "fresh/ref", entry["agentApiKeySecretRef"])
	})
}

// TestEnsureReleaseAndBinding_MergesTraitAndComponentTypeConfigsInOneWrite guards the fix for the
// double-pod-generation bug: a deploy used to write the release pin and workload overrides here,
// then a separate follow-up call wrote trait/component-type configs — two writes to the same
// binding that raced each other's resourceVersion and let OpenChoreo apply two different renders
// for one deploy. Everything must now land in exactly one PUT, with the same merge semantics
// (agent API key secret ref carried forward, stale runtimeClassName cleared, restartedAt bumped)
// that UpdateReleaseBindingTraitConfigs already provides for the standalone deploy-settings path.
func TestEnsureReleaseAndBinding_MergesTraitAndComponentTypeConfigsInOneWrite(t *testing.T) {
	const componentName = "myagent"
	const environment = "dev"
	const bindingName = componentName + "-" + environment
	const releaseName = componentName + "-release-1"
	envInjKey := componentName + "-" + string(TraitEnvInjection)

	existingBinding := gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: bindingName},
		Spec: &gen.ReleaseBindingSpec{
			Environment: environment,
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: componentName, ProjectName: "myproject"},
			TraitEnvironmentConfigs: &map[string]interface{}{
				envInjKey: map[string]interface{}{"agentApiKeySecretRef": "org/proj/dev/myagent/key"},
			},
			ComponentTypeEnvironmentConfigs: &map[string]interface{}{
				"runtimeClassName": "gvisor",
				"restartedAt":      "2020-01-01T00:00:00Z",
			},
		},
	}

	var putCount int
	var gotBody gen.ReleaseBinding
	srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/generate-release"):
			w.WriteHeader(http.StatusCreated)
			require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: releaseName}}))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: []gen.ReleaseBinding{existingBinding}}))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(existingBinding))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
			putCount++
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(gotBody))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))

	envOverrides := []EnvVar{{Key: "FOO", Value: "bar"}}
	incomingTraitConfigs := map[string]interface{}{
		envInjKey: map[string]interface{}{"envInjectionEnabled": true},
	}
	incomingCTConfigs := map[string]interface{}{
		"someOtherKey": "value",
	}

	srv.resourceLabels = map[string]string{"example.com/product": "agent-manager"}

	err := srv.EnsureReleaseAndBinding(context.Background(), "acme", "myproject", componentName, environment,
		envOverrides, nil, incomingTraitConfigs, incomingCTConfigs)

	require.NoError(t, err)
	assert.Equal(t, 1, putCount, "the release pin, overrides, and trait/component-type configs must land in a single write")

	require.NotNil(t, gotBody.Spec)
	require.NotNil(t, gotBody.Spec.ReleaseName)
	assert.Equal(t, releaseName, *gotBody.Spec.ReleaseName)

	require.NotNil(t, gotBody.Spec.WorkloadOverrides)
	require.NotNil(t, gotBody.Spec.WorkloadOverrides.Container)
	require.NotNil(t, gotBody.Spec.WorkloadOverrides.Container.Env)
	envs := *gotBody.Spec.WorkloadOverrides.Container.Env
	require.Len(t, envs, 1)
	assert.Equal(t, "FOO", envs[0].Key)

	require.NotNil(t, gotBody.Spec.TraitEnvironmentConfigs)
	traitCfg, ok := (*gotBody.Spec.TraitEnvironmentConfigs)[envInjKey].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, traitCfg["envInjectionEnabled"], "incoming trait config must be applied")
	assert.Equal(t, "org/proj/dev/myagent/key", traitCfg["agentApiKeySecretRef"],
		"the agent API key secret ref must be carried forward from the existing binding")

	require.NotNil(t, gotBody.Spec.ComponentTypeEnvironmentConfigs)
	ctCfg := *gotBody.Spec.ComponentTypeEnvironmentConfigs
	assert.Equal(t, "value", ctCfg["someOtherKey"], "incoming component-type config must be merged in")
	_, hasRuntimeClass := ctCfg["runtimeClassName"]
	assert.False(t, hasRuntimeClass, "a stale runtimeClassName must be cleared when the incoming configs omit it")
	assert.NotEmpty(t, ctCfg["restartedAt"], "the binding must be stamped with a fresh restartedAt to trigger a pod rollout")

	// The existing binding carries no labels, as one created by the OpenChoreo
	// controller does; the write must still stamp the configured resource labels.
	require.NotNil(t, gotBody.Metadata.Labels)
	assert.Equal(t, "agent-manager", (*gotBody.Metadata.Labels)["example.com/product"])
}

// TestEnsureReleaseAndBinding_ClearsStaleRuntimeClassWithEmptyComponentTypeConfigs guards a
// regression found in review: buildComponentTypeEnvConfigs returns a non-nil EMPTY map (not nil)
// when an environment has reverted to the default runc tier, specifically so the stale
// runtimeClassName gets cleared. Gating the merge on "len(componentTypeConfigs) > 0" collapsed
// that authoritative empty map into a nil ctConfigs pointer and skipped the merge block entirely,
// silently leaving the previous isolation tier's runtimeClassName in place. The gate must be a
// nilness check, not an emptiness check. Covers both the existing-binding update path and the
// 409-conflict-adoption path, since the merge logic is duplicated between them.
func TestEnsureReleaseAndBinding_ClearsStaleRuntimeClassWithEmptyComponentTypeConfigs(t *testing.T) {
	const componentName = "myagent"
	const environment = "dev"
	const bindingName = componentName + "-" + environment
	const releaseName = componentName + "-release-1"

	bindingWithStaleRuntimeClass := gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: bindingName},
		Spec: &gen.ReleaseBindingSpec{
			Environment: environment,
			Owner: struct {
				ComponentName string `json:"componentName"`
				ProjectName   string `json:"projectName"`
			}{ComponentName: componentName, ProjectName: "myproject"},
			ComponentTypeEnvironmentConfigs: &map[string]interface{}{
				"runtimeClassName": "gvisor",
			},
		},
	}

	t.Run("existing-binding update path", func(t *testing.T) {
		var putCount int
		var gotBody gen.ReleaseBinding
		srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/generate-release"):
				w.WriteHeader(http.StatusCreated)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: releaseName}}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: []gen.ReleaseBinding{bindingWithStaleRuntimeClass}}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(bindingWithStaleRuntimeClass))
			case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
				putCount++
				require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(gotBody))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))

		// A non-nil, empty map is what buildComponentTypeEnvConfigs returns for the default
		// runc tier — deliberately distinct from nil ("don't touch component-type configs").
		err := srv.EnsureReleaseAndBinding(context.Background(), "acme", "myproject", componentName, environment,
			nil, nil, nil, map[string]interface{}{})

		require.NoError(t, err)
		assert.Equal(t, 1, putCount)
		require.NotNil(t, gotBody.Spec.ComponentTypeEnvironmentConfigs)
		_, hasRuntimeClass := (*gotBody.Spec.ComponentTypeEnvironmentConfigs)["runtimeClassName"]
		assert.False(t, hasRuntimeClass, "reverting to the default tier must clear the stale runtimeClassName")
	})

	t.Run("409-conflict-adoption path", func(t *testing.T) {
		var putCount int
		var gotBody gen.ReleaseBinding
		srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/generate-release"):
				w.WriteHeader(http.StatusCreated)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: releaseName}}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				// No existing binding found for this environment, so EnsureReleaseAndBinding
				// proceeds to create one — and loses the race to a concurrent writer.
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: []gen.ReleaseBinding{}}))
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				w.WriteHeader(http.StatusConflict)
				require.NoError(t, json.NewEncoder(w).Encode(gen.Conflict{Error: "already exists"}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(bindingWithStaleRuntimeClass))
			case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
				putCount++
				require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(gotBody))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))

		err := srv.EnsureReleaseAndBinding(context.Background(), "acme", "myproject", componentName, environment,
			nil, nil, nil, map[string]interface{}{})

		require.NoError(t, err)
		assert.Equal(t, 1, putCount)
		require.NotNil(t, gotBody.Spec.ComponentTypeEnvironmentConfigs)
		_, hasRuntimeClass := (*gotBody.Spec.ComponentTypeEnvironmentConfigs)["runtimeClassName"]
		assert.False(t, hasRuntimeClass, "adopting the winning binding must still clear the stale runtimeClassName")
	})
}
