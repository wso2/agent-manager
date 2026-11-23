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
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedFallback(t *testing.T) {
	reloadAfter(t)
	dir := t.TempDir()
	t.Setenv("AMP_SECRETS_DIR", dir)
	unset(t, "IDP_CLIENT_SECRET", "AMP_API_CLIENT_SECRET", "LOG_LEVEL")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "amp-api-client-secret"), []byte("generated\n"), 0o600))
	require.NoError(t, loadWith(t, "server: {port: 8080}\n"))

	assert.Equal(t, "generated", Value("IDP_CLIENT_SECRET"), "an unset secret falls back to its generated file")
	assert.Equal(t, "generated", GetConfig().IDP.ClientSecret)

	t.Setenv("IDP_CLIENT_SECRET", "")
	assert.Equal(t, "generated", Value("IDP_CLIENT_SECRET"), "an empty secret counts as unset")

	t.Setenv("IDP_CLIENT_SECRET", "mine")
	assert.Equal(t, "mine", Value("IDP_CLIENT_SECRET"), "a set secret wins")

	t.Setenv("LOG_LEVEL", "")
	_, ok := Lookup("LOG_LEVEL")
	assert.True(t, ok, "an empty setting that is not a generated secret is still set")

	path, ok := GeneratedFile("IDP_CLIENT_SECRET")
	assert.True(t, ok)
	assert.Equal(t, filepath.Join(dir, "amp-api-client-secret"), path)
	_, ok = GeneratedFile("LOG_LEVEL")
	assert.False(t, ok)
}

func TestGeneratedMissingFile(t *testing.T) {
	reloadAfter(t)
	t.Setenv("AMP_SECRETS_DIR", t.TempDir())
	unset(t, "IDP_CLIENT_SECRET", "AMP_API_CLIENT_SECRET")
	require.NoError(t, loadWith(t, "server: {port: 8080}\n"))
	assert.Equal(t, "", Value("IDP_CLIENT_SECRET"), "no file and no value: unset, not the old literal default")
}
