// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strings"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
)

func ValidateAgentCreatePayload(payload spec.CreateAgentRequest) error {
	// Validate agent name
	if err := ValidateResourceName(payload.Name, "agent"); err != nil {
		return fmt.Errorf("invalid agent name: %w", err)
	}
	if err := ValidateResourceDisplayName(payload.DisplayName, "agent"); err != nil {
		return fmt.Errorf("invalid agent display name: %w", err)
	}
	// Validate agent provisioning
	if err := validateAgentProvisioning(payload.Provisioning); err != nil {
		return fmt.Errorf("invalid agent provisioning: %w", err)
	}
	// Validate agent type and subtype
	if err := validateAgentType(payload.AgentType); err != nil {
		return fmt.Errorf("invalid agent type or subtype: %w", err)
	}
	// Additional validations for internal agents
	if payload.Provisioning.Type == string(InternalAgent) {
		if err := validateInternalAgent(payload); err != nil {
			return err
		}
	}

	return nil
}

// validateInternalAgent performs validations specific to internal agents
func validateInternalAgent(payload spec.CreateAgentRequest) error {
	// Validate Agent Type
	if err := validateAgentSubType(payload.AgentType); err != nil {
		return fmt.Errorf("invalid agent subtype: %w", err)
	}
	// Validate API input interface for API agents
	if payload.AgentType.Type == string(AgentTypeAPI) {
		if err := validateInputInterface(payload.AgentType, payload.InputInterface); err != nil {
			return fmt.Errorf("invalid inputInterface: %w", err)
		}
	}

	// Validate runtime configurations
	if payload.RuntimeConfigs == nil {
		return fmt.Errorf("runtimeConfigs is required for internal agents")
	}

	if err := validateLanguage(payload.RuntimeConfigs.Language, payload.RuntimeConfigs.LanguageVersion); err != nil {
		return fmt.Errorf("invalid language: %w", err)
	}

	return nil
}

func validateAgentType(agentType spec.AgentType) error {
	if agentType.Type != string(AgentTypeAPI) {
		return fmt.Errorf("unsupported agent type: %s", agentType.Type)
	}
	return nil
}

func validateAgentSubType(agentType spec.AgentType) error {
	if agentType.SubType == nil {
		return fmt.Errorf("agent subtype is required")
	}
	if agentType.Type != string(AgentTypeAPI) {
		return fmt.Errorf("unsupported agent type: %s", agentType.Type)
	}
	// Validate subtype for API agent type
	subType := StrPointerAsStr(agentType.SubType, "")
	if subType != string(AgentSubTypeChatAPI) && subType != string(AgentSubTypeCustomAPI) {
		return fmt.Errorf("unsupported agent subtype for type %s: %s", agentType.Type, subType)
	}

	return nil
}

func validateAgentProvisioning(provisioning spec.Provisioning) error {
	if provisioning.Type != string(InternalAgent) && provisioning.Type != string(ExternalAgent) {
		return fmt.Errorf("provisioning type must be either 'internal' or 'external'")
	}
	if provisioning.Type == string(InternalAgent) {
		// Validate repository details for internal agents
		if err := validateRepoDetails(provisioning.Repository); err != nil {
			return fmt.Errorf("invalid repository details: %w", err)
		}
	}
	return nil
}

func ValidateResourceDisplayName(displayName string, resourceType string) error {
	if displayName == "" {
		return fmt.Errorf("%s name cannot be empty", resourceType)
	}
	return nil
}

