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
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func strPtr(s string) *string { return &s }

func bindingSpecWith(env []gen.EnvVar, files []gen.FileVar, configs map[string]interface{}) *gen.ReleaseBindingSpec {
	spec := &gen.ReleaseBindingSpec{
		Environment:       "dev",
		WorkloadOverrides: &gen.WorkloadOverrides{Container: &gen.ContainerOverride{Env: &env, Files: &files}},
	}
	if configs != nil {
		spec.ComponentTypeEnvironmentConfigs = &configs
	}
	return spec
}

func TestEncodeBindingSecrets_MovesSecretsToDirectStoreKeys(t *testing.T) {
	spec := bindingSpecWith(
		[]gen.EnvVar{
			{Key: "PLAIN", Value: strPtr("v")},
			{Key: "OPENAI_API_KEY", ValueFrom: secretValueFrom("agent-dev-secrets", "OPENAI_API_KEY")},
		},
		[]gen.FileVar{{Key: "cert.pem", MountPath: "/etc/cert.pem", ValueFrom: secretValueFrom("agent-dev-secrets", "cert.pem")}},
		map[string]interface{}{"replicas": 2},
	)

	out, err := encodeBindingSecrets(spec, "ou-1")
	require.NoError(t, err)

	env := *out.WorkloadOverrides.Container.Env
	require.Len(t, env, 1, "only plain env vars reach OpenChoreo's overrides")
	assert.Equal(t, "PLAIN", env[0].Key)
	assert.Empty(t, *out.WorkloadOverrides.Container.Files)

	configs := *out.ComponentTypeEnvironmentConfigs
	assert.InDelta(t, 2, configs["replicas"], 0, "other configs are kept")
	raw, _ := json.Marshal(configs[bindingSecretEnvsField])
	assert.JSONEq(t, `[{"name":"OPENAI_API_KEY","secret":"agent-dev-secrets","property":"OPENAI_API_KEY","remoteKey":"ou-1/agent-dev-secrets"}]`, string(raw))
	raw, _ = json.Marshal(configs[bindingSecretFilesField])
	assert.JSONEq(t, `[{"key":"cert.pem","mountPath":"/etc/cert.pem","secret":"agent-dev-secrets","property":"cert.pem","remoteKey":"ou-1/agent-dev-secrets"}]`, string(raw))

	// The caller's spec is untouched, so a retried write encodes it again.
	assert.Len(t, *spec.WorkloadOverrides.Container.Env, 2)
	assert.NotContains(t, *spec.ComponentTypeEnvironmentConfigs, bindingSecretEnvsField)
}

func TestEncodeDecodeBindingSecrets_RoundTrip(t *testing.T) {
	spec := bindingSpecWith(
		[]gen.EnvVar{
			{Key: "PLAIN", Value: strPtr("v")},
			{Key: "TOKEN", ValueFrom: secretValueFrom("s1", "token")},
		},
		[]gen.FileVar{{Key: "f", MountPath: "/f", ValueFrom: secretValueFrom("s2", "f")}},
		nil,
	)
	out, err := encodeBindingSecrets(spec, "ou-1")
	require.NoError(t, err)

	decodeBindingSecrets(out)

	assert.NotContains(t, *out.ComponentTypeEnvironmentConfigs, bindingSecretEnvsField)
	assert.NotContains(t, *out.ComponentTypeEnvironmentConfigs, bindingSecretFilesField)
	env := *out.WorkloadOverrides.Container.Env
	require.Len(t, env, 2)
	name, key, ok := secretRef(env[1].ValueFrom)
	require.True(t, ok)
	assert.Equal(t, "TOKEN", env[1].Key)
	assert.Equal(t, "s1", name)
	assert.Equal(t, "token", key)
	files := *out.WorkloadOverrides.Container.Files
	require.Len(t, files, 1)
	name, key, ok = secretRef(files[0].ValueFrom)
	require.True(t, ok)
	assert.Equal(t, "/f", files[0].MountPath)
	assert.Equal(t, "s2", name)
	assert.Equal(t, "f", key)
}

func TestEncodeBindingSecrets_WithoutSecretsDropsTheFields(t *testing.T) {
	// A binding whose last secret was removed must not keep the old entries,
	// and a ComponentType that declares no secret fields must never see them.
	spec := bindingSpecWith([]gen.EnvVar{{Key: "PLAIN", Value: strPtr("v")}}, nil, map[string]interface{}{
		bindingSecretEnvsField:  []interface{}{map[string]interface{}{"name": "OLD"}},
		bindingSecretFilesField: []interface{}{},
	})
	out, err := encodeBindingSecrets(spec, "")
	require.NoError(t, err, "no secrets means no organization is needed")
	assert.NotContains(t, *out.ComponentTypeEnvironmentConfigs, bindingSecretEnvsField)
	assert.NotContains(t, *out.ComponentTypeEnvironmentConfigs, bindingSecretFilesField)
}

func TestEncodeBindingSecrets_SecretsWithoutOrganizationFail(t *testing.T) {
	spec := bindingSpecWith([]gen.EnvVar{{Key: "TOKEN", ValueFrom: secretValueFrom("s1", "token")}}, nil, nil)
	_, err := encodeBindingSecrets(spec, "")
	require.ErrorIs(t, err, errSecretOuIDMissing)
}

