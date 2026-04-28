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

package api

import (
	"encoding/json"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

func registerWellKnownRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		cfg := config.GetConfig()

		if cfg.ServerPublicURL == "" {
			logger.GetLogger(r.Context()).Error("SERVER_PUBLIC_URL is not configured; cannot serve protected resource metadata")
			http.Error(w, "protected resource metadata not configured", http.StatusServiceUnavailable)
			return
		}

		body := protectedResourceMetadata{
			Resource:               cfg.ServerPublicURL,
			AuthorizationServers:   cfg.KeyManagerConfigurations.Issuer,
			BearerMethodsSupported: []string{"header"},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			logger.GetLogger(r.Context()).Error("failed to encode protected resource metadata", "error", err)
		}
	})
}
