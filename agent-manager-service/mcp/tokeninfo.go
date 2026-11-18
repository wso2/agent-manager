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

package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// claimsTokenVerifier adapts the SDK's auth.TokenVerifier interface to the
// claims our own JWT middleware already validated and stored on the request
// context. It deliberately ignores the raw bearer token string: by the time
// this verifier runs (see RegisterRoute, which nests auth.RequireBearerToken
// inside our auth middleware) the token has already been verified against
// JWKS and its claims placed on the context. This adapter exists purely to
// hand that already-validated identity to the SDK so it can (a) activate its
// session-hijack guard, which compares TokenInfo.UserID across requests on
// the same session, and (b) expose per-request scopes to tool handlers via
// CallToolRequest.Extra.TokenInfo.
func claimsTokenVerifier(ctx context.Context, _ string, r *http.Request) (*auth.TokenInfo, error) {
	claims := jwtassertion.GetTokenClaims(ctx)
	if claims == nil {
		return nil, fmt.Errorf("%w: no validated token claims on request context", auth.ErrInvalidToken)
	}
	if claims.ExpiresAt == nil {
		// The SDK's RequireBearerToken middleware treats a zero Expiration as
		// an immediate 401 anyway; return the error explicitly here so the
		// reason is clear rather than relying on that implicit behavior.
		return nil, fmt.Errorf("%w: token claims have no expiration", auth.ErrInvalidToken)
	}
	if claims.Sub == "" {
		// The streamable transport's session-hijack guard only activates when
		// the session's userID is non-empty (it skips the check entirely for
		// ""), so an empty sub would silently deactivate the guard we just
		// wired up. Reject it instead of letting UserID default to "".
		return nil, fmt.Errorf("%w: token missing sub claim", auth.ErrInvalidToken)
	}
	extra := map[string]any{tools.TokenInfoOUIDKey: claims.OuId}
	if r != nil {
		if report := growthanalytics.NewMCPReporter(r); report != nil {
			extra[growthanalytics.MCPReporterKey] = report
		}
	}
	return &auth.TokenInfo{
		Scopes:     strings.Fields(claims.Scope),
		UserID:     claims.Sub,
		Expiration: claims.ExpiresAt.Time,
		// Record the caller's org so authzMiddleware can confirm each per-request
		// token targets the same org as the session (see mcp/tools/authz.go).
		Extra: extra,
	}, nil
}
