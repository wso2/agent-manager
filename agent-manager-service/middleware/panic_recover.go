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
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
)

func RecovererOnPanic() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					correlationId := "unknown"
					if id := r.Context().Value(utils.CorrelationIdCtxKey()); id != nil {
						if idStr, ok := id.(string); ok {
							correlationId = idStr
						}
					}

					operation := "unknown"
					if op := r.Context().Value("operation"); op != nil {
						if opStr, ok := op.(string); ok {
							operation = opStr
						}
					}

					slog.Error("recoverOnPanic",
						"correlationID", correlationId,
						"operation", operation,
						"log_type", "err_response",
						"panic", rec,
						"stack", string(debug.Stack()))

					utils.WriteErrorResponse(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
