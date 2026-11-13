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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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

	t.Run("existing empty ref is carried forward", func(t *testing.T) {
		// An empty ref marks an environment whose key comes from the agent's env vars;
		// dropping it would let the trait fall back to the path in its parameters.
		existing := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": ""}}
		incoming := map[string]interface{}{envInjKey: map[string]interface{}{"envInjectionEnabled": true}}
		got := mergeAgentAPIKeySecretRef(&existing, incoming, "myagent")
		entry := got[envInjKey].(map[string]interface{})
		ref, present := entry["agentApiKeySecretRef"]
		assert.True(t, present)
		assert.Equal(t, "", ref)
		assert.Equal(t, true, entry["envInjectionEnabled"])
	})

	t.Run("incoming empty ref replaces an existing path", func(t *testing.T) {
		existing := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": "old/path", "agentApiKeySecretProperty": "api-key"}}
		incoming := map[string]interface{}{envInjKey: map[string]interface{}{"agentApiKeySecretRef": ""}}
		got := mergeAgentAPIKeySecretRef(&existing, incoming, "myagent")
		entry := got[envInjKey].(map[string]interface{})
		assert.Equal(t, "", entry["agentApiKeySecretRef"])
		_, present := entry["agentApiKeySecretProperty"]
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

// releaseBindingStub serves one release binding and counts PUTs, capturing the last body.
func releaseBindingStub(t *testing.T, existing gen.ReleaseBinding) (*openChoreoClient, *int, *gen.ReleaseBinding) {
	t.Helper()
	var putCount int
	var gotBody gen.ReleaseBinding
	name := existing.Metadata.Name
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: []gen.ReleaseBinding{existing}}))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings/"+name):
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(existing))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/releasebindings/"+name):
			putCount++
			require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(gotBody))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	return c, &putCount, &gotBody
}

func bindingWithConfigs(traitCfgs, ctCfgs *map[string]interface{}) gen.ReleaseBinding {
	return gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "myagent-dev"},
		Spec: &gen.ReleaseBindingSpec{
			Environment:                     "dev",
			TraitEnvironmentConfigs:         traitCfgs,
			ComponentTypeEnvironmentConfigs: ctCfgs,
		},
	}
}

type testPolicy struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

func TestUpdateReleaseBindingTraitConfigs_SkipsWriteWhenConfigsUnchanged(t *testing.T) {
	envInjKey := "myagent-" + string(TraitEnvInjection)
	apiKey := "myagent-api-configuration"

	t.Run("identical configs with typed values and a carried-forward secret ref", func(t *testing.T) {
		existing := bindingWithConfigs(
			&map[string]interface{}{
				apiKey:    map[string]interface{}{"policies": []interface{}{map[string]interface{}{"name": "cors", "port": 8080}}},
				envInjKey: map[string]interface{}{"envInjectionEnabled": true, "agentApiKeySecretRef": "ref", "agentApiKeySecretProperty": "key"},
			},
			&map[string]interface{}{"runtimeClassName": "gvisor", "restartedAt": "2020-01-01T00:00:00Z"},
		)
		c, puts, _ := releaseBindingStub(t, existing)

		err := c.UpdateReleaseBindingTraitConfigs(context.Background(), "acme", "myagent", "dev",
			map[string]interface{}{
				apiKey:    map[string]interface{}{"policies": []testPolicy{{Name: "cors", Port: 8080}}},
				envInjKey: map[string]interface{}{"envInjectionEnabled": true},
			},
			map[string]interface{}{"runtimeClassName": "gvisor"})

		require.NoError(t, err)
		assert.Equal(t, 0, *puts, "an unchanged deploy-settings save must not write or roll the pod")
	})

	t.Run("nil existing configs and empty incoming configs", func(t *testing.T) {
		c, puts, _ := releaseBindingStub(t, bindingWithConfigs(nil, nil))

		err := c.UpdateReleaseBindingTraitConfigs(context.Background(), "acme", "myagent", "dev",
			map[string]interface{}{}, map[string]interface{}{})

		require.NoError(t, err)
		assert.Equal(t, 0, *puts, "nil and empty configs are the same configuration")
	})
}

