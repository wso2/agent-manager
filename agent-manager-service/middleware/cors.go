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
	"strings"
)

// CORS enables Cross-Origin Resource Sharing for the provided origins.
// allowedOrigin is comma-separated list of allowed origins.
// It sets the necessary headers and short-circuits OPTIONS preflight requests.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Always set Vary headers for proper caching behavior
			w.Header().Add("Vary", "Origin")
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")

			// Check if origin is allowed
			var matchedOrigin string
			if origin != "" {
				// Parse comma-separated list of allowed origins
				allowedOrigins := strings.Split(allowedOrigin, ",")
				for _, allowed := range allowedOrigins {
					allowed = strings.TrimSpace(allowed)
					if allowed == "*" {
						matchedOrigin = "*"
						break
					} else if origin == allowed {
						matchedOrigin = origin
						break
					}
				}
			}

			if matchedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", matchedOrigin)
				// Allow credentials if using cookies or Authorization header
				if matchedOrigin != "*" {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With, Accept, Origin, x-correlation-id")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight request
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
