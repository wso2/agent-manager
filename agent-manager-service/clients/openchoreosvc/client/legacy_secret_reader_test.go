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
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

func TestReadLegacyManagedSecret(t *testing.T) {
	for _, tt := range []struct {
		name   string
		owner  string
		org    string
		value  []byte
		errMsg string
	}{
		{name: "decodes base64", owner: "amp-agent-manager", value: []byte("first line\nsecond line")},
		{name: "other owner", owner: "external", value: []byte("private"), errMsg: secretmanagersvc.ErrNotManaged.Error()},
		{name: "other organization", owner: "amp-agent-manager", org: "other", value: []byte("private"), errMsg: "different organization"},
		{name: "binary value", owner: "amp-agent-manager", value: []byte{0xff}, errMsg: "non-UTF-8"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1alpha1/namespaces/default/secrets/agent-secret", r.URL.Path)
				labels := map[string]string{"managed-by": tt.owner, string(LabelKeyOrgUUID): tt.org}
				data := map[string][]byte{"config": tt.value}
				assert.NoError(t, json.NewEncoder(w).Encode(gen.Secret{
					Metadata: gen.ObjectMeta{Name: "agent-secret", Labels: &labels},
					Data:     &data,
				}))
			}))
			data, err := client.ReadLegacyManagedSecret(context.Background(), "tenant", "agent-secret")
			if tt.errMsg != "" {
				require.ErrorContains(t, err, tt.errMsg)
				require.Empty(t, data)
				return
			}
			require.NoError(t, err)
			require.Equal(t, map[string]string{"config": string(tt.value)}, data)
		})
	}
}