func TestUpdateReleaseBindingTraitConfigs_BumpsRestartedAtOnRealChange(t *testing.T) {
	const staleRestart = "2020-01-01T00:00:00Z"
	envInjKey := "myagent-" + string(TraitEnvInjection)
	enabled := map[string]interface{}{envInjKey: map[string]interface{}{"envInjectionEnabled": true}}

	cases := map[string]struct {
		trait map[string]interface{}
		ct    map[string]interface{}
	}{
		"trait config changed":          {trait: map[string]interface{}{envInjKey: map[string]interface{}{"envInjectionEnabled": false}}, ct: map[string]interface{}{"runtimeClassName": "gvisor"}},
		"runtime class cleared":         {trait: enabled, ct: map[string]interface{}{}},
		"component-type config changed": {trait: enabled, ct: map[string]interface{}{"runtimeClassName": "kata"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			existing := bindingWithConfigs(
				&map[string]interface{}{envInjKey: map[string]interface{}{"envInjectionEnabled": true}},
				&map[string]interface{}{"runtimeClassName": "gvisor", "restartedAt": staleRestart},
			)
			c, puts, got := releaseBindingStub(t, existing)

			err := c.UpdateReleaseBindingTraitConfigs(context.Background(), "acme", "myagent", "dev", tc.trait, tc.ct)

			require.NoError(t, err)
			require.Equal(t, 1, *puts)
			require.NotNil(t, got.Spec.ComponentTypeEnvironmentConfigs)
			restartedAt := (*got.Spec.ComponentTypeEnvironmentConfigs)["restartedAt"]
			assert.NotEmpty(t, restartedAt)
			assert.NotEqual(t, staleRestart, restartedAt, "a real config change must roll the pod")
		})
	}
}

func TestEnsureReleaseBindingRuntimeClass_SkipsWriteWhenAlreadyCorrect(t *testing.T) {
	c, puts, _ := releaseBindingStub(t, bindingWithConfigs(nil, &map[string]interface{}{"runtimeClassName": "gvisor"}))

	require.NoError(t, c.EnsureReleaseBindingRuntimeClass(context.Background(), "acme", "myagent", "dev", "gvisor"))
	assert.Equal(t, 0, *puts)
}

func TestGetReleaseBindingRollout(t *testing.T) {
	port := int32(8000)
	aPath, bPath, scheme := "/a", "/b", "http"
	binding := func(release string, conditions ...gen.Condition) gen.ReleaseBinding {
		b := bindingWithConfigs(nil, nil)
		b.Metadata.Name = "myagent-dev"
		b.Spec.Environment = "dev"
		b.Spec.ReleaseName = &release
		b.Status = &gen.ReleaseBindingStatus{
			Conditions: &conditions,
			Endpoints: &[]gen.EndpointURLStatus{
				{Name: "b", ServiceURL: &gen.EndpointURL{Host: "b.svc", Port: &port}, ExternalURLs: &gen.EndpointGatewayURLs{Http: &gen.EndpointURL{Scheme: &scheme, Host: "gw.example", Path: &bPath}}},
				{Name: "a", ServiceURL: &gen.EndpointURL{Host: "dev.svc", Port: &port}, ExternalURLs: &gen.EndpointGatewayURLs{Http: &gen.EndpointURL{Scheme: &scheme, Host: "gw.example", Path: &aPath}}},
			},
		}
		return b
	}
	serve := func(t *testing.T, items ...gen.ReleaseBinding) *openChoreoClient {
		t.Helper()
		return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: items}))
		}))
	}
	ready := gen.Condition{Type: "Ready", Status: "True", Reason: "Ready"}
	progressing := gen.Condition{Type: "Ready", Status: "False", Reason: "ResourcesProgressing"}

	t.Run("a ready binding serves its release", func(t *testing.T) {
		got, err := serve(t, binding("myagent-r2", ready)).GetReleaseBindingRollout(context.Background(), "acme", "myagent", "dev")
		require.NoError(t, err)
		assert.Equal(t, ReleaseBindingRollout{
			ServiceURL: "http://b.svc:8000", ExternalURL: "http://gw.example/a", ReleaseName: "myagent-r2", Serving: true, Deployed: true,
		}, got, "the external URL is the first endpoint by name, as the card fetch needs")
	})

	t.Run("a rollout in progress is not serving its release yet", func(t *testing.T) {
		got, err := serve(t, binding("myagent-r2", progressing)).GetReleaseBindingRollout(context.Background(), "acme", "myagent", "dev")
		require.NoError(t, err)
		assert.Equal(t, "myagent-r2", got.ReleaseName)
		assert.False(t, got.Serving)
		assert.True(t, got.Deployed, "a binding mid-rollout is deployed")
	})

	t.Run("a binding not reconciled yet is deployed", func(t *testing.T) {
		b := binding("myagent-r1")
		b.Status = nil
		got, err := serve(t, b).GetReleaseBindingRollout(context.Background(), "acme", "myagent", "dev")
		require.NoError(t, err)
		assert.True(t, got.Deployed, "a first deploy must not read as undeployed")
		assert.False(t, got.Serving)
	})

	t.Run("an undeployed binding is not serving", func(t *testing.T) {
		b := binding("myagent-r2", ready)
		undeploy := gen.ReleaseBindingSpecStateUndeploy
		b.Spec.State = &undeploy
		got, err := serve(t, b).GetReleaseBindingRollout(context.Background(), "acme", "myagent", "dev")
		require.NoError(t, err)
		assert.False(t, got.Serving)
		assert.False(t, got.Deployed)
	})

	t.Run("no binding for the environment is the zero rollout", func(t *testing.T) {
		got, err := serve(t).GetReleaseBindingRollout(context.Background(), "acme", "myagent", "dev")
		require.NoError(t, err)
		assert.Equal(t, ReleaseBindingRollout{}, got)
	})
}

