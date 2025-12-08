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

package middleware

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// WithPathParamValidation wraps a handler and validates required path parameters
// This runs after route matching, so r.PathValue() works correctly
func WithPathParamValidation(handler http.HandlerFunc, requiredParams ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate each required parameter
		for _, paramName := range requiredParams {
			value := r.PathValue(paramName)
			if strings.TrimSpace(value) == "" {
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required path parameter: "+paramName)
				return
			}
		}

		// All validations passed, call the original handler
		handler(w, r)
	}
}

// HandleFuncWithValidation is a helper that registers a route with automatic path parameter validation
// It extracts parameter names from the pattern and applies validation automatically
func HandleFuncWithValidation(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	// Extract parameter names from pattern like "GET /orgs/{orgName}/projects/{projName}"
	params := extractPathParams(pattern)

	if len(params) > 0 {
		// Wrap handler with validation for extracted parameters
		handler = WithPathParamValidation(handler, params...)
	}

	mux.HandleFunc(pattern, handler)
}

// extractPathParams extracts parameter names from a route pattern
// Example: "GET /orgs/{orgName}/projects/{projName}" -> ["orgName", "projName"]
func extractPathParams(pattern string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(pattern, -1)

	params := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			paramName := strings.TrimSpace(match[1])
			params = append(params, paramName)
		}
	}

	return params
}
