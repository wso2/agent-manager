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

package gwutil

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gateway parses these zips by file name, so names, contents and bytes are pinned.
func TestYamlZipHelpersOutput(t *testing.T) {
	tests := []struct {
		name       string
		create     func(map[string]string) ([]byte, error)
		prefix     string
		singleHash string
	}{
		{name: "api", create: CreateAPIYamlZip, prefix: "api-", singleHash: "7801d93bb544b74171a5cdc967d98f124b4004057fba8006c0ab4cb825696874"},
		{name: "llm provider", create: CreateLLMProviderYamlZip, prefix: "llm-provider-", singleHash: "a6be7da7bcd56acb1e4b362f2d997eecfe08a268ad31626f71babf8a8f490eb8"},
		{name: "llm proxy", create: CreateLLMProxyYamlZip, prefix: "llm-proxy-", singleHash: "1bbbf15a66c27e244e59e1e483bed0e5d311d8e5faf3f4f7c5e8158a20e48c16"},
		{name: "mcp proxy", create: CreateMCPProxyYamlZip, prefix: "mcp-proxy-", singleHash: "3eece7d082a4b007beff81d951e2a8297e3d7712d3b8b3a3f5524f4d287402f3"},
		{name: "agent", create: CreateAgentYamlZip, prefix: "agent-", singleHash: "b521d3be3525e6405f9acce5e96d408c415b783b6b9f4dc09802608043a9e3f8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := map[string]string{"id-1": "kind: One\n", "id-2": "kind: Two\n"}
			data, err := tt.create(in)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				tt.prefix + "id-1.yaml": "kind: One\n",
				tt.prefix + "id-2.yaml": "kind: Two\n",
			}, readZipEntries(t, data))

			single, err := tt.create(map[string]string{"id-1": "kind: One\n"})
			require.NoError(t, err)
			sum := sha256.Sum256(single)
			assert.Equal(t, tt.singleHash, hex.EncodeToString(sum[:]))
		})
	}
}

func TestYamlZipHelpersEmptyMap(t *testing.T) {
	data, err := CreateAgentYamlZip(map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, readZipEntries(t, data))
}

func readZipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	entries := make(map[string]string, len(reader.File))
	for _, f := range reader.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		entries[f.Name] = string(content)
	}
	return entries
}
