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

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestA2AProxyTable(t *testing.T) {
	assert.Equal(t, "a2a_proxies", A2AProxy{}.TableName())
}

// The Agent YAML carries the ciphertext as secretRef; the internal API decrypts it for the gateway.
func TestA2AProxyUpstreamAuth(t *testing.T) {
	assert.Nil(t, (&A2AProxy{}).UpstreamAuth(), "no header, no upstream auth")
	assert.Nil(t, (&A2AProxy{UpstreamAuthHeader: "X-Key"}).UpstreamAuth(), "a header without a secret is not auth")

	auth := (&A2AProxy{UpstreamAuthHeader: "X-Key", UpstreamAuthSecret: "c2VjcmV0"}).UpstreamAuth()
	require.NotNil(t, auth)
	assert.Equal(t, "api-key", *auth.Type)
	assert.Equal(t, "X-Key", *auth.Header)
	assert.Equal(t, "c2VjcmV0", *auth.SecretRef)
	assert.Nil(t, auth.Value, "the plaintext never reaches the YAML AMS stores")
}
