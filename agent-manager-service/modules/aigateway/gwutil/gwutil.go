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

// Package gwutil holds the AI Gateway's small helpers: path parameter names,
// pagination bounds, agent subtypes, secret references, deployment archives and
// name validation.
package gwutil

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/aesgcm"
)

const AgentTypeAPI AgentType = "agent-api"

const (
	AgentSubTypeChatAPI   AgentSubType = "chat-api"
	AgentSubTypeCustomAPI AgentSubType = "custom-api"
	// AgentSubTypeA2A is an agent that speaks the A2A protocol. It provisions
	// with the same agent-api component type as the other two but gets no REST
	// API: the api-configuration trait is not attached, and the agent is
	// published to the gateway as a kind: Agent resource instead.
	AgentSubTypeA2A AgentSubType = "a2a-agent"
)

const MaxResourceNameLength = 25

// Path parameters of the gateway's own routes.
const (
	PathParamProviderId   = "providerId"
	PathParamTemplateId   = "templateId"
	PathParamProxyId      = "proxyId"
	PathParamConfigId     = "configId"
	PathParamGatewayId    = "gatewayId"
	PathParamEnvID        = "envID"
	PathParamDeploymentId = "deploymentId"
	PathParamScopeAction  = "scopeAction"
)

// AgentType is an agent's component type.
type AgentType string

type AgentSubType string

// IsA2AAgentSubType reports whether a subtype string names an A2A agent. It
// exists so the several gates that branch on this subtype read the same and
// cannot drift apart.
func IsA2AAgentSubType(subType string) bool {
	return subType == string(AgentSubTypeA2A)
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

// ExponentialBackoff is min(base · 2^attempt, maxDelay).
func ExponentialBackoff(attempt int, base, maxDelay time.Duration) time.Duration {
	delay := base
	for i := 0; i < attempt && delay < maxDelay; i++ {
		delay *= 2
	}
	return min(delay, maxDelay)
}

// EncryptSecretRef encrypts plaintext with EncryptBytes into a base64 secretRef.
func EncryptSecretRef(plaintext string, key []byte) (string, error) {
	ciphertext, err := aesgcm.Encrypt([]byte(plaintext), key)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptSecretRef reverses EncryptSecretRef.
func DecryptSecretRef(ref string, key []byte) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ref)
	if err != nil {
		return "", fmt.Errorf("failed to base64-decode: %w", err)
	}
	plaintext, err := aesgcm.Decrypt(ciphertext, key)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// GenerateAPIKey generates a cryptographically secure random API key for gateway authentication.
// The API key is returned as a plain string (to be shown to the user once).
func GenerateAPIKey() (string, error) {
	keyBytes := make([]byte, APIKeyLength)
	if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}
	// Return as hex string for easier usage
	return hex.EncodeToString(keyBytes), nil
}

// GenerateHandle generates a URL-safe handle from a display name.
// This is a simplified version that sanitizes the input string.
func GenerateHandle(displayName string) (string, error) {
	if displayName == "" {
		return "", errors.New("display name cannot be empty")
	}

	// Convert to lowercase and replace spaces with hyphens
	handle := ""
	for _, ch := range displayName {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			handle += string(ch)
		} else if ch >= 'A' && ch <= 'Z' {
			handle += string(ch + 32) // Convert to lowercase
		} else if ch == ' ' || ch == '-' || ch == '_' {
			handle += "-"
		}
	}

	// Remove leading/trailing hyphens and ensure not empty
	for len(handle) > 0 && handle[0] == '-' {
		handle = handle[1:]
	}
	for len(handle) > 0 && handle[len(handle)-1] == '-' {
		handle = handle[:len(handle)-1]
	}

	if handle == "" {
		// Fallback to random string if sanitization removed everything
		randomBytes := make([]byte, 8)
		if _, err := rand.Read(randomBytes); err != nil {
			return "", fmt.Errorf("failed to generate random handle: %w", err)
		}
		handle = "key-" + hex.EncodeToString(randomBytes)
	}

	return handle, nil
}

// ValidateTemplateHandle validates the format and length of a template handle
func ValidateTemplateHandle(handle string) error {
	if len(handle) == 0 {
		return errors.New("template handle cannot be empty")
	}
	if len(handle) > 255 {
		return errors.New("template handle length must not exceed 255 characters")
	}
	if !templateHandleRegex.MatchString(handle) {
		return errors.New("template handle contains invalid characters (only alphanumeric, hyphens, and underscores allowed)")
	}
	return nil
}

// ValidateConfigName validates the format and length of a configuration name
func ValidateConfigName(name string) error {
	if len(name) == 0 {
		return errors.New("configuration name cannot be empty")
	}
	if len(name) > 255 {
		return errors.New("configuration name must not exceed 255 characters")
	}
	if !configNameRegex.MatchString(name) {
		return errors.New("configuration name contains invalid characters (only alphanumeric, spaces, hyphens, and underscores allowed)")
	}
	return nil
}

// ValidateEnvironmentVariableName validates that a generated environment variable name
// doesn't conflict with common system variables
func ValidateEnvironmentVariableName(varName string) error {
	if len(varName) == 0 {
		return errors.New("environment variable name cannot be empty")
	}
	if len(varName) > 255 {
		return errors.New("environment variable name too long")
	}

	// Check format: must start with letter or underscore, followed by alphanumeric or underscores
	if !envVarNameRegex.MatchString(varName) {
		return errors.New("invalid environment variable name format")
	}

	return nil
}

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

const (
	// APIKeyLength is the length of the generated API key in bytes
	APIKeyLength = 32
)

// templateHandleRegex allows alphanumeric characters, hyphens, and underscores
var templateHandleRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// envVarNameRegex validates environment variable name format (must start with letter or underscore, followed by alphanumeric or underscores)
var envVarNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// configNameRegex allows alphanumeric characters, spaces, hyphens, and underscores
var configNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 _-]+$`)

// Path parameters the gateway's routes share with the core's.
const (
	PathParamOrgName   = "orgName"
	PathParamProjName  = "projName"
	PathParamAgentName = "agentName"
	PathParamGroupID   = "groupID"
	PathParamRoleID    = "roleID"
)

// Pagination bounds of the gateway's list routes.
const (
	DefaultLimit  = 50
	MinLimit      = 1
	MaxLimit      = 100
	DefaultOffset = 0
	MinOffset     = 0
)

// AgentProvisioningType says whether the platform runs an agent or only
// registers it.
type AgentProvisioningType string

const (
	InternalAgent AgentProvisioningType = "internal"
	ExternalAgent AgentProvisioningType = "external"
)

// StrPointerAsStr returns *v, or defaultValue when v is nil.
func StrPointerAsStr(v *string, defaultValue string) string {
	if v == nil {
		return defaultValue
	}
	return *v
}

// StrAsStrPointer returns a pointer to v.
func StrAsStrPointer(v string) *string {
	return &v
}
