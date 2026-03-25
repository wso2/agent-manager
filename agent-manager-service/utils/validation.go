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
	"errors"
	"regexp"
)

// templateHandleRegex allows alphanumeric characters, hyphens, and underscores
var templateHandleRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// userIDRegex allows alphanumeric characters, hyphens, underscores, @ and dots
// This covers common user ID formats like emails, UUIDs, and usernames
var userIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_@.-]+$`)

// envVarNameRegex validates environment variable name format (must start with letter or underscore, followed by alphanumeric or underscores)
var envVarNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

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

// ValidateUserID validates the format and length of a user ID
func ValidateUserID(userID string) error {
	if len(userID) == 0 {
		return errors.New("user ID cannot be empty")
	}
	if len(userID) > 255 {
		return errors.New("user ID length must not exceed 255 characters")
	}
	// Check for control characters and newlines (header injection prevention)
	for _, ch := range userID {
		if ch < 32 || ch == 127 {
			return errors.New("user ID contains invalid control characters")
		}
	}
	if !userIDRegex.MatchString(userID) {
		return errors.New("user ID contains invalid characters")
	}
	return nil
}

// configNameRegex allows alphanumeric characters, spaces, hyphens, and underscores
var configNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 _-]+$`)

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
