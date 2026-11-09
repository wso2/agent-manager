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
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

// registerSecretStoreRoute serves the in-house ClusterSecretStore (External
// Secrets Operator webhook provider). It returns one key of one secret as
// {"value": "..."} and accepts only the store's bearer token. An empty token
// leaves the route closed.
//
// {ref} is the SecretReference entry's remoteRef.key, "<ouID>/<name>". ESO
// escapes it into a single path segment (the slash arrives as %2F), and
// PathValue returns it unescaped.
func registerSecretStoreRoute(mux *http.ServeMux, resolver secretmanagersvc.SecretResolver, token string) {
	mux.HandleFunc("GET /secrets/{ref}/{key}", func(w http.ResponseWriter, r *http.Request) {
		if !secretStoreAuthorized(r, token) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ouID, name, ok := strings.Cut(r.PathValue("ref"), "/")
		if !ok || ouID == "" || name == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		value, err := resolver.Resolve(r.Context(), ouID, name, r.PathValue("key"))
		if err != nil {
			if errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			logger.GetLogger(r.Context()).Error("secret store: failed to resolve secret",
				"ouID", ouID, "name", name, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(map[string]string{"value": value}); err != nil {
			logger.GetLogger(r.Context()).Error("secret store: failed to write response", "error", err)
		}
	})
}

func secretStoreAuthorized(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}