func TestGetReleaseBindingTraits(t *testing.T) {
	release := func(traits ...string) gen.ComponentRelease {
		r := gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "myagent-r2"}, Spec: &gen.ComponentReleaseSpec{}}
		componentTraits := make([]gen.ComponentTrait, 0, len(traits))
		for _, name := range traits {
			componentTraits = append(componentTraits, gen.ComponentTrait{Name: name, InstanceName: name + "-1"})
		}
		r.Spec.ComponentProfile = &struct {
			Parameters *map[string]interface{} `json:"parameters,omitempty"`
			Traits     *[]gen.ComponentTrait   `json:"traits,omitempty"`
		}{Traits: &componentTraits}
		return r
	}
	serve := func(t *testing.T, r gen.ComponentRelease, bindings ...gen.ReleaseBinding) (*openChoreoClient, *[]string) {
		t.Helper()
		var releaseReads []string
		return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if strings.HasSuffix(req.URL.Path, "/releasebindings") {
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: bindings}))
				return
			}
			releaseReads = append(releaseReads, req.URL.Path)
			require.NoError(t, json.NewEncoder(w).Encode(r))
		})), &releaseReads
	}
	bound := func(releaseName string) gen.ReleaseBinding {
		b := bindingWithConfigs(nil, nil)
		b.Spec.ReleaseName = &releaseName
		return b
	}

	t.Run("the bound release's trait names", func(t *testing.T) {
		c, reads := serve(t, release(string(TraitAPIManagement), string(TraitOTELInstrumentation)), bound("myagent-r2"))

		traits, isBound, err := c.GetReleaseBindingTraits(context.Background(), "acme", "myagent", "dev")

		require.NoError(t, err)
		assert.True(t, isBound)
		assert.Equal(t, []TraitType{TraitAPIManagement, TraitOTELInstrumentation}, traits)
		require.Len(t, *reads, 1)
		assert.True(t, strings.HasSuffix((*reads)[0], "/componentreleases/myagent-r2"), (*reads)[0])
	})
	t.Run("a release with no traits", func(t *testing.T) {
		c, _ := serve(t, release(), bound("myagent-r2"))

		traits, isBound, err := c.GetReleaseBindingTraits(context.Background(), "acme", "myagent", "dev")

		require.NoError(t, err)
		assert.True(t, isBound)
		assert.Empty(t, traits)
	})
	t.Run("no binding for the environment is unbound", func(t *testing.T) {
		c, reads := serve(t, release(string(TraitAPIManagement)))

		traits, isBound, err := c.GetReleaseBindingTraits(context.Background(), "acme", "myagent", "dev")

		require.NoError(t, err)
		assert.False(t, isBound)
		assert.Empty(t, traits)
		assert.Empty(t, *reads, "no release is read without a binding")
	})
}

