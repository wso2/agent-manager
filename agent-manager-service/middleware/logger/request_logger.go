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

package logger

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
)

type loggerKey struct{}

// WithLogger adds a logger to the context
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// GetLogger retrieves the logger from context, or returns the configured default logger
func GetLogger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	// Use the globally configured logger instead of slog.Default()
	return slog.Default()
}

func RequestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlationID := "unknown"
			if id := r.Context().Value(utils.CorrelationIdCtxKey()); id != nil {
				if idStr, ok := id.(string); ok {
					correlationID = idStr
				}
			}
			// Use the globally configured logger
			reqLogger := slog.Default().With(
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("correlation_id", correlationID),
			)
			ctx := WithLogger(r.Context(), reqLogger)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