// TestOCAPI_TranslatesBindingsOnTheWire checks the wrapper end to end: a write
// sends direct store keys and no valueFrom, and a read shows valueFrom again.
func TestOCAPI_TranslatesBindingsOnTheWire(t *testing.T) {
	var sent gen.ReleaseBinding
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(body, &sent))
			_, _ = w.Write(body)
		case http.MethodGet:
			require.NoError(t, json.NewEncoder(w).Encode(sent))
		}
	}))
	spec := bindingSpecWith([]gen.EnvVar{{Key: "TOKEN", ValueFrom: secretValueFrom("s1", "token")}}, nil, nil)
	ctx := withSecretOuID(context.Background(), "ou-1")

	_, err := c.ocClient.UpdateReleaseBindingWithResponse(ctx, "default", "agent-dev", gen.ReleaseBinding{Spec: spec})
	require.NoError(t, err)

	for _, e := range *sent.Spec.WorkloadOverrides.Container.Env {
		assert.Nil(t, e.ValueFrom, "OpenChoreo never receives a secret valueFrom")
	}
	raw, _ := json.Marshal((*sent.Spec.ComponentTypeEnvironmentConfigs)[bindingSecretEnvsField])
	assert.Contains(t, string(raw), `"remoteKey":"ou-1/s1"`)

	got, err := c.ocClient.GetReleaseBindingWithResponse(context.Background(), "default", "agent-dev")
	require.NoError(t, err)
	env := *got.JSON200.Spec.WorkloadOverrides.Container.Env
	require.Len(t, env, 1)
	name, key, ok := secretRef(env[0].ValueFrom)
	require.True(t, ok)
	assert.Equal(t, "s1", name)
	assert.Equal(t, "token", key)
}

// releaseServer serves one ComponentRelease whose frozen ComponentType is
// componentType, and records the binding it is asked to store.
func releaseServer(t *testing.T, componentType string, sent *gen.ReleaseBinding) *openChoreoClient {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"metadata":{"name":"agent-r1"},"spec":{"componentType":` + componentType +
				`,"owner":{"componentName":"agent","projectName":"p"},"workload":{}}}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, sent))
		_, _ = w.Write(body)
	}))
}

func pinnedSecretBinding() gen.ReleaseBinding {
	spec := bindingSpecWith([]gen.EnvVar{{Key: "TOKEN", ValueFrom: secretValueFrom("s1", "token")}}, nil, nil)
	spec.ReleaseName = strPtr("agent-r1")
	return gen.ReleaseBinding{Spec: spec}
}

func TestOCAPI_LeavesSecretRefsOfLegacyReleaseUntouched(t *testing.T) {
	for name, componentType := range map[string]string{
		"spec":     `{"environmentConfigs":{"openAPIV3Schema":{"properties":{"secretSyncRevision":{"type":"string"}}}}}`,
		"resource": `{"spec":{"environmentConfigs":{"openAPIV3Schema":{"properties":{"secretSyncRevision":{"type":"string"}}}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var sent gen.ReleaseBinding
			c := releaseServer(t, componentType, &sent)

			// No organization on the context: a legacy binding is not encoded, so none is needed.
			_, err := c.ocClient.UpdateReleaseBindingWithResponse(context.Background(), "default", "agent-dev", pinnedSecretBinding())
			require.NoError(t, err)

			env := *sent.Spec.WorkloadOverrides.Container.Env
			require.Len(t, env, 1)
			secret, key, ok := secretRef(env[0].ValueFrom)
			require.True(t, ok, "the legacy release still reads the valueFrom reference")
			assert.Equal(t, "s1", secret)
			assert.Equal(t, "token", key)
			assert.Nil(t, sent.Spec.ComponentTypeEnvironmentConfigs)
		})
	}
}

func TestOCAPI_EncodesSecretRefsOfCurrentRelease(t *testing.T) {
	for name, componentType := range map[string]string{
		"declares secretEnvs": `{"environmentConfigs":{"openAPIV3Schema":{"properties":{"secretEnvs":{"type":"array"}}}}}`,
		"no readable schema":  `{"name":"deployment/agent-api"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var sent gen.ReleaseBinding
			c := releaseServer(t, componentType, &sent)

			_, err := c.ocClient.CreateReleaseBindingWithResponse(withSecretOuID(context.Background(), "ou-1"), "default", pinnedSecretBinding())
			require.NoError(t, err)

			assert.Empty(t, *sent.Spec.WorkloadOverrides.Container.Env)
			raw, _ := json.Marshal((*sent.Spec.ComponentTypeEnvironmentConfigs)[bindingSecretEnvsField])
			assert.Contains(t, string(raw), `"remoteKey":"ou-1/s1"`)
		})
	}
}

func TestOCAPI_FailsBindingWriteWhenReleaseIsUnreadable(t *testing.T) {
	writes := 0
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writes++
	}))

	_, err := c.ocClient.UpdateReleaseBindingWithResponse(withSecretOuID(context.Background(), "ou-1"), "default", "agent-dev", pinnedSecretBinding())
	require.Error(t, err)
	assert.Zero(t, writes, "the binding is not written without knowing its release")
}