func TestGetReleaseBindingServiceURL(t *testing.T) {
	port := int32(8000)
	https := "https"
	withEndpoints := func(env string, eps *[]gen.EndpointURLStatus) gen.ReleaseBinding {
		b := bindingWithConfigs(nil, nil)
		b.Metadata.Name = "myagent-" + env
		b.Spec.Environment = env
		b.Status = &gen.ReleaseBindingStatus{Endpoints: eps}
		return b
	}
	serve := func(t *testing.T, status int, items ...gen.ReleaseBinding) *openChoreoClient {
		t.Helper()
		return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodGet, r.Method)
			require.True(t, strings.HasSuffix(r.URL.Path, "/releasebindings"), r.URL.Path)
			w.WriteHeader(status)
			if status == http.StatusOK {
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: items}))
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"error": "boom"}))
		}))
	}

	t.Run("picks the environment's first endpoint with a host and defaults the scheme to http", func(t *testing.T) {
		c := serve(
			t, http.StatusOK,
			withEndpoints("prod", &[]gen.EndpointURLStatus{{Name: "a", ServiceURL: &gen.EndpointURL{Host: "prod.svc", Port: &port}}}),
			withEndpoints("dev", &[]gen.EndpointURLStatus{
				{Name: "blank", ServiceURL: &gen.EndpointURL{Host: " "}},
				{Name: "a", ServiceURL: &gen.EndpointURL{Host: "dev.svc", Port: &port}},
			}),
		)
		got, err := c.GetReleaseBindingServiceURL(context.Background(), "acme", "myagent", "dev")
		require.NoError(t, err)
		assert.Equal(t, "http://dev.svc:8000", got)
	})

	t.Run("keeps an explicit scheme", func(t *testing.T) {
		c := serve(t, http.StatusOK,
			withEndpoints("dev", &[]gen.EndpointURLStatus{{Name: "a", ServiceURL: &gen.EndpointURL{Host: "dev.svc", Scheme: &https}}}))
		got, err := c.GetReleaseBindingServiceURL(context.Background(), "acme", "myagent", "dev")
		require.NoError(t, err)
		assert.Equal(t, "https://dev.svc", got)
	})

	// A WebSocket endpoint and the agent's own HTTP endpoint, in either order.
	ws := "ws"
	wsUpper := "WS"
	wsPort := int32(8080)
	agentPort := int32(9090)
	agentPath := "/chat-agent"
	socket := gen.EndpointURLStatus{Name: "events", ServiceURL: &gen.EndpointURL{Host: "myagent.svc", Port: &wsPort, Scheme: &ws}}
	own := gen.EndpointURLStatus{Name: "myagent-endpoint", ServiceURL: &gen.EndpointURL{Host: "myagent.svc", Port: &agentPort, Path: &agentPath}}

	picks := map[string]struct {
		endpoints []gen.EndpointURLStatus
		want      string
	}{
		"skips a WebSocket endpoint listed before the agent's own": {
			endpoints: []gen.EndpointURLStatus{socket, own},
			want:      "http://myagent.svc:9090/chat-agent",
		},
		"skips a WebSocket endpoint whatever its scheme's case": {
			endpoints: []gen.EndpointURLStatus{
				{Name: "events", ServiceURL: &gen.EndpointURL{Host: "myagent.svc", Port: &wsPort, Scheme: &wsUpper}},
				{Name: "other", ServiceURL: &gen.EndpointURL{Host: "other.svc", Port: &port}},
			},
			want: "http://other.svc:8000",
		},
		"takes the agent's own endpoint over another HTTP endpoint listed first": {
			endpoints: []gen.EndpointURLStatus{{Name: "a", ServiceURL: &gen.EndpointURL{Host: "a.svc", Port: &port}}, own},
			want:      "http://myagent.svc:9090/chat-agent",
		},
		"falls back to the first HTTP endpoint when none is named after the agent": {
			endpoints: []gen.EndpointURLStatus{socket, {Name: "a", ServiceURL: &gen.EndpointURL{Host: "a.svc", Port: &port, Scheme: &https}}},
			want:      "https://a.svc:8000",
		},
	}
	for name, tc := range picks {
		t.Run(name, func(t *testing.T) {
			eps := tc.endpoints
			got, err := serve(t, http.StatusOK, withEndpoints("dev", &eps)).GetReleaseBindingServiceURL(context.Background(), "acme", "myagent", "dev")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	notReady := map[string][]gen.ReleaseBinding{
		"no binding for the environment": {withEndpoints("prod", &[]gen.EndpointURLStatus{{Name: "a", ServiceURL: &gen.EndpointURL{Host: "prod.svc"}}})},
		"status has no endpoints yet":    {withEndpoints("dev", nil)},
		"no endpoint carries a host":     {withEndpoints("dev", &[]gen.EndpointURLStatus{{Name: "a"}})},
		"only a WebSocket endpoint":      {withEndpoints("dev", &[]gen.EndpointURLStatus{socket})},
	}
	for name, items := range notReady {
		t.Run(name+" is not ready, not an error", func(t *testing.T) {
			got, err := serve(t, http.StatusOK, items...).GetReleaseBindingServiceURL(context.Background(), "acme", "myagent", "dev")
			require.NoError(t, err)
			assert.Empty(t, got)
		})
	}

	t.Run("a failed list is an error", func(t *testing.T) {
		_, err := serve(t, http.StatusInternalServerError).GetReleaseBindingServiceURL(context.Background(), "acme", "myagent", "dev")
		require.Error(t, err)
	})
}

// Deploy writes the deployed build's health checks onto the binding in the same update as
// the release: its checks replace the earlier build's, an explicit nil (the build has none)
// removes them, and leaving the key out (the build couldn't be looked up) keeps them. Other
// component-type settings (here replicas) are kept as before.
func TestEnsureReleaseAndBinding_ReplacesOrRemovesTheBuildsHealthChecks(t *testing.T) {
	const componentName = "myagent"
	const environment = "dev"
	const bindingName = componentName + "-" + environment

	// The binding as an earlier build left it: its readiness check plus an environment setting.
	bindingFromEarlierBuild := func() gen.ReleaseBinding {
		return gen.ReleaseBinding{
			Metadata: gen.ObjectMeta{Name: bindingName},
			Spec: &gen.ReleaseBindingSpec{
				Environment: environment,
				Owner: struct {
					ComponentName string `json:"componentName"`
					ProjectName   string `json:"projectName"`
				}{ComponentName: componentName, ProjectName: "myproject"},
				ComponentTypeEnvironmentConfigs: &map[string]interface{}{
					"replicas":     float64(2),
					"healthChecks": map[string]interface{}{"readiness": map[string]interface{}{"type": "TCP"}},
				},
			},
		}
	}

	// configsWrittenByEnsureReleaseAndBinding runs EnsureReleaseAndBinding against a fake OpenChoreo,
	// sending the given component-type configs, and returns the configs written to the binding.
	// The binding already exists, or, with adoption, the create loses a race and it is adopted.
	configsWrittenByEnsureReleaseAndBinding := func(t *testing.T, adoption bool, componentTypeConfigs map[string]interface{}) map[string]interface{} {
		t.Helper()
		var written gen.ReleaseBinding
		srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/generate-release"):
				w.WriteHeader(http.StatusCreated)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "release-2"}}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				items := []gen.ReleaseBinding{bindingFromEarlierBuild()}
				if adoption {
					items = []gen.ReleaseBinding{}
				}
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: items}))
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				w.WriteHeader(http.StatusConflict)
				require.NoError(t, json.NewEncoder(w).Encode(gen.Conflict{Error: "already exists"}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(bindingFromEarlierBuild()))
			case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName):
				require.NoError(t, json.NewDecoder(r.Body).Decode(&written))
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(written))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		require.NoError(t, srv.EnsureReleaseAndBinding(context.Background(), "acme", "myproject", componentName, environment,
			nil, nil, nil, componentTypeConfigs))
		require.NotNil(t, written.Spec, "expected the binding to be written")
		require.NotNil(t, written.Spec.ComponentTypeEnvironmentConfigs)
		return *written.Spec.ComponentTypeEnvironmentConfigs
	}

	httpCheck := "HTTP"
	deployedBuildsChecks := &models.HealthChecks{Readiness: &models.HealthCheck{Type: &httpCheck}}

	for _, path := range []struct {
		name     string
		adoption bool
	}{{"existing-binding update path", false}, {"409-conflict-adoption path", true}} {
		t.Run(path.name+": the deployed build's checks replace the earlier build's", func(t *testing.T) {
			writtenConfigs := configsWrittenByEnsureReleaseAndBinding(t, path.adoption, map[string]interface{}{"healthChecks": deployedBuildsChecks})
			assert.Equal(t, map[string]interface{}{"readiness": map[string]interface{}{"type": "HTTP"}}, writtenConfigs["healthChecks"])
			assert.Equal(t, float64(2), writtenConfigs["replicas"], "other configs are kept")
		})
		t.Run(path.name+": a build without checks removes the earlier build's", func(t *testing.T) {
			writtenConfigs := configsWrittenByEnsureReleaseAndBinding(t, path.adoption, map[string]interface{}{"healthChecks": nil})
			assert.NotContains(t, writtenConfigs, "healthChecks")
			assert.Equal(t, float64(2), writtenConfigs["replicas"], "other configs are kept")
		})
		t.Run(path.name+": no healthChecks key keeps the current ones", func(t *testing.T) {
			writtenConfigs := configsWrittenByEnsureReleaseAndBinding(t, path.adoption, map[string]interface{}{})
			assert.Equal(t, map[string]interface{}{"readiness": map[string]interface{}{"type": "TCP"}}, writtenConfigs["healthChecks"])
			assert.Equal(t, float64(2), writtenConfigs["replicas"], "other configs are kept")
		})
	}

	// The environment has no binding yet, so it is created. A build without checks sends an
	// explicit nil, which must be left out of the create rather than written as null.
	t.Run("new binding: a build without checks writes no healthChecks key", func(t *testing.T) {
		var created map[string]interface{}
		srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/generate-release"):
				w.WriteHeader(http.StatusCreated)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "release-2"}}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: []gen.ReleaseBinding{}}))
			case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				require.NoError(t, json.NewDecoder(r.Body).Decode(&created))
				w.WriteHeader(http.StatusCreated)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: bindingName}}))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		componentTypeConfigs := map[string]interface{}{"healthChecks": nil, "runtimeClassName": "gvisor"}
		require.NoError(t, srv.EnsureReleaseAndBinding(context.Background(), "acme", "myproject", componentName, environment,
			nil, nil, nil, componentTypeConfigs))

		spec, ok := created["spec"].(map[string]interface{})
		require.True(t, ok, "expected the binding to be created")
		writtenConfigs, ok := spec["componentTypeEnvironmentConfigs"].(map[string]interface{})
		require.True(t, ok)
		assert.NotContains(t, writtenConfigs, "healthChecks")
		assert.Equal(t, "gvisor", writtenConfigs["runtimeClassName"], "other configs are kept")
		assert.Contains(t, componentTypeConfigs, "healthChecks", "the caller's map is not changed")
	})
}

