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
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"time"
)

// YAML file name prefixes, by the kind the gateway knows the artifact as. The single pulls and the batch fetch name
// files the same way.
const (
	apiFilePrefix         = "api-"
	llmProviderFilePrefix = "llm-provider-"
	llmProxyFilePrefix    = "llm-proxy-"
	mcpProxyFilePrefix    = "mcp-proxy-"
	agentFilePrefix       = "agent-"
)

var batchFilePrefixes = map[string]string{
	"LlmProvider": llmProviderFilePrefix,
	"LlmProxy":    llmProxyFilePrefix,
	"Mcp":         mcpProxyFilePrefix,
	"Agent":       agentFilePrefix,
}

// BatchDeployment is one deployment in a gateway's batch fetch: its YAML, and the kind and id the gateway knows its
// artifact by.
type BatchDeployment struct {
	DeploymentID string
	Kind         string
	ArtifactID   string
	Content      []byte
}

// CreateBatchDeploymentTarGz packs deployments for POST /deployments/fetch-batch: a tar.gz with one directory per
// deployment id, holding the artifact's YAML. The gateway reads the deployment id from the directory name.
func CreateBatchDeploymentTarGz(deployments []BatchDeployment) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	modTime := time.Now()

	for _, d := range deployments {
		prefix, ok := batchFilePrefixes[d.Kind]
		if !ok {
			prefix = apiFilePrefix
		}
		header := &tar.Header{
			Name:    fmt.Sprintf("%s/%s%s.yaml", d.DeploymentID, prefix, d.ArtifactID),
			Mode:    0o644,
			Size:    int64(len(d.Content)),
			ModTime: modTime,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("failed to write tar header for %s: %w", header.Name, err)
		}
		if _, err := tarWriter.Write(d.Content); err != nil {
			return nil, fmt.Errorf("failed to write tar content for %s: %w", header.Name, err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

// CreateAPIYamlZip creates a ZIP file containing API YAML files
// Compatible with api-platform's implementation
func CreateAPIYamlZip(apiYamlMap map[string]string) ([]byte, error) {
	return createYamlZip(apiFilePrefix, apiYamlMap)
}

// CreateLLMProviderYamlZip creates a ZIP file containing LLM provider YAML files
func CreateLLMProviderYamlZip(providerYamlMap map[string]string) ([]byte, error) {
	return createYamlZip(llmProviderFilePrefix, providerYamlMap)
}

// CreateLLMProxyYamlZip creates a ZIP file containing LLM proxy YAML files
func CreateLLMProxyYamlZip(proxyYamlMap map[string]string) ([]byte, error) {
	return createYamlZip(llmProxyFilePrefix, proxyYamlMap)
}

// CreateMCPProxyYamlZip creates a ZIP file containing MCP proxy YAML files.
func CreateMCPProxyYamlZip(proxyYamlMap map[string]string) ([]byte, error) {
	return createYamlZip(mcpProxyFilePrefix, proxyYamlMap)
}

// CreateAgentYamlZip creates a ZIP file containing A2A Agent YAML files.
func CreateAgentYamlZip(agentYamlMap map[string]string) ([]byte, error) {
	return createYamlZip(agentFilePrefix, agentYamlMap)
}

// createYamlZip writes each YAML as <prefix><id>.yaml at the zip root.
func createYamlZip(fileNamePrefix string, yamlMap map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for id, yamlContent := range yamlMap {
		fileWriter, err := zipWriter.Create(fmt.Sprintf("%s%s.yaml", fileNamePrefix, id))
		if err != nil {
			return nil, closeAfterZipError(zipWriter, fmt.Errorf("failed to create file in zip: %w", err))
		}
		if _, err := fileWriter.Write([]byte(yamlContent)); err != nil {
			return nil, closeAfterZipError(zipWriter, fmt.Errorf("failed to write file content: %w", err))
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func closeAfterZipError(zipWriter *zip.Writer, err error) error {
	if closeErr := zipWriter.Close(); closeErr != nil {
		return errors.Join(err, fmt.Errorf("close error: %w", closeErr))
	}
	return err
}
