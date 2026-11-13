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

package middleware

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// environmentPathParams and environmentQueryParams name every way a route
// accepts an environment.
var (
	environmentPathParams  = []string{"envID", "envName", "environmentId", "environmentName"}
	environmentQueryParams = []string{"environment"}
)

// WithDefaultEnvironment rewrites any environment a request names to the
// organization's only environment. Routes keep their environment segments for
// API compatibility; the value is never used to pick between environments.
func WithDefaultEnvironment(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, name := range environmentPathParams {
			if r.PathValue(name) != "" {
				r.SetPathValue(name, models.DefaultEnvironmentName)
			}
		}
		query := r.URL.Query()
		rewritten := false
		for _, name := range environmentQueryParams {
			if query.Has(name) {
				query.Set(name, models.DefaultEnvironmentName)
				rewritten = true
			}
		}
		if rewritten {
			r.URL.RawQuery = query.Encode()
		}
		next(w, r)
	}
}
