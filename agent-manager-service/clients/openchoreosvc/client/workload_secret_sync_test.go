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

// TestExternalSecretSyncedRevision rejects acknowledgements for an earlier revision.
func TestExternalSecretSyncedRevision(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"generation":  float64(1),
			"annotations": map[string]any{"force-sync": "test-revision"},
			"labels":      map[string]any{"app": "agent"},
		},
		"status": map[string]any{
			"syncedResourceVersion": "1-ce9063c0777c3deca21a2209131773cc3816e75a5761b32b44ff91ac",
			"conditions": []any{map[string]any{
				"type": "Ready", "status": "True", "reason": "SecretSynced",
			}},
		},
	}
	if !externalSecretSyncedRevision(obj, "test-revision") {
		t.Fatal("expected ESO's successful acknowledgement to match the requested revision")
	}
	metadata := obj["metadata"].(map[string]any)
	metadata["annotations"].(map[string]any)["force-sync"] = "next-revision"
	if externalSecretSyncedRevision(obj, "next-revision") {
		t.Fatal("an applied annotation and the previous Ready status must not acknowledge a new revision")
	}
}

// TestWorkloadSecretSyncTimeoutError reports pending resources without exposing provider details.
func TestWorkloadSecretSyncTimeoutError(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{"force-sync": "revision"},
		},
		"status": map[string]any{
			"syncedResourceVersion": "old-ack",
			"conditions": []any{map[string]any{
				"type": "Ready", "status": "False", "reason": "SecretSyncedError",
				"message": "private provider detail",
			}},
		},
	}
	err := workloadSecretSyncTimeoutError("revision", []string{workloadSecretSyncObservation("agent-env", obj)})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorContains(t, err, "agent-env")
	assert.ErrorContains(t, err, "SecretSyncedError")
	assert.ErrorContains(t, err, "old-ack")
	assert.NotContains(t, err.Error(), "private provider detail")
}

func externalSecretWritingKeys(keys ...string) map[string]any {
	data := make([]any, 0, len(keys))
	for _, key := range keys {
		data = append(data, map[string]any{
			"secretKey": key,
			"remoteRef": map[string]any{"key": "wc-secrets/ns/generic/my-agent-dev-secrets", "property": key},
		})
	}
	return map[string]any{
		"metadata": map[string]any{"name": "my-agent-dev-env-secrets", "annotations": map[string]any{"force-sync": ""}},
		"spec":     map[string]any{"data": data},
	}
}

// TestReferencesEditedSecret matches on the key the ExternalSecret writes.
func TestReferencesEditedSecret(t *testing.T) {
	assert.True(t, referencesEditedSecret(externalSecretWritingKeys("OPENAI_API_KEY", "DB_PASSWORD"), []string{"DB_PASSWORD"}))
	assert.True(t, referencesEditedSecret(externalSecretWritingKeys("config.json"), []string{"config.json"}), "file secrets use the file name as secretKey")
	assert.False(t, referencesEditedSecret(externalSecretWritingKeys("OPENAI_API_KEY"), []string{"DB_PASSWORD"}))
	assert.False(t, referencesEditedSecret(externalSecretWritingKeys(), []string{"DB_PASSWORD"}))
	assert.False(t, referencesEditedSecret(map[string]any{"spec": map[string]any{"data": []any{"not-an-entry"}}}, []string{"DB_PASSWORD"}))
}

// TestSyncEditedWorkloadSecretsDoesNotReadSecretReferences keeps the sync off
// the SecretReferences API, which some deployments do not expose.
func TestSyncEditedWorkloadSecretsDoesNotReadSecretReferences(t *testing.T) {
	const bindingName = "my-agent-dev"
	group := "external-secrets.io"
	binding := gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: bindingName},
		Spec:     &gen.ReleaseBindingSpec{Environment: "dev"},
	}
	tree := gen.K8sResourceTreeResponse{RenderedReleases: []gen.ReleaseResourceTree{{Nodes: []gen.ResourceNode{{
		Kind: "ExternalSecret", Group: &group, Name: "my-agent-dev-env-secrets", Uid: "es-1",
		Object: externalSecretWritingKeys("OPENAI_API_KEY"),
	}}}}}

	var paths []string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "/secretreferences"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings"):
			require.NoError(t, json.NewEncoder(w).Encode(gen.ReleaseBindingList{Items: []gen.ReleaseBinding{binding}}))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releasebindings/"+bindingName+"/k8sresources/tree"):
			require.NoError(t, json.NewEncoder(w).Encode(tree))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	err := c.SyncEditedWorkloadSecrets(context.Background(), "default", "my-agent", "dev", []string{"DB_PASSWORD"})

	require.NoError(t, err, "no live ExternalSecret writes the edited key, so there is nothing to sync")
	for _, p := range paths {
		assert.NotContains(t, p, "secretreferences")
	}
}
