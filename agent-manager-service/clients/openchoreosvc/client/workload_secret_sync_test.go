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
	"testing"

	"github.com/stretchr/testify/assert"
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
