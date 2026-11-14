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
	"errors"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/fileauth"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxLoginBodyBytes caps the request body. The handler reads credentials, so
// nothing legitimate is large, and an uncapped JSON decode on an
// unauthenticated route is a free memory amplifier.
const maxLoginBodyBytes = 4 << 10

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// registerFileAuthRoutes registers the file-based login endpoint.
//
// The route sits at the root, beside the JWKS and well-known endpoints, and is
// not authenticated — it is where a caller goes to *get* a token. It is
// registered whether or not file mode is on, so that a deployment using an
// identity provider answers a login attempt with a clear 404 rather than
// nothing at all.
//
// Unlike its neighbours it is called by the browser rather than by a server,
// so it carries CORS of its own. The /api/v1 surface wraps its whole mux
// (app.go), which this route sits outside of: without this, the console's
// preflight OPTIONS gets a 405 from the ServeMux and the POST that follows is
// discarded by the browser for want of an Allow-Origin header — surfacing to
// the user as "could not reach the server" against a server that answered 200.
func registerFileAuthRoutes(
	mux *http.ServeMux,
	svc fileauth.Service,
	recorder audit.Recorder,
	allowedOrigin string,
) {
	// The route does not go through RouteRegistrar, so nothing installs an
	// audit recorder for it. Without this the service's fail-closed emit
	// reaches the "not installed" fallback and every login is refused.
	withRecorder := middleware.WithAuditRecorder(recorder, audit.SurfaceAPI)
	handler := middleware.CORS(allowedOrigin)(withRecorder(loginHandler(svc)))
	mux.Handle("POST /auth/login", handler)
	// The preflight the browser sends before the POST. ServeMux matches by
	// method, so without this pattern OPTIONS falls through to a 405 that never
	// reaches the CORS middleware.
	mux.Handle("OPTIONS /auth/login", handler)
}

func loginHandler(svc fileauth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !svc.Enabled() {
			utils.WriteErrorResponse(w, http.StatusNotFound,
				"file-based login is not enabled on this deployment")
			return
		}

		var req loginRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&req); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "request body must be JSON with username and password")
			return
		}

		username := strings.TrimSpace(req.Username)
		if username == "" || req.Password == "" {
			// Refused before reaching the service so that an empty submission
			// is a 400 about the request rather than a 401 about the user.
			utils.WriteErrorResponse(w, http.StatusBadRequest, "username and password are required")
			return
		}

		token, expiresIn, err := svc.Login(r.Context(), username, req.Password)
		if err != nil {
			if errors.Is(err, fileauth.ErrInvalidCredentials) {
				// One message for both an unknown user and a wrong password.
				utils.WriteErrorResponse(w, http.StatusUnauthorized, "invalid username or password")
				return
			}
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		// A bearer token must never be cached by a proxy or the browser.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(loginResponse{
			AccessToken: token,
			TokenType:   "Bearer",
			ExpiresIn:   expiresIn,
		})
	}
}