// Promote moves the source environment's build to the target, and the build's health checks
// travel with it, whatever the caller passed for the target's own configuration. Health checks
// sit in the binding's component-type configs, which promote otherwise leaves alone (normal
// target) or replaces wholesale (gVisor target); either way the target ends up with the
// source's health checks, or none when the source has none.
func TestPromoteComponent_CarriesTheSourcesHealthChecks(t *testing.T) {
	const componentName = "myagent"
	sourceChecks := map[string]interface{}{"readiness": map[string]interface{}{"type": "HTTP", "path": "/health"}}
	olderBuildChecks := map[string]interface{}{"readiness": map[string]interface{}{"type": "TCP"}}

	binding := func(environment string, componentTypeConfigs map[string]interface{}) gen.ReleaseBinding {
		b := gen.ReleaseBinding{
			Metadata: gen.ObjectMeta{Name: componentName + "-" + environment},
			Spec: &gen.ReleaseBindingSpec{
				Environment: environment,
				Owner: struct {
					ComponentName string `json:"componentName"`
					ProjectName   string `json:"projectName"`
				}{ComponentName: componentName, ProjectName: "myproject"},
			},
		}
		if environment == "default" {
			release := "release-from-build-1"
			b.Spec.ReleaseName = &release
		}
		if componentTypeConfigs != nil {
			b.Spec.ComponentTypeEnvironmentConfigs = &componentTypeConfigs
		}
		return b
	}

	// configsWrittenByPromote promotes default -> sandbox against a fake OpenChoreo and returns
	// the component-type configs written to sandbox's binding. target is nil when sandbox has
	// no binding yet (first promote, so one is created).
	configsWrittenByPromote := func(t *testing.T, source gen.ReleaseBinding, target *gen.ReleaseBinding, componentTypeConfigs map[string]interface{}) map[string]interface{} {
		t.Helper()
		var written gen.ReleaseBinding
		srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
				items := []gen.ReleaseBinding{source}
				if target != nil {
					items = append(items, *target)
				}
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: items}))
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings/"+componentName+"-sandbox"):
				if target == nil {
					w.WriteHeader(http.StatusNotFound)
					require.NoError(t, json.NewEncoder(w).Encode(gen.NotFound{Error: "not found"}))
					return
				}
				w.WriteHeader(http.StatusOK)
				require.NoError(t, json.NewEncoder(w).Encode(*target))
			case (r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/releasebindings/"+componentName+"-sandbox")) ||
				(r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releasebindings")):
				require.NoError(t, json.NewDecoder(r.Body).Decode(&written))
				w.WriteHeader(map[string]int{http.MethodPut: http.StatusOK, http.MethodPost: http.StatusCreated}[r.Method])
				require.NoError(t, json.NewEncoder(w).Encode(written))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		require.NoError(t, srv.PromoteComponent(context.Background(), "acme", "myproject", componentName,
			"default", "sandbox", nil, nil, nil, componentTypeConfigs))
		require.NotNil(t, written.Spec, "expected sandbox's binding to be written")
		require.NotNil(t, written.Spec.ComponentTypeEnvironmentConfigs)
		return *written.Spec.ComponentTypeEnvironmentConfigs
	}

	t.Run("normal target: the source's checks replace an older build's, other configs kept", func(t *testing.T) {
		target := binding("sandbox", map[string]interface{}{"replicas": float64(2), "healthChecks": olderBuildChecks})
		writtenConfigs := configsWrittenByPromote(t, binding("default", map[string]interface{}{"healthChecks": sourceChecks}), &target, nil)
		assert.Equal(t, sourceChecks, writtenConfigs["healthChecks"])
		assert.Equal(t, float64(2), writtenConfigs["replicas"], "other configs are kept")
	})

	t.Run("normal target: a source without checks removes an older build's", func(t *testing.T) {
		target := binding("sandbox", map[string]interface{}{"replicas": float64(2), "healthChecks": olderBuildChecks})
		writtenConfigs := configsWrittenByPromote(t, binding("default", nil), &target, nil)
		assert.NotContains(t, writtenConfigs, "healthChecks")
		assert.Equal(t, float64(2), writtenConfigs["replicas"], "other configs are kept")
	})

	t.Run("gVisor target: configs replaced as before, plus the source's checks", func(t *testing.T) {
		target := binding("sandbox", map[string]interface{}{"healthChecks": olderBuildChecks})
		passed := map[string]interface{}{"runtimeClassName": "gvisor"}
		writtenConfigs := configsWrittenByPromote(t, binding("default", map[string]interface{}{"healthChecks": sourceChecks}), &target, passed)
		assert.Equal(t, "gvisor", writtenConfigs["runtimeClassName"])
		assert.Equal(t, sourceChecks, writtenConfigs["healthChecks"])
		assert.NotContains(t, passed, "healthChecks", "the caller's map is not changed")
	})

	t.Run("first promote: the new binding gets the source's checks", func(t *testing.T) {
		writtenConfigs := configsWrittenByPromote(t, binding("default", map[string]interface{}{"healthChecks": sourceChecks}), nil, nil)
		assert.Equal(t, sourceChecks, writtenConfigs["healthChecks"])
	})
}

