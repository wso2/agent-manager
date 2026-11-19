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

// userIDRegex allows alphanumeric characters, hyphens, underscores, @ and dots
// This covers common user ID formats like emails, UUIDs, and usernames
var userIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_@.-]+$`)

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
