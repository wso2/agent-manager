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

package utils

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gateway fetches the Agent definition as a ZIP and pulls the YAML out of
// it (FetchResourceZip then ExtractYAMLFromZip), so the response shape is fixed
// by that code.
func TestCreateAgentYamlZip(t *testing.T) {
	yamlBody := "apiVersion: gateway.api-platform.wso2.com/v1\nkind: Agent\n"
	data, err := CreateAgentYamlZip(map[string]string{"agent-uuid-1": yamlBody})
	require.NoError(t, err)

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, reader.File, 1)
	assert.Equal(t, "agent-agent-uuid-1.yaml", reader.File[0].Name)

	rc, err := reader.File[0].Open()
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, yamlBody, string(content))
}