// A deployment reports the health checks its environment runs, read from the binding where
// the build's checks were written, so the deploy page can show them per environment.
func TestToDeploymentDetailsResponse_ReportsTheEnvironmentsHealthChecks(t *testing.T) {
	bindingWith := func(componentTypeConfigs map[string]interface{}) *gen.ReleaseBinding {
		binding := &gen.ReleaseBinding{
			Metadata: gen.ObjectMeta{Name: "myagent-default"},
			Spec:     &gen.ReleaseBindingSpec{Environment: "default"},
		}
		if componentTypeConfigs != nil {
			binding.Spec.ComponentTypeEnvironmentConfigs = &componentTypeConfigs
		}
		return binding
	}
	deploymentOf := func(t *testing.T, binding *gen.ReleaseBinding) *models.DeploymentResponse {
		t.Helper()
		deployment, err := toDeploymentDetailsResponse(binding, nil, nil, nil, nil, "", runtimeReplicaState{})
		require.NoError(t, err)
		return deployment
	}

	t.Run("the binding's health checks are reported", func(t *testing.T) {
		deployment := deploymentOf(t, bindingWith(map[string]interface{}{
			"replicas":     float64(1),
			"healthChecks": map[string]interface{}{"readiness": map[string]interface{}{"type": "HTTP", "path": "/health"}},
		}))
		require.NotNil(t, deployment.HealthChecks)
		require.NotNil(t, deployment.HealthChecks.Readiness)
		assert.Equal(t, "/health", *deployment.HealthChecks.Readiness.Path)
	})

	t.Run("a binding without health checks reports none, so the defaults apply", func(t *testing.T) {
		assert.Nil(t, deploymentOf(t, bindingWith(nil)).HealthChecks)
		assert.Nil(t, deploymentOf(t, bindingWith(map[string]interface{}{"replicas": float64(1)})).HealthChecks)
	})

	t.Run("an unreadable value is left out instead of failing the deployment", func(t *testing.T) {
		assert.Nil(t, deploymentOf(t, bindingWith(map[string]interface{}{"healthChecks": "not an object"})).HealthChecks)
	})
}

