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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extractLikeTheGateway reads a batch archive the way gateway v2026.09.24 does
// (pkg/utils/api_utils.go ExtractDeploymentsFromBatchZip): directories are skipped, the deployment id is the name of
// the directory holding the file, and only .yaml or .yml files count.
func extractLikeTheGateway(t *testing.T, archive []byte) (map[string][]byte, []string) {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	deployments := map[string][]byte{}
	var names []string
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		names = append(names, header.Name)
		if header.Typeflag == tar.TypeDir {
			continue
		}
		cleanPath := filepath.Clean(header.Name)
		deploymentID := filepath.Base(filepath.Dir(cleanPath))
		if deploymentID == "." || deploymentID == "" {
			continue
		}
		if ext := filepath.Ext(cleanPath); ext != ".yaml" && ext != ".yml" {
			continue
		}
		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		deployments[deploymentID] = content
	}
	return deployments, names
}

func TestCreateBatchDeploymentTarGzLayout(t *testing.T) {
	archive, err := CreateBatchDeploymentTarGz([]BatchDeployment{
		{DeploymentID: "d-provider", Kind: "LlmProvider", ArtifactID: "a-1", Content: []byte("kind: LlmProvider\n")},
		{DeploymentID: "d-proxy", Kind: "LlmProxy", ArtifactID: "proxy-handle", Content: []byte("kind: LlmProxy\n")},
		{DeploymentID: "d-mcp", Kind: "Mcp", ArtifactID: "a-3", Content: []byte("kind: Mcp\n")},
		{DeploymentID: "d-agent", Kind: "Agent", ArtifactID: "a-4", Content: []byte("kind: Agent\n")},
	})
	require.NoError(t, err)

	deployments, names := extractLikeTheGateway(t, archive)

	assert.Equal(t, []string{
		"d-provider/llm-provider-a-1.yaml",
		"d-proxy/llm-proxy-proxy-handle.yaml",
		"d-mcp/mcp-proxy-a-3.yaml",
		"d-agent/agent-a-4.yaml",
	}, names, "one directory per deployment id, each file named as the single pull names it")
	assert.Equal(t, map[string][]byte{
		"d-provider": []byte("kind: LlmProvider\n"),
		"d-proxy":    []byte("kind: LlmProxy\n"),
		"d-mcp":      []byte("kind: Mcp\n"),
		"d-agent":    []byte("kind: Agent\n"),
	}, deployments)
}

func TestCreateBatchDeploymentTarGzEmpty(t *testing.T) {
	archive, err := CreateBatchDeploymentTarGz(nil)
	require.NoError(t, err)

	deployments, names := extractLikeTheGateway(t, archive)

	assert.Empty(t, names)
	assert.Empty(t, deployments)
}
