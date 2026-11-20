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
	"fmt"
	"math/rand"
	"regexp"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// capitalize returns a string with the first letter capitalized
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

type agentPayload struct {
	name         string
	displayName  string
	provisioning spec.Provisioning
	agentType    *spec.AgentType
}

func ValidateAgentBasicInfoUpdatePayload(payload spec.UpdateAgentBasicInfoRequest) error {
	if err := ValidateResourceDisplayName(payload.DisplayName, "agent"); err != nil {
		return err
	}
	if payload.Labels != nil {
		if err := ValidateLabels(*payload.Labels); err != nil {
			return err
		}
	}
	return nil
}

func ValidateProjectUpdatePayload(payload spec.UpdateProjectRequest) error {
	return ValidateResourceDisplayName(payload.DisplayName, "project")
}

func ValidateAgentCreatePayload(payload spec.CreateAgentRequest) error {
	if payload.Labels != nil {
		if err := ValidateLabels(*payload.Labels); err != nil {
			return err
		}
	}
	// The platform does not run external agents, so health checks sent for one
	// would be dropped without effect.
	if payload.HealthChecks != nil {
		return httpx.NewValidationError(
			"Health checks can only be set for agents built from source",
			"healthChecks is only allowed for internal agents built from a repository",
		)
	}
	return validateAgentPayload(agentPayload{
		name:         payload.Name,
		displayName:  payload.DisplayName,
		provisioning: payload.Provisioning,
		agentType:    payload.AgentType,
	})
}

func validateAgentPayload(payload agentPayload) error {
	// Validate agent name
	if err := ValidateResourceName(payload.name, "agent"); err != nil {
		return err
	}
	if err := ValidateResourceDisplayName(payload.displayName, "agent"); err != nil {
		return err
	}
	if payload.provisioning.Type != string(ExternalAgent) {
		return httpx.NewValidationError(
			"Only external agents can be created",
			"provisioning type must be 'external'",
		)
	}
	if payload.agentType == nil {
		return httpx.NewValidationError(
			"Agent type is required",
			"agentType is required",
		)
	}
	return validateAgentType(*payload.agentType, payload.provisioning.Type)
}

// validateAgentType accepts only the agent type that matches the provisioning
// type: the type is not stored, and reads report the one derived from provisioning.
func validateAgentType(agentType spec.AgentType, provisioningType string) error {
	if agentType.Type != string(AgentTypeAPI) && agentType.Type != string(AgentTypeExternalAPI) {
		return fmt.Errorf("unsupported agent type: %s", agentType.Type)
	}
	if want := AgentTypeForProvisioning(provisioningType); agentType.Type != string(want) {
		return httpx.NewValidationError(
			fmt.Sprintf("Agent type must be %s for %s agents", want, provisioningType),
			fmt.Sprintf("agentType.type must be %q for provisioning type %q, got %q", want, provisioningType, agentType.Type),
		)
	}
	return nil
}

func ValidateResourceDisplayName(displayName string, resourceType string) error {
	if displayName == "" {
		return fmt.Errorf("%s display name cannot be empty", capitalize(resourceType))
	}
	return nil
}

// validates that a resource name follows RFC 1035 DNS label standards
func ValidateResourceName(name string, resourceType string) error {
	rt := capitalize(resourceType)
	if name == "" {
		return fmt.Errorf("%s name cannot be empty", rt)
	}

	// Check length
	if len(name) > MaxResourceNameLength {
		return fmt.Errorf("%s name must be at most %d characters, got %d", rt, MaxResourceNameLength, len(name))
	}

	// Check if name contains only lowercase alphanumeric characters or '-'
	validChars := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !validChars.MatchString(name) {
		return fmt.Errorf("%s name must contain only lowercase alphanumeric characters or '-'", rt)
	}

	// Check if name starts with an alphabetic character
	if !regexp.MustCompile(`^[a-z]`).MatchString(name) {
		return fmt.Errorf("%s name must start with an alphabetic character", rt)
	}

	// Check if name ends with an alphanumeric character
	if !regexp.MustCompile(`[a-z0-9]$`).MatchString(name) {
		return fmt.Errorf("%s name must end with an alphanumeric character", rt)
	}
	return nil
}

func ValidateResourceNameRequest(payload spec.ResourceNameRequest) error {
	if err := ValidateResourceDisplayName(payload.DisplayName, "resource"); err != nil {
		return fmt.Errorf("invalid resource display name: %w", err)
	}
	if payload.ResourceType != string(ResourceTypeAgent) && payload.ResourceType != string(ResourceTypeProject) && payload.ResourceType != string(ResourceTypeEnvironment) {
		return fmt.Errorf("invalid resource type")
	}
	if payload.ResourceType == string(ResourceTypeAgent) {
		if payload.ProjectName != nil && *payload.ProjectName == "" {
			return fmt.Errorf("projectName cannot be empty for agent resource type")
		}
	}
	return nil
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

// MaxEnvNameLength returns the maximum environment name length for a given org.
// The constraint comes from the gateway runtime Service name shape:
//
//	api-platform-<org>-<env>-gw-gateway-gateway-runtime  ≤ 63 chars (k8s metadata.name)
//
// Fixed portion: len("api-platform-") + len("-") + len("-gw-gateway-gateway-runtime") = 13 + 1 + 27 = 41
// So: len(env) ≤ 63 - 41 - len(org) = 22 - len(org).
// The returned value is never less than 1.
func MaxEnvNameLength(orgName string) int {
	maxLen := 22 - len(orgName)
	if maxLen < 1 {
		return 1
	}
	return maxLen
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