func TestEndpointServiceURL(t *testing.T) {
	port := int32(8080)
	binding := func(project, component, env string, eps *[]gen.EndpointURLStatus) gen.ReleaseBinding {
		b := bindingWithConfigs(nil, nil)
		b.Metadata.Name = component + "-" + env
		b.Spec.Environment = env
		b.Spec.Owner.ProjectName = project
		b.Spec.Owner.ComponentName = component
		b.Status = &gen.ReleaseBindingStatus{Endpoints: eps}
		return b
	}
	serve := func(t *testing.T, items ...gen.ReleaseBinding) *openChoreoClient {
		t.Helper()
		return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.True(t, strings.HasSuffix(r.URL.Path, "/releasebindings"), r.URL.Path)
			require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: items}))
		}))
	}
	twoEndpoints := &[]gen.EndpointURLStatus{
		{Name: "http", ServiceURL: &gen.EndpointURL{Host: "http.svc", Port: &port}},
		{Name: "mcp", ServiceURL: &gen.EndpointURL{Host: "mcp.svc", Port: &port}},
	}

	t.Run("PicksNamedEndpoint", func(t *testing.T) {
		c := serve(t, binding("proj", "orders", "dev", twoEndpoints))
		got, err := c.GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "")
		require.NoError(t, err)
		assert.Equal(t, "http://mcp.svc:8080", got)
	})

	t.Run("NotReconciled_Empty", func(t *testing.T) {
		c := serve(t, binding("proj", "orders", "dev", nil))
		got, err := c.GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("EndpointWithoutHost_Empty", func(t *testing.T) {
		c := serve(t, binding("proj", "orders", "dev", &[]gen.EndpointURLStatus{{Name: "mcp"}}))
		got, err := c.GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("UnknownEndpoint_ErrEndpointNotFound", func(t *testing.T) {
		c := serve(t, binding("proj", "orders", "dev", twoEndpoints))
		_, err := c.GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "nope", "dev", "")
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})

	t.Run("NoBinding_ErrReleaseBindingNotFound", func(t *testing.T) {
		c := serve(t, binding("proj", "orders", "prod", twoEndpoints))
		_, err := c.GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
		require.ErrorIs(t, err, utils.ErrNotFound)
	})

	t.Run("SameNamedComponentInAnotherProjectIsIgnored", func(t *testing.T) {
		other := binding("other-proj", "orders", "dev", &[]gen.EndpointURLStatus{
			{Name: "mcp", ServiceURL: &gen.EndpointURL{Host: "other.svc", Port: &port}},
		})
		mine := binding("proj", "orders", "dev", twoEndpoints)
		got, err := serve(t, other, mine).GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "")
		require.NoError(t, err)
		assert.Equal(t, "http://mcp.svc:8080", got)

		_, err = serve(t, other).GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	// IP keeps a binding per deployment track, so a component can have more than one in an environment.
	trackBinding := func(name, env, host string) gen.ReleaseBinding {
		b := binding("proj", "orders", env, &[]gen.EndpointURLStatus{
			{Name: "mcp", ServiceURL: &gen.EndpointURL{Host: host, Port: &port}},
		})
		b.Metadata.Name = name
		return b
	}
	main, beta := trackBinding("orders-main-dev", "dev", "main.svc"), trackBinding("orders-beta-dev", "dev", "beta.svc")

	t.Run("TwoBindingsWithoutName_ErrAmbiguousReleaseBinding", func(t *testing.T) {
		_, err := serve(t, main, beta).GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "")
		require.ErrorIs(t, err, ErrAmbiguousReleaseBinding)
		assert.Contains(t, err.Error(), "orders-main-dev")
		assert.Contains(t, err.Error(), "orders-beta-dev")
	})

	t.Run("NamePicksTheBinding", func(t *testing.T) {
		got, err := serve(t, main, beta).GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "orders-beta-dev")
		require.NoError(t, err)
		assert.Equal(t, "http://beta.svc:8080", got)
	})

	t.Run("NameOfAnotherEnvironment_ErrReleaseBindingMismatch", func(t *testing.T) {
		prod := trackBinding("orders-beta-prod", "prod", "prod.svc")
		_, err := serve(t, main, prod).GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "orders-beta-prod")
		require.ErrorIs(t, err, ErrReleaseBindingMismatch)
	})

	t.Run("NameOfAnotherProject_ErrReleaseBindingMismatch", func(t *testing.T) {
		other := trackBinding("orders-other-dev", "dev", "other.svc")
		other.Spec.Owner.ProjectName = "other-proj"
		_, err := serve(t, main, other).GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "orders-other-dev")
		require.ErrorIs(t, err, ErrReleaseBindingMismatch)
	})

	t.Run("UnknownName_ErrReleaseBindingNotFound", func(t *testing.T) {
		_, err := serve(t, main, beta).GetReleaseBindingEndpointServiceURL(context.Background(), "acme", "proj", "orders", "mcp", "dev", "orders-gone-dev")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})
}

