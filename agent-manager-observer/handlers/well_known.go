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

package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware/logger"
)

// protectedResourceMetadata is the RFC 9728 OAuth 2.0 Protected Resource
// Metadata document served at the root compatibility location and the RFC
// 9728 path-specific /.well-known/oauth-protected-resource/mcp location.
type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
}

// ampAPIResourceIdentifier is the shared Agent Manager API resource registered
// in Thunder. Observer REST endpoints accept that platform API audience, while
// the Observer MCP endpoint has its own deployment-specific URL resource.
const ampAPIResourceIdentifier = "urn:wso2:amp"

// RegisterWellKnownRoutes registers the RFC 9728 OAuth 2.0 protected resource
// metadata endpoint on mux. The route is unauthenticated and returns 503 when
// SERVER_PUBLIC_URL or OAUTH_AUTHORIZATION_SERVERS is not configured.
//
// Ported from agent-manager-service/api/well_known_routes.go.
func RegisterWellKnownRoutes(mux *http.ServeMux, cfg config.AuthConfig) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", protectedResourceMetadataHandler(cfg, false))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", protectedResourceMetadataHandler(cfg, true))
}

func protectedResourceMetadataHandler(cfg config.AuthConfig, mcpResource bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.ServerPublicURL == "" {
			logger.GetLogger(r.Context()).Error("SERVER_PUBLIC_URL is not configured; cannot serve protected resource metadata")
			http.Error(w, "protected resource metadata not configured", http.StatusServiceUnavailable)
			return
		}
		if len(cfg.AuthorizationServers) == 0 {
			logger.GetLogger(r.Context()).Error("OAUTH_AUTHORIZATION_SERVERS is not configured; cannot serve protected resource metadata")
			http.Error(w, "protected resource metadata not configured", http.StatusServiceUnavailable)
			return
		}

		resource := ampAPIResourceIdentifier
		if mcpResource {
			resource = strings.TrimSuffix(cfg.ServerPublicURL, "/") + "/mcp"
		}

		body := protectedResourceMetadata{
			Resource:               resource,
			AuthorizationServers:   cfg.AuthorizationServers,
			BearerMethodsSupported: []string{"header"},
			ScopesSupported:        cfg.ScopesSupported,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			logger.GetLogger(r.Context()).Error("failed to encode protected resource metadata", "error", err)
		}
	}
}
