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

package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeGatewayManifest(t *testing.T) {
	push := func(body string) (map[string]interface{}, bool, int) {
		w := httptest.NewRecorder()
		manifest, ok := decodeGatewayManifest(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		return manifest, ok, w.Code
	}

	manifest, ok, _ := push(`{"mcp-auth":{"version":"v1"}}`)
	require.True(t, ok)
	assert.Contains(t, manifest, "mcp-auth")

	_, ok, code := push(`{"padding":"` + strings.Repeat("a", maxGatewayManifestBytes) + `"}`)
	assert.False(t, ok)
	assert.Equal(t, http.StatusRequestEntityTooLarge, code)

	_, ok, code = push("{")
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, code)
}