func TestReleaseAgentAPIKeyTrait(t *testing.T) {
	envInjectionTrait := func(create map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"kind": "Trait", "name": string(TraitEnvInjection),
			"spec": map[string]interface{}{"creates": []interface{}{create}},
		}
	}
	externalSecret := map[string]interface{}{"kind": "ExternalSecret"}
	profile := func(params map[string]interface{}) *struct {
		Parameters *map[string]interface{} `json:"parameters,omitempty"`
		Traits     *[]gen.ComponentTrait   `json:"traits,omitempty"`
	} {
		traits := []gen.ComponentTrait{{Name: string(TraitEnvInjection), InstanceName: "a-" + string(TraitEnvInjection), Parameters: &params}}
		return &struct {
			Parameters *map[string]interface{} `json:"parameters,omitempty"`
			Traits     *[]gen.ComponentTrait   `json:"traits,omitempty"`
		}{Traits: &traits}
	}

	t.Run("trait cut before the env var key always fetches its path", func(t *testing.T) {
		traits := []map[string]interface{}{envInjectionTrait(map[string]interface{}{"template": externalSecret})}
		got := releaseAgentAPIKeyTrait(&gen.ComponentRelease{Spec: &gen.ComponentReleaseSpec{
			Traits:           &traits,
			ComponentProfile: profile(map[string]interface{}{"agentApiKeySecretRef": "kv/dev", "agentApiKeySecretProperty": "api-key"}),
		}})
		assert.Equal(t, ReleaseAgentAPIKeyTrait{Attached: true, SecretRef: "kv/dev", SecretProperty: "api-key"}, got)
	})

	t.Run("trait whose ExternalSecret is gated takes the env var key", func(t *testing.T) {
		traits := []map[string]interface{}{envInjectionTrait(map[string]interface{}{
			"includeWhen": `${parameters.agentApiKeySecretRef != ""}`, "template": externalSecret,
		})}
		got := releaseAgentAPIKeyTrait(&gen.ComponentRelease{Spec: &gen.ComponentReleaseSpec{
			Traits:           &traits,
			ComponentProfile: profile(map[string]interface{}{}),
		}})
		assert.Equal(t, ReleaseAgentAPIKeyTrait{Attached: true, TakesEnvVarKey: true}, got)
	})

	t.Run("release without the trait takes the env var key", func(t *testing.T) {
		got := releaseAgentAPIKeyTrait(&gen.ComponentRelease{Spec: &gen.ComponentReleaseSpec{}})
		assert.Equal(t, ReleaseAgentAPIKeyTrait{TakesEnvVarKey: true}, got)
	})
}

func TestBindingAgentAPIKeySecretRef(t *testing.T) {
	envInjKey := "myagent-" + string(TraitEnvInjection)
	withConfigs := func(cfgs map[string]interface{}) *gen.ReleaseBinding {
		return &gen.ReleaseBinding{Spec: &gen.ReleaseBindingSpec{TraitEnvironmentConfigs: &cfgs}}
	}

	assert.Equal(t, "", bindingAgentAPIKeySecretRef(nil, "myagent"))
	assert.Equal(t, "", bindingAgentAPIKeySecretRef(withConfigs(map[string]interface{}{}), "myagent"))
	assert.Equal(t, "kv/staging", bindingAgentAPIKeySecretRef(withConfigs(map[string]interface{}{
		envInjKey: map[string]interface{}{"agentApiKeySecretRef": "kv/staging"},
	}), "myagent"))
}