// validates that a resource name follows RFC 1035 DNS label standards
func ValidateResourceName(name string, resourceType string) error {
	if name == "" {
		return fmt.Errorf("%s name cannot be empty", resourceType)
	}

	// Check length
	if len(name) > MaxResourceNameLength {
		return fmt.Errorf("%s name must be at most %d characters, got %d", resourceType, MaxResourceNameLength, len(name))
	}

	// Check if name contains only lowercase alphanumeric characters or '-'
	validChars := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !validChars.MatchString(name) {
		return fmt.Errorf("%s name must contain only lowercase alphanumeric characters or '-'", resourceType)
	}

	// Check if name starts with an alphabetic character
	if !regexp.MustCompile(`^[a-z]`).MatchString(name) {
		return fmt.Errorf("%s name must start with an alphabetic character", resourceType)
	}

	// Check if name ends with an alphanumeric character
	if !regexp.MustCompile(`[a-z0-9]$`).MatchString(name) {
		return fmt.Errorf("%s name must end with an alphanumeric character", resourceType)
	}
	return nil
}

func validateRepoDetails(repo *spec.RepositoryConfig) error {
	if repo == nil {
		return fmt.Errorf("repository details are required for internal agents")
	}
	if repo.Url == "" {
		return fmt.Errorf("repository URL cannot be empty")
	}
	if !strings.HasPrefix(repo.Url, "https://github.com/") {
		return fmt.Errorf("only GitHub URLs are supported (format: https://github.com/owner/repo)")
	}
	// Validate repository path format (owner/repo)
	parts := strings.TrimPrefix(repo.Url, "https://github.com/")
	if !strings.Contains(parts, "/") || strings.Count(parts, "/") > 1 {
		return fmt.Errorf("invalid GitHub repository format (expected: https://github.com/owner/repo)")
	}
	if repo.Branch == "" {
		return fmt.Errorf("repository branch cannot be empty")
	}
	return nil
}

// ValidateInputInterface validates the inputInterface field in CreateAgentRequest
func validateInputInterface(agentType spec.AgentType, inputInterface *spec.InputInterface) error {
	if inputInterface == nil {
		return fmt.Errorf("inputInterface is required for internal agents")
	}
	if inputInterface.Type != string(InputInterfaceTypeHTTP) {
		return fmt.Errorf("unsupported inputInterface type: %s", inputInterface.Type)
	}
	if StrPointerAsStr(agentType.SubType, "") == string(AgentSubTypeCustomAPI) {
		if inputInterface.Schema.Path == "" {
			return fmt.Errorf("inputInterface.schema.path is required")
		}
		if inputInterface.Port <= 0 || inputInterface.Port > 65535 {
			return fmt.Errorf("inputInterface.port must be a valid port number (1-65535)")
		}
		if inputInterface.BasePath == "" {
			return fmt.Errorf("inputInterface.basePath is required")
		}
	}

	return nil
}

func validateLanguage(language string, languageVersion *string) error {
	if language == "" {
		return fmt.Errorf("language cannot be empty")
	}
	if languageVersion == nil && language != string(LanguageBallerina) {
		return fmt.Errorf("language version cannot be empty")
	}

	// Find the buildpack for the given language
	for _, buildpack := range Buildpacks {
		if buildpack.Language != language {
			continue
		}

		if language == string(LanguageBallerina) {
			// Ballerina does not require version validation
			return nil
		}

		// Language found, now check if version is supported
		supportedVersions := strings.Split(buildpack.SupportedVersions, ",")
		for _, version := range supportedVersions {
			version = strings.TrimSpace(version)
			if isVersionMatching(version, *languageVersion) {
				return nil
			}
		}

		// Language found but version not supported
		return fmt.Errorf("unsupported language version '%s' for language '%s'", *languageVersion, language)
	}

	// Language not found
	return fmt.Errorf("unsupported language '%s'", language)
}

// isVersionMatching checks if a provided version matches against a supported version pattern
// Supports matching partial versions against patterns with 'x' wildcards
// Examples: "3.11" matches "3.11.x", "12.5" matches "12.x.x"
func isVersionMatching(supportedVersion, providedVersion string) bool {
	// Exact match
	if supportedVersion == providedVersion {
		return true
	}

	// If no wildcards, only exact match is valid
	if !strings.Contains(supportedVersion, "x") {
		return false
	}

	// Check if provided version is a valid prefix of the pattern
	// Replace 'x' with any digit pattern and check if provided version matches the prefix
	supportedParts := strings.Split(supportedVersion, ".")
	providedParts := strings.Split(providedVersion, ".")

	// Provided version can't be longer than supported pattern
	if len(providedParts) > len(supportedParts) {
		return false
	}

	// Check each part matches or is wildcarded
	for i, providedPart := range providedParts {
		supportedPart := supportedParts[i]
		if supportedPart != "x" && supportedPart != providedPart {
			return false
		}
	}

	return true
}

