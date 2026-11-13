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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
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
	name          string
	displayName   string
	provisioning  spec.Provisioning
	agentType     *spec.AgentType
	configuration *spec.Configurations
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
		return NewValidationError(
			"Health checks can only be set for agents built from source",
			"healthChecks is only allowed for internal agents built from a repository",
		)
	}
	return validateAgentPayload(agentPayload{
		name:          payload.Name,
		displayName:   payload.DisplayName,
		provisioning:  payload.Provisioning,
		agentType:     payload.AgentType,
		configuration: payload.Configurations,
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
		return NewValidationError(
			"Only external agents can be created",
			"provisioning type must be 'external'",
		)
	}
	// File mounts on the create payload (under .configuration.Files) — guard size /
	// key shape / dedup early so a bad request fails before we touch the platform store.
	if payload.configuration != nil {
		if err := ValidateFileMounts(payload.configuration.Files); err != nil {
			return fmt.Errorf("invalid file mounts: %w", err)
		}
		if err := validateEnvironmentVariables(payload.configuration.Env); err != nil {
			return fmt.Errorf("invalid environment variables: %w", err)
		}
	}
	if payload.agentType == nil {
		return NewValidationError(
			"Agent type is required",
			"agentType is required",
		)
	}
	return validateAgentType(*payload.agentType)
}

