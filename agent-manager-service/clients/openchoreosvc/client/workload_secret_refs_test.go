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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func TestDeleteAgentSecretReference(t *testing.T) {
	const prefix = "/api/v1/namespaces/default/"
	const refName = "agent-secret"
	for _, tt := range []struct {
		name       string
		labels     map[string]string
		remoteKey  string
		getStatus  int
		deletePath string
	}{
		{
			name:       "OpenBao-backed reference",
			labels:     map[string]string{"managed-by": "amp-agent-manager", "openchoreo.dev/managed-by": "openchoreo-api"},
			getStatus:  http.StatusOK,
			deletePath: "/api/v1alpha1/namespaces/default/secrets/" + refName,
		},
		{
			name:       "early in-house reference",
			labels:     map[string]string{"openchoreo.dev/project": "project", "openchoreo.dev/component": "agent"},
			remoteKey:  "tenant/agent-secret",
			getStatus:  http.StatusOK,
			deletePath: prefix + "secretreferences/" + refName,
		},
		{
			name:      "other owner",
			labels:    map[string]string{"managed-by": "external"},
			getStatus: http.StatusOK,
		},
		{
			name:      "owned by a different agent",
			labels:    map[string]string{"managed-by": "amp-agent-manager", "openchoreo.dev/component": "other"},
			getStatus: http.StatusOK,
		},
		{
			name:      "another tenant's store key",
			labels:    map[string]string{"openchoreo.dev/project": "project", "openchoreo.dev/component": "agent"},
			remoteKey: "other/agent-secret",
			getStatus: http.StatusOK,
		},
		{name: "already absent", getStatus: http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deleted := make(chan string, 2)
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					assert.Equal(t, prefix+"secretreferences/"+refName, r.URL.Path)
					w.WriteHeader(tt.getStatus)
					if tt.getStatus == http.StatusOK {
						ref := gen.SecretReference{
							Metadata: gen.ObjectMeta{Name: refName, Labels: &tt.labels},
							Spec: &gen.SecretReferenceSpec{
								Data: []gen.SecretDataSource{{RemoteRef: gen.RemoteReference{Key: tt.remoteKey}}},
							},
						}
						assert.NoError(t, json.NewEncoder(w).Encode(ref))
					} else {
						assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{"message": "not found"}))
					}
				case http.MethodDelete:
					deleted <- r.URL.Path
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected HTTP method %s", r.Method)
				}
			}))

			require.NoError(t, client.DeleteAgentSecretReference(context.Background(), "tenant", "project", "agent", refName))
			if tt.deletePath == "" {
				assert.Empty(t, deleted)
			} else {
				require.Len(t, deleted, 1)
				assert.Equal(t, tt.deletePath, <-deleted)
			}
		})
	}
}