func ValidateResourceNameRequest(payload spec.ResourceNameRequest) error {
	if err := ValidateResourceDisplayName(payload.DisplayName, "resource"); err != nil {
		return fmt.Errorf("invalid resource display name: %w", err)
	}
	if payload.ResourceType != string(ResourceTypeAgent) && payload.ResourceType != string(ResourceTypeProject) {
		return fmt.Errorf("invalid resource type")
	}
	if payload.ResourceType == string(ResourceTypeAgent) {
		if payload.ProjectName != nil && *payload.ProjectName == "" {
			return fmt.Errorf("projectName cannot be empty for agent resource type")
		}
	}
	return nil
}

// WriteSuccessResponse writes a successful API response
func WriteSuccessResponse[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if statusCode == http.StatusNoContent {
		return
	}
	_ = json.NewEncoder(w).Encode(data) // Ignore encoding errors for response
}

// WriteErrorResponse writes an error API response
func WriteErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	errPayload := &spec.ErrorResponse{
		Message: message,
	}
	_ = json.NewEncoder(w).Encode(errPayload) // Ignore encoding errors for response
}

// generateRandomSuffix creates a random suffix of specified length using custom alphabet
func generateRandomSuffix(length int) string {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = NameGenerationAlphabet[rand.Intn(len(NameGenerationAlphabet))]
	}

	return string(result)
}

// GenerateCandidateName transforms display name following the specified rules
func GenerateCandidateName(displayName string) string {
	// Trim whitespace
	candidate := strings.TrimSpace(displayName)

	// Convert to lowercase
	candidate = strings.ToLower(candidate)

	// Remove all non-alphanumeric characters except spaces and hyphens
	re := regexp.MustCompile(`[^a-zA-Z0-9\s-]`)
	candidate = re.ReplaceAllString(candidate, "")

	// Replace multiple spaces with single hyphen
	re = regexp.MustCompile(`\s+`)
	candidate = re.ReplaceAllString(candidate, "-")

	// Limit to max resource name length
	if len(candidate) > MaxResourceNameLength {
		candidate = candidate[:MaxResourceNameLength]
	}

	// Remove leading and trailing hyphens
	re = regexp.MustCompile(`^-+|-+$`)
	candidate = re.ReplaceAllString(candidate, "")

	return candidate
}

// NameChecker is a function type that checks if a name is available
// Returns true if name is available, false if taken, error if check failed
type NameChecker func(name string) (bool, error)

// GenerateUniqueNameWithSuffix creates a unique name by appending a random suffix
func GenerateUniqueNameWithSuffix(baseName string, checker NameChecker) (string, error) {
	// Prepare base name for unique suffix
	var baseForUnique string
	if len(baseName) <= ValidCandidateLength {
		baseForUnique = baseName
	} else {
		baseForUnique = baseName[:ValidCandidateLength]
	}

	for attempts := 0; attempts < MaxNameGenerationAttempts; attempts++ {
		// Generate random suffix
		suffix := generateRandomSuffix(RandomSuffixLength)
		uniqueName := fmt.Sprintf("%s-%s", baseForUnique, suffix)

		// Check if this name is available
		available, err := checker(uniqueName)
		if err != nil {
			return "", err
		}
		if available {
			return uniqueName, nil
		}
		// Name is taken, try again with different suffix
	}

	return "", fmt.Errorf("failed to generate unique name after %d attempts", MaxNameGenerationAttempts)
}