func validateAgentType(agentType spec.AgentType) error {
	if agentType.Type != string(AgentTypeAPI) && agentType.Type != string(AgentTypeExternalAPI) {
		return fmt.Errorf("unsupported agent type: %s", agentType.Type)
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

// ValidateCreateGitSecretRequest validates the CreateGitSecretRequest body
func ValidateCreateGitSecretRequest(req *spec.CreateGitSecretRequest) error {
	// Validate name using existing resource name validation
	if err := ValidateResourceName(req.Name, "git secret"); err != nil {
		return err
	}

	// Validate type
	if req.Type != GitSecretTypeBasicAuth {
		return fmt.Errorf("git secret type must be '%s'", GitSecretTypeBasicAuth)
	}

	// Validate credentials - username and password are required for basic-auth
	if req.Credentials.Username == "" {
		return fmt.Errorf("username is required for basic-auth type")
	}

	if req.Credentials.Password == "" {
		return fmt.Errorf("password is required for basic-auth type")
	}

	return nil
}

// fileMountKeyRe matches Kubernetes' allowed character set for ConfigMap/Secret data keys.
var fileMountKeyRe = regexp.MustCompile(`^[-._a-zA-Z0-9]+$`)

// fileMountPathRe restricts mountPath to a conservative POSIX subset.
var fileMountPathRe = regexp.MustCompile(`^/[A-Za-z0-9._\-/]*$`)

// ValidateFileMounts validates a file-mounts payload. Returns nil for an empty
// slice — callers decide whether files are optional.
//
// Checks per entry: non-empty + well-formed key, absolute mountPath with no
// traversal segments, non-empty value (when not a secret reference), per-file
// size cap. Also enforces uniqueness on keys/paths and a total payload cap so
// the rendered ConfigMap/Secret stays within Kubernetes' 1 MiB object limit.
func ValidateFileMounts(files []spec.FileMount) error {
	if len(files) == 0 {
		return nil
	}
	limits := config.GetConfig().FileMountLimits
	seenKeys := make(map[string]bool, len(files))
	seenPaths := make(map[string]bool, len(files))
	total := 0
	for i, f := range files {
		// Key
		if f.Key == "" {
			return fmt.Errorf("file mount at index %d has an empty key", i)
		}
		if len(f.Key) > MaxFileMountKeyLength {
			return fmt.Errorf("file mount key %q is longer than %d characters", f.Key, MaxFileMountKeyLength)
		}
		if !fileMountKeyRe.MatchString(f.Key) {
			return fmt.Errorf("file mount key %q has invalid characters; allowed: letters, digits, '.', '_', '-'", f.Key)
		}
		if seenKeys[f.Key] {
			return fmt.Errorf("duplicate file mount key %q", f.Key)
		}
		seenKeys[f.Key] = true

		// Mount path
		if f.MountPath == "" {
			return fmt.Errorf("file mount %q has an empty mountPath", f.Key)
		}
		if len(f.MountPath) > MaxMountPathLength {
			return fmt.Errorf("file mount %q mountPath is longer than %d characters", f.Key, MaxMountPathLength)
		}
		if !strings.HasPrefix(f.MountPath, "/") {
			return fmt.Errorf("file mount %q mountPath %q must be an absolute path", f.Key, f.MountPath)
		}
		if strings.Contains(f.MountPath, "..") {
			return fmt.Errorf("file mount %q mountPath %q must not contain path traversal (..)", f.Key, f.MountPath)
		}
		if !fileMountPathRe.MatchString(f.MountPath) {
			return fmt.Errorf("file mount %q mountPath %q contains invalid characters", f.Key, f.MountPath)
		}
		if seenPaths[f.MountPath] {
			return fmt.Errorf("duplicate file mount mountPath %q", f.MountPath)
		}
		seenPaths[f.MountPath] = true

		// Size — only count direct values. Secret-ref mounts carry no inline
		// content here, so they don't consume the budget.
		valueLen := 0
		if f.Value != nil {
			valueLen = len(*f.Value)
		}
		if valueLen > limits.MaxFileBytes {
			return fmt.Errorf("file mount %q value is %d bytes; max %d", f.Key, valueLen, limits.MaxFileBytes)
		}
		total += valueLen
	}
	if total > limits.MaxTotalBytes {
		return fmt.Errorf("file mounts total size %d bytes exceeds limit %d", total, limits.MaxTotalBytes)
	}
	return nil
}

// validateEnvironmentVariables validates environment variables if present in the payload
// Environment variables are optional, but if provided, they must follow naming conventions
func validateEnvironmentVariables(envVars []spec.EnvironmentVariable) error {
	if len(envVars) == 0 {
		// Environment variables are optional
		return nil
	}

	// Pattern is K8s Secret key compliant (subset of allowed chars) and POSIX env var compliant.
	// Must start with letter or underscore, followed by letters, digits, or underscores.
	validKeyPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	seenKeys := make(map[string]bool)

	for i, envVar := range envVars {
		// Validate key is not empty
		if envVar.Key == "" {
			return fmt.Errorf("environment variable at index %d has an empty key", i)
		}

		// Validate key follows naming conventions
		if !validKeyPattern.MatchString(envVar.Key) {
			return fmt.Errorf("environment variable key '%s' is invalid. Must start with a letter or underscore and contain only letters, digits, or underscores", envVar.Key)
		}

		// Check for duplicate keys
		if seenKeys[envVar.Key] {
			return fmt.Errorf("duplicate environment variable key '%s'", envVar.Key)
		}
		seenKeys[envVar.Key] = true

		// Value can be any string, including empty string, so no validation needed for value
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

// WriteSuccessResponse writes a successful API response
func WriteSuccessResponse[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if statusCode == http.StatusNoContent {
		return
	}
	_ = json.NewEncoder(w).Encode(data) // Ignore encoding errors for response
}

// WriteErrorResponse writes an error API response with auto-derived error code.
// For more control, use WriteErrorResponseWithReason.
func WriteErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	code := statusCodeToErrorCode(statusCode)
	WriteErrorResponseWithReason(w, statusCode, message, "", code)
}

// WriteErrorResponseWithReason writes an error API response with full error details.
// - message: User-friendly message for display in UI
// - reason: Technical details for debugging (can be empty)
// - code: Machine-readable error code for programmatic handling
func WriteErrorResponseWithReason(w http.ResponseWriter, statusCode int, message, reason, code string) {
	if code == "" {
		// default error code based on status code if not provided
		code = statusCodeToErrorCode(statusCode)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	errPayload := spec.NewErrorResponse(message, code)
	if reason != "" {
		errPayload.SetReason(reason)
	}
	_ = json.NewEncoder(w).Encode(errPayload) // Ignore encoding errors for response
}

// WriteValidationErrorResponse writes a validation error response.
// If err is a ValidationError, uses its user-friendly Message and technical Reason.
// Otherwise, uses err.Error() for both message and reason.
func WriteValidationErrorResponse(w http.ResponseWriter, err error) {
	if ve := IsValidationError(err); ve != nil {
		WriteErrorResponseWithReason(w, http.StatusBadRequest, ve.Message, ve.Reason, ErrCodeValidation)
		return
	}
	// Fallback for non-ValidationError
	WriteErrorResponseWithReason(w, http.StatusBadRequest, err.Error(), err.Error(), ErrCodeValidation)
}

// statusCodeToErrorCode maps HTTP status codes to default error codes
func statusCodeToErrorCode(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return ErrCodeBadRequest
	case http.StatusUnauthorized:
		return ErrCodeUnauthorized
	case http.StatusForbidden:
		return ErrCodeForbidden
	case http.StatusNotFound:
		return ErrCodeNotFound
	case http.StatusConflict:
		return ErrCodeConflict
	case http.StatusInternalServerError:
		return ErrCodeInternalError
	case http.StatusServiceUnavailable:
		return ErrCodeServiceUnavailable
	case http.StatusNotImplemented:
		return ErrCodeNotImplemented
	default:
		// Classify unmapped status codes by range
		if statusCode >= 400 && statusCode < 500 {
			return ErrCodeBadRequest
		}
		return ErrCodeInternalError
	}
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

// isValidGitHubIdentifier validates that a string contains only characters allowed in GitHub usernames/repos
// GitHub allows alphanumeric characters, hyphens, underscores, and periods
// This prevents path traversal attacks and URL manipulation
func isValidGitHubIdentifier(value string) bool {
	if value == "" {
		return false
	}
	// GitHub identifiers: alphanumeric, hyphens, underscores, and periods only
	// Must not contain: /, .., or other special characters that could be used for path traversal
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !validPattern.MatchString(value) {
		return false
	}
	// Reject path traversal patterns
	if strings.Contains(value, "..") {
		return false
	}
	return true
}

// gitBranchRe is a positive allowlist for git ref names: letters, digits and the
// punctuation that real branch names use. An allowlist is used rather than a
// denylist of git's special characters because a branch name is handed to git
// inside the build container -- a denylist has to enumerate every character that
// could matter there, and the previous one permitted ";" among others.
var gitBranchRe = regexp.MustCompile(`^[A-Za-z0-9._@+/-]+$`)

// isValidGitHubBranch validates branch names. Deliberately stricter than git's
// own refname rules: it covers every branch name seen in practice while leaving
// no room for shell metacharacters or for a leading "-" that git would read as
// an option instead of a ref.
func isValidGitHubBranch(branch string) bool {
	if branch == "" || len(branch) > MaxGitBranchLength {
		return false
	}

	if !gitBranchRe.MatchString(branch) {
		return false
	}

	// A leading "-" is read by git as an option rather than a ref.
	if strings.HasPrefix(branch, "-") {
		return false
	}

	// Refname rules the character class above cannot express.
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") ||
		strings.Contains(branch, "//") {
		return false
	}
	if strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
		return false
	}
	if strings.HasSuffix(branch, ".") || strings.HasSuffix(branch, ".lock") {
		return false
	}

	return true
}

// gitCommitIDRe matches an abbreviated or full SHA-1 object name. The lower bound
// is git's own minimum abbreviation length rather than the 7 that
// "git rev-parse --short" happens to emit, so a caller passing a shorter
// abbreviation is not rejected here and left to fail against the real repository.
var gitCommitIDRe = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

// ValidateGitCommitID validates an optional commit to build. An empty value means
// "use the latest commit on the branch" and is accepted. The commit reaches git
// inside the build container, and its first eight characters also become part of
// the built image tag, so it is held to an object name and nothing else.
func ValidateGitCommitID(commitID string) error {
	if commitID == "" {
		return nil
	}
	if !gitCommitIDRe.MatchString(commitID) {
		return NewValidationError(
			"Invalid commit. Provide a commit SHA of 4 to 40 hexadecimal characters",
			"commitId must be a hexadecimal git object name of 4 to 40 characters",
		)
	}
	return nil
}

// ValidateListBranchesRequest validates the ListBranchesRequest payload
func ValidateListBranchesRequest(payload *spec.ListBranchesRequest) error {
	// Normalize and validate owner
	payload.Owner = strings.TrimSpace(payload.Owner)
	if payload.Owner == "" {
		return fmt.Errorf("owner cannot be empty")
	}

	// Validate owner contains only safe characters
	if !isValidGitHubIdentifier(payload.Owner) {
		return fmt.Errorf("owner contains invalid characters or path traversal patterns")
	}

	// Normalize and validate repository
	payload.Repository = strings.TrimSpace(payload.Repository)
	if payload.Repository == "" {
		return fmt.Errorf("repository cannot be empty")
	}

	// Validate repository contains only safe characters
	if !isValidGitHubIdentifier(payload.Repository) {
		return fmt.Errorf("repository contains invalid characters or path traversal patterns")
	}

	// The org that scopes the secretRef comes from the caller's token, not the
	// payload, so only the reference itself is validated here.
	if payload.HasSecretRef() && payload.GetSecretRef() == "" {
		return fmt.Errorf("secretRef cannot be empty")
	}
	return nil
}

// ValidateListCommitsRequest validates the ListCommitsRequest payload
func ValidateListCommitsRequest(payload *spec.ListCommitsRequest) error {
	// Normalize and validate owner
	payload.Owner = strings.TrimSpace(payload.Owner)
	if payload.Owner == "" {
		return fmt.Errorf("owner cannot be empty")
	}

	// Validate owner contains only safe characters
	if !isValidGitHubIdentifier(payload.Owner) {
		return fmt.Errorf("owner contains invalid characters or path traversal patterns")
	}

	// Normalize and validate repo
	payload.Repo = strings.TrimSpace(payload.Repo)
	if payload.Repo == "" {
		return fmt.Errorf("repo cannot be empty")
	}

	// Validate repo contains only safe characters
	if !isValidGitHubIdentifier(payload.Repo) {
		return fmt.Errorf("repo contains invalid characters or path traversal patterns")
	}

	// Normalize and validate optional branch field if provided
	if payload.Branch != nil {
		branchVal := strings.TrimSpace(*payload.Branch)
		if branchVal == "" {
			return fmt.Errorf("branch cannot be empty or whitespace")
		}
		if !isValidGitHubBranch(branchVal) {
			return fmt.Errorf("branch contains invalid characters or path traversal patterns")
		}
		*payload.Branch = branchVal // Normalize by writing back trimmed value
	}

	// Normalize and validate optional path field if provided
	if payload.Path != nil {
		pathVal := strings.TrimSpace(*payload.Path)
		if pathVal == "" {
			return fmt.Errorf("path cannot be empty or whitespace")
		}
		// Path can contain slashes, but reject path traversal patterns
		if strings.Contains(pathVal, "..") {
			return fmt.Errorf("path contains path traversal patterns")
		}
		*payload.Path = pathVal // Normalize by writing back trimmed value
	}

	// Normalize and validate optional author field if provided
	if payload.Author != nil {
		authorVal := strings.TrimSpace(*payload.Author)
		if authorVal == "" {
			return fmt.Errorf("author cannot be empty or whitespace")
		}
		// Author can be a username or email, so allow @ and . characters
		// But still reject path traversal patterns
		if strings.Contains(authorVal, "..") || strings.Contains(authorVal, "/") {
			return fmt.Errorf("author contains invalid characters or path traversal patterns")
		}
		*payload.Author = authorVal // Normalize by writing back trimmed value
	}

	// Validate time fields if both are provided
	if payload.Since != nil && payload.Until != nil {
		if payload.Until.Before(*payload.Since) {
			return fmt.Errorf("until time must be after since time")
		}
	}

	// Validate since time is not in the future
	if payload.Since != nil && payload.Since.After(time.Now()) {
		return fmt.Errorf("since time cannot be in the future")
	}

	// Validate until time is not in the future
	if payload.Until != nil && payload.Until.After(time.Now()) {
		return fmt.Errorf("until time cannot be in the future")
	}

	// The org that scopes the secretRef comes from the caller's token, not the
	// payload, so only the reference itself is validated here.
	if payload.HasSecretRef() && payload.GetSecretRef() == "" {
		return fmt.Errorf("secretRef cannot be empty")
	}

	if payload.HasProjectName() != payload.HasComponentName() {
		return fmt.Errorf("projectName and componentName must be provided together")
	}
	if payload.HasProjectName() {
		projectName := strings.TrimSpace(payload.GetProjectName())
		componentName := strings.TrimSpace(payload.GetComponentName())
		if projectName == "" || componentName == "" {
			return fmt.Errorf("projectName and componentName cannot be empty")
		}
		if !isValidGitHubIdentifier(projectName) || !isValidGitHubIdentifier(componentName) {
			return fmt.Errorf("projectName or componentName contains invalid characters or path traversal patterns")
		}
		payload.ProjectName = &projectName
		payload.ComponentName = &componentName
	}
	return nil
}

// ParseGitHubURL extracts the owner and repository name from a GitHub URL.
// Supports formats:
// - https://github.com/owner/repo
// - https://github.com/owner/repo.git
// - https://github.com/owner/my.repo.git (dots allowed in repo names)
// - git@github.com:owner/repo.git
// Returns empty strings if the URL cannot be parsed.
func ParseGitHubURL(url string) (owner, repo string) {
	if url == "" {
		return "", ""
	}

	// Handle HTTPS URLs: https://github.com/owner/repo or https://github.com/owner/repo.git
	// Allow dots in repo name, strip optional .git suffix and trailing slash
	httpsPattern := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)
	if matches := httpsPattern.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1], matches[2]
	}

	// Handle SSH URLs: git@github.com:owner/repo.git
	// Allow dots in repo name, strip optional .git suffix
	sshPattern := regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := sshPattern.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1], matches[2]
	}

	return "", ""
}

