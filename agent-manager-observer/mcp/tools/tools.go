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

// Package tools implements the am-obs-mcp trace tools. Handlers call
// controllers.TracingController
// directly — there is no HTTP hop and no claims parsing: every tool takes
// an explicit, required "organization" input (except get_span_details,
// whose underlying controller call is already scoped by trace/span ID and
// never uses organization).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// Toolsets holds the controllers backing the am-obs-mcp tools.
type Toolsets struct {
	Tracing *controllers.TracingController
	// authorize, when set, builds each tool's per-call guard in place of the
	// enforcing one.
	authorize func(rbac.Permission) func(*gomcp.CallToolRequest) error
}

// guard is the per-call authorization check a tool registers with. Leaving
// Toolsets.authorize nil yields the real, enforcing guard.
func (t *Toolsets) guard(perm rbac.Permission) func(*gomcp.CallToolRequest) error {
	if t.authorize != nil {
		return t.authorize(perm)
	}
	return requireToolPermission(perm)
}

// Register wires every tool onto server. Toolsets left nil are skipped, so
// partial wiring is safe.
func (t *Toolsets) Register(server *gomcp.Server) {
	if t == nil {
		return
	}
	if t.Tracing != nil {
		t.registerTraceTools(server)
	}
}

// tracesMaxWindow is the max start..end span accepted by the trace
// tools, ported from am-mcp's resolveTraceTimeWindow.
const tracesMaxWindow = 30 * 24 * time.Hour

// resolveTraceTimeWindow resolves a start/end time window for the trace
// tools (max 30 days), defaulting to the last 24h when neither is provided.
func resolveTraceTimeWindow(start, end string) (time.Time, time.Time, error) {
	return resolveWindowWithLimit(start, end, tracesMaxWindow)
}

func resolveWindowWithLimit(start, end string, maxDuration time.Duration) (time.Time, time.Time, error) {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)

	if start == "" && end == "" {
		endTime := time.Now().UTC()
		return endTime.Add(-24 * time.Hour), endTime, nil
	}
	if start == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("start_time is required when end_time is provided")
	}
	startTime, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_time format; use RFC3339")
	}
	endTime := time.Now().UTC()
	if end != "" {
		endTime, err = time.Parse(time.RFC3339, end)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end_time format; use RFC3339")
		}
	}
	if !startTime.Before(endTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("start_time must be before end_time")
	}
	if endTime.Sub(startTime) > maxDuration {
		days := int(maxDuration.Hours() / 24)
		return time.Time{}, time.Time{}, fmt.Errorf("time range cannot exceed %d days", days)
	}
	return startTime.UTC(), endTime.UTC(), nil
}

// requireField trims value and rejects it (and whitespace-only input) as
// missing. Most required fields are already enforced by the tool's JSON
// schema (no "omitempty" tag), but the schema only rejects a fully absent
// property — this catches "" and " " too.
func requireField(value, name string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return trimmed, nil
}

// optionalScope trims value and returns a pointer to it, or nil when it is
// empty/whitespace — mirroring optionalStr in handlers/handlers.go for optional
// scoping filters (project/agent) that are omitted rather than
// required.
func optionalScope(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// validSortOrders are the only sort_order values accepted by the trace and
// log tools, mirroring parseSortOrder in handlers/handlers.go.
var validSortOrders = map[string]bool{"asc": true, "desc": true}

// validateSortOrder mirrors parseSortOrder: empty input returns defaultVal,
// anything other than "asc"/"desc" is rejected.
func validateSortOrder(order, defaultVal string) (string, error) {
	order = strings.TrimSpace(order)
	if order == "" {
		return defaultVal, nil
	}
	if !validSortOrders[order] {
		return "", fmt.Errorf("sort_order must be 'asc' or 'desc'")
	}
	return order, nil
}

// validateLimit resolves an optional limit input against [1, maxVal],
// defaulting to defaultVal when nil, mirroring parseLimit in
// handlers/handlers.go (used by the trace routes: GetTraceOverviews,
// ExportTraces, GetTraceSpans): a non-positive limit is rejected, but a
// limit over maxVal is silently clamped down to maxVal rather than
// rejected.
func validateLimit(limit *int, defaultVal, maxVal int) (int, error) {
	if limit == nil {
		return defaultVal, nil
	}
	if *limit < 1 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	if *limit > maxVal {
		return maxVal, nil
	}
	return *limit, nil
}

// requireToolPermission builds the per-call authorization guard for a tool,
// applying the same policy as the REST routes (middleware.AuthorizePermission):
// ordinary tokens need the tool's amp scope, and publisher-audience
// (amp-publisher-*) tokens are confined to their implicit trace-read
// permission. The per-call bearer token is
// read from req.Extra.Header (set by the streamable-HTTP transport for every
// tool call); JWTAuth has already verified it. The handler ctx cannot be used
// instead: the transport derives it from the session-initializing request, so
// middleware.GetTokenClaims(ctx) would reflect the initialize token, not the
// current call's.
func requireToolPermission(perm rbac.Permission) func(*gomcp.CallToolRequest) error {
	return func(req *gomcp.CallToolRequest) error {
		var authHeader string
		if req != nil && req.Extra != nil && req.Extra.Header != nil {
			authHeader = req.Extra.Header.Get("Authorization")
		}
		claims := middleware.ParseUnverifiedClaims(authHeader)
		if err := middleware.AuthorizePermission(claims, perm); err != nil {
			return fmt.Errorf("%w: this tool requires the %s scope", err, perm.Scope())
		}
		return nil
	}
}

// wrapToolError adds the failing tool's name to err for easier debugging by
// an LLM caller juggling multiple tool calls.
func wrapToolError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", toolName, err)
}

// withToolLogging logs the outcome and duration of every tool call.
func withToolLogging[T any](toolName string, handler func(context.Context, *gomcp.CallToolRequest, T) (*gomcp.CallToolResult, any, error)) func(context.Context, *gomcp.CallToolRequest, T) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input T) (*gomcp.CallToolResult, any, error) {
		log := logger.GetLogger(ctx)
		start := time.Now()
		result, meta, err := handler(ctx, req, input)
		duration := time.Since(start).Milliseconds()
		if err != nil {
			log.Error("mcp tool failed", "tool", toolName, "duration_ms", duration, "error", err)
		} else {
			log.Info("mcp tool succeeded", "tool", toolName, "duration_ms", duration)
		}
		return result, meta, err
	}
}

// handleToolResult marshals a successful result to the tool's text content.
func handleToolResult(result any, err error) (*gomcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{
			&gomcp.TextContent{Text: string(jsonData)},
		},
	}, result, nil
}
