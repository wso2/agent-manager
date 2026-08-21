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
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

// requestInfo carries the identity a request acquires *after* the access-log
// wrapper has already been entered. Org resolution and authorization run inside
// it, and a context value they add is invisible out here — so they write into
// this shared holder instead, and the completion record reads it back.
//
// This mirrors audit.NewRequestScope (see audit_route.go), which solves the
// same ordering problem for the audit trail.
type requestInfo struct {
	ouID string
}

type requestInfoKey struct{}

// newRequestInfo attaches a fresh holder to ctx and returns it for the caller
// to read once the handler has returned.
func newRequestInfo(ctx context.Context) (context.Context, *requestInfo) {
	info := &requestInfo{}
	return context.WithValue(ctx, requestInfoKey{}, info), info
}

// RequestInfoFrom returns the holder for this request, or nil outside one.
func requestInfoFrom(ctx context.Context) *requestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(*requestInfo)
	return info
}

// setRequestIdentity records the org the request resolved to. Safe to call when
// no holder is present (unit tests, non-registrar surfaces).
//
// The token subject is not recorded here — see RequireOrgMatch.
func setRequestIdentity(ctx context.Context, ouID string) {
	if info := requestInfoFrom(ctx); info != nil {
		info.ouID = ouID
	}
}

// WithRequestLog emits exactly one record per request describing its outcome,
// and enriches the context logger with the audit action so every line the
// handler writes underneath is filterable by operation.
//
// It is installed for *every* route, audited or not. The audit trail covers
// only audited routes and answers a different question ("who changed what");
// without this, a 5xx on an unaudited route whose handler forgot to log leaves
// no trace at all.
func WithRequestLog(meta audit.RouteMeta) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newResponseRecorder(w)

			// The endpoint is not repeated on the record: `path` already carries
			// the concrete URL, and the pattern only mattered for aggregating by
			// endpoint, which this deployment does not do.
			ctx := r.Context()
			// Only audited routes have an action; the rest would carry "".
			if meta.Action != "" {
				ctx = logger.Enrich(ctx, slog.String("action", string(meta.Action)))
			}
			ctx, info := newRequestInfo(ctx)

			defer func() {
				// RecovererOnPanic is the outermost middleware, so a panic
				// unwinds through here first. Record the 500 it is about to
				// write, then re-panic so its behaviour is unchanged —
				// otherwise the request would be logged as a success.
				panicked := recover()
				if panicked != nil {
					rec.setStatus(http.StatusInternalServerError)
				}

				emitRequestLog(ctx, rec, info, start)

				if panicked != nil {
					panic(panicked)
				}
			}()

			next(rec, r.WithContext(ctx))
		}
	}
}

// emitRequestLog writes the completion record. Method, path, client IP and
// correlation ID are already on the context logger (logger.RequestLogger), so
// only the outcome is added here.
func emitRequestLog(ctx context.Context, rec *responseRecorder, info *requestInfo, start time.Time) {
	status := rec.Status()

	attrs := []any{
		slog.String("log_type", "request"),
		slog.Int("status", status),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.Int64("bytes", rec.bytes),
	}
	if info.ouID != "" {
		attrs = append(attrs, slog.String("ou_id", info.ouID))
	}

	log := logger.GetLogger(ctx)
	switch {
	case status >= http.StatusInternalServerError:
		// The service failed. Everything below 500 is the caller being told no.
		log.Error("request completed", attrs...)
	case status >= http.StatusBadRequest:
		log.Warn("request completed", attrs...)
	default:
		log.Info("request completed", attrs...)
	}
}