// ToShortSHA converts a full git commit SHA to short format (first 8 characters).
// If the commit is already short or invalid, returns it as-is.
// This matches the behavior of the build workflow templates that use cut -c1-8.
func ToShortSHA(commit string) string {
	if commit == "" {
		return commit
	}

	// If commit is already 8 characters or less, return as-is
	if len(commit) <= 8 {
		return commit
	}

	// Return first 8 characters
	return commit[:8]
}

// BuildSecretRefName constructs the name for SecretReference CR.
// The name format is: {componentName}-secrets
func BuildSecretRefName(componentName string) string {
	return fmt.Sprintf("%s-secrets", componentName)
}

// SanitizeString converts s to a lowercase DNS-label-safe string.
// All characters that are not ASCII lowercase letters, digits, or hyphens
// are replaced with a hyphen. Uppercase letters are lowercased first.
// Used when generating Kubernetes secret names and environment variable prefixes
// from user-supplied config/env names.
func SanitizeString(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, strings.ToLower(s))
}

// riskyEvaluatorSourcePatterns are heuristics only, not a security boundary — evaluator
// source still runs unsandboxed regardless.
var riskyEvaluatorSourcePatterns = []struct {
	label   string
	pattern *regexp.Regexp
}{
	{"import os", regexp.MustCompile(`(?m)^\s*(?:from\s+os\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*os\b)`)},
	{"import subprocess", regexp.MustCompile(`(?m)^\s*(?:from\s+subprocess\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*subprocess\b)`)},
	{"import socket", regexp.MustCompile(`(?m)^\s*(?:from\s+socket\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*socket\b)`)},
	{"import ctypes", regexp.MustCompile(`(?m)^\s*(?:from\s+ctypes\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*ctypes\b)`)},
	{"import importlib", regexp.MustCompile(`(?m)^\s*(?:from\s+importlib\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*importlib\b)`)},
	{"import urllib", regexp.MustCompile(`(?m)^\s*(?:from\s+urllib\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*urllib\b)`)},
	{"import http", regexp.MustCompile(`(?m)^\s*(?:from\s+http\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*http\b)`)},
	{"import requests", regexp.MustCompile(`(?m)^\s*(?:from\s+requests\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*requests\b)`)},
	{"import ftplib", regexp.MustCompile(`(?m)^\s*(?:from\s+ftplib\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*ftplib\b)`)},
	{"import smtplib", regexp.MustCompile(`(?m)^\s*(?:from\s+smtplib\b|import\s+(?:\w+(?:\s+as\s+\w+)?\s*,\s*)*smtplib\b)`)},
	{"__import__() call", regexp.MustCompile(`__import__\s*\(`)},
}

