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

	"github.com/wso2/agent-manager/agent-manager-service/utils"
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

// Enrich returns a context whose logger carries attrs in addition to whatever
// it already had. Middleware use it to attach a field once — at the point the
// value becomes known — instead of every downstream call site repeating it.
//
// Enrichment only reaches handlers that run *inside* the enriching middleware:
// a context value added here is invisible to anything further out in the chain.
func Enrich(ctx context.Context, attrs ...any) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	return WithLogger(ctx, GetLogger(ctx).With(attrs...))
}

// FromRequest is GetLogger(r.Context()) — the form controllers need.
func FromRequest(r *http.Request) *slog.Logger {
	return GetLogger(r.Context())
}

func RequestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlationID := utils.GetCorrelationId(r.Context())

			// Use the globally configured logger
			// No log_type here. Records that belong to a specific stream add
			// their own (request, upstream, audit, …), and slog appends rather
			// than replaces — stamping "app" on the base logger put the key in
			// the record twice, which makes a term query ambiguous. An
			// application log line is the one with no log_type.
			reqLogger := slog.Default().With(
				slog.String("method", r.Method),
				slog.String("path", utils.TruncateForLog(utils.SanitizeForLog(r.URL.Path), 512)),
				slog.String("correlation_id", correlationID),
			)
			ctx := WithLogger(r.Context(), reqLogger)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
