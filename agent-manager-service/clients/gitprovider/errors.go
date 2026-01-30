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

package gitprovider

import "errors"

var (
	// ErrUnauthorized is returned when authentication fails
	ErrUnauthorized = errors.New("unauthorized: invalid or missing token")

	// ErrNotFound is returned when a repository is not found
	ErrNotFound = errors.New("repository not found")

	// ErrRateLimited is returned when rate limit is exceeded
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrForbidden is returned when access is forbidden
	ErrForbidden = errors.New("access forbidden")
)

// IsNotFoundError checks if the error is a not found error
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	ghErr := &GitHubError{}
	if errors.As(err, &ghErr) {
		return ghErr.StatusCode == 404
	}
	return false
}

// IsUnauthorizedError checks if the error is an unauthorized error
func IsUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnauthorized) {
		return true
	}
	ghErr := &GitHubError{}
	if errors.As(err, &ghErr) {
		return ghErr.StatusCode == 401
	}
	return false
}

// IsRateLimitedError checks if the error is a rate limit error
func IsRateLimitedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	ghErr := &GitHubError{}
	if errors.As(err, &ghErr) {
		return ghErr.StatusCode == 403 || ghErr.StatusCode == 429
	}
	return false
}