func validateEvaluatorSourceContent(source string) error {
	for _, p := range riskyEvaluatorSourcePatterns {
		if p.pattern.MatchString(source) {
			return fmt.Errorf(
				"source contains a disallowed pattern (%s); "+
					"custom evaluator source may not import or reference os, subprocess, socket, ctypes, importlib, "+
					"urllib, http, requests, ftplib, or smtplib, or use __import__()",
				p.label,
			)
		}
	}
	return nil
}

func ValidateCreateCustomEvaluatorPayload(payload spec.CreateCustomEvaluatorRequest) error {
	if strings.TrimSpace(payload.DisplayName) == "" {
		return fmt.Errorf("display name is required")
	}
	if payload.Type != "code" && payload.Type != "llm_judge" {
		return fmt.Errorf("type must be 'code' or 'llm_judge'")
	}
	if payload.Level != "trace" && payload.Level != "agent" && payload.Level != "llm" {
		return fmt.Errorf("level must be 'trace', 'agent', or 'llm'")
	}
	if strings.TrimSpace(payload.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if err := validateEvaluatorSourceContent(payload.Source); err != nil {
		return err
	}
	return nil
}

func ValidateUpdateCustomEvaluatorPayload(payload spec.UpdateCustomEvaluatorRequest) error {
	if payload.DisplayName != nil && strings.TrimSpace(*payload.DisplayName) == "" {
		return fmt.Errorf("display name cannot be empty")
	}
	if payload.Source != nil {
		if strings.TrimSpace(*payload.Source) == "" {
			return fmt.Errorf("source cannot be empty")
		}
		if err := validateEvaluatorSourceContent(*payload.Source); err != nil {
			return err
		}
	}
	return nil
}
