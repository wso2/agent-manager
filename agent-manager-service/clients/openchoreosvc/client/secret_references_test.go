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
//

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

// secretEnvVar builds an env var whose value comes from a SecretReference.
func secretEnvVar(key, secretName string) gen.EnvVar {
	name := secretName
	return gen.EnvVar{
		Key: key,
		ValueFrom: &gen.EnvVarValueFrom{
			SecretKeyRef: &struct {
				Key  *string `json:"key,omitempty"`
				Name *string `json:"name,omitempty"`
			}{Name: &name},
		},
	}
}

// secretFileVar builds a file mount whose content comes from a SecretReference.
func secretFileVar(key, secretName string) gen.FileVar {
	name := secretName
	return gen.FileVar{
		Key:       key,
		MountPath: "/etc/" + key,
		ValueFrom: &gen.EnvVarValueFrom{
			SecretKeyRef: &struct {
				Key  *string `json:"key,omitempty"`
				Name *string `json:"name,omitempty"`
			}{Name: &name},
		},
	}
}

// serveLists answers the two list calls GetWorkloadSecretRefNames makes.
func serveLists(t *testing.T, bindings gen.ReleaseBindingList, workloads gen.WorkloadList) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releasebindings"):
			require.NoError(t, json.NewEncoder(w).Encode(bindings))
		case strings.HasSuffix(r.URL.Path, "/workloads"):
			require.NoError(t, json.NewEncoder(w).Encode(workloads))
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func bindingWithOverrides(env []gen.EnvVar, files []gen.FileVar) gen.ReleaseBinding {
	container := &gen.ContainerOverride{}
	if len(env) > 0 {
		container.Env = &env
	}
	if len(files) > 0 {
		container.Files = &files
	}
	return gen.ReleaseBinding{
		Spec: &gen.ReleaseBindingSpec{
			WorkloadOverrides: &gen.WorkloadOverrides{Container: container},
		},
	}
}

// Configuration lives on the per-environment ReleaseBindings, not the Workload, so deletion has
// to discover secrets there — and in every environment, since a deploy writes only the
// environment it targets.
func TestGetWorkloadSecretRefNames_CollectsFromAllBindings(t *testing.T) {
	bindings := gen.ReleaseBindingList{Items: []gen.ReleaseBinding{
		bindingWithOverrides([]gen.EnvVar{secretEnvVar("API_KEY", "dev-api-key")}, nil),
		bindingWithOverrides(
			[]gen.EnvVar{secretEnvVar("API_KEY", "prod-api-key")},
			[]gen.FileVar{secretFileVar("creds.json", "prod-creds")},
		),
	}}
	c := newTestClient(t, serveLists(t, bindings, gen.WorkloadList{}))

	names, err := c.GetWorkloadSecretRefNames(context.Background(), "acme", "my-project", "my-agent")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dev-api-key", "prod-api-key", "prod-creds"}, names)
}

// Agents created before configuration moved to the bindings still carry their refs on the
// Workload; both sources are unioned, and a ref present in both is reported once.
func TestGetWorkloadSecretRefNames_UnionsWorkloadAndDeduplicates(t *testing.T) {
	bindings := gen.ReleaseBindingList{Items: []gen.ReleaseBinding{
		bindingWithOverrides([]gen.EnvVar{secretEnvVar("API_KEY", "shared-secret")}, nil),
	}}
	workloadEnv := []gen.EnvVar{secretEnvVar("API_KEY", "shared-secret"), secretEnvVar("LEGACY", "legacy-secret")}
	workloads := gen.WorkloadList{Items: []gen.Workload{{
		Spec: &gen.WorkloadSpec{Container: &gen.WorkloadContainer{Image: "img", Env: &workloadEnv}},
	}}}
	c := newTestClient(t, serveLists(t, bindings, workloads))

	names, err := c.GetWorkloadSecretRefNames(context.Background(), "acme", "my-project", "my-agent")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"shared-secret", "legacy-secret"}, names)
}

// Bindings without overrides, and plain-valued config, contribute nothing.
func TestGetWorkloadSecretRefNames_NoSecrets(t *testing.T) {
	plain := "plain"
	bindings := gen.ReleaseBindingList{Items: []gen.ReleaseBinding{
		{Spec: &gen.ReleaseBindingSpec{}},
		bindingWithOverrides([]gen.EnvVar{{Key: "LOG_LEVEL", Value: &plain}}, nil),
	}}
	c := newTestClient(t, serveLists(t, bindings, gen.WorkloadList{}))

	names, err := c.GetWorkloadSecretRefNames(context.Background(), "acme", "my-project", "my-agent")

	require.NoError(t, err)
	assert.Empty(t, names)
}
