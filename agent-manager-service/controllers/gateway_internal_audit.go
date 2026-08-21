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

package controllers

import (
	"net/http"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// recordGatewayAuthFailure records a rejected gateway api-key.
//
// The internal server has no JWT middleware, so its authentication happens
// inside each handler and previously produced no event at all. That left a
// gateway-credential brute force with no signal anywhere: these keys grant
// access to every API key the gateway is entitled to sync.
//
// The presented key is never recorded, only whether one was supplied and why it
// was refused. Failures are rate-limited per source so a misconfigured gateway
// retrying in a loop cannot flood the trail.
func recordGatewayAuthFailure(r *http.Request, reason, gatewayID string) {
	ip := utils.ClientIP(r)
	if !gatewayAuthLimiter.allow(ip + "|" + reason) {
		return
	}

	audit.RecordAncillary(
		r.Context(), audit.ActionAuthnFailure,
		audit.SurfaceOpt(audit.SurfaceInternal),
		audit.Actor(audit.ActorGateway, gatewayID, ""),
		audit.AuthMethod("api-key"),
		audit.OutcomeOpt(audit.OutcomeDeny),
		audit.Status(http.StatusUnauthorized),
		audit.Detail("reason", reason),
		audit.Detail("authHeader", r.Header.Get("api-key") != ""),
	)
}

// gatewayAuthLimiter bounds how often an identical gateway auth failure is
// recorded. Gateways poll continuously, so one with a stale key would otherwise
// emit an event every few seconds indefinitely.
var gatewayAuthLimiter = &coalescer{window: time.Minute, seen: map[string]time.Time{}}

// coalescer suppresses repeats of the same key within a window.
//
// Used for the internal surface, where gateways poll on a timer: without it the
// bulk-sync reads and their failures would produce millions of near-identical
// records and bury everything else.
type coalescer struct {
	mu     sync.Mutex
	window time.Duration
	seen   map[string]time.Time
	// lastSweep bounds map growth; a scan runs at most once per window.
	lastSweep time.Time
}

// allow reports whether this key should be recorded now.
func (c *coalescer) allow(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if now.Sub(c.lastSweep) >= c.window {
		c.lastSweep = now
		for k, at := range c.seen {
			if now.Sub(at) >= c.window {
				delete(c.seen, k)
			}
		}
	}

	if at, ok := c.seen[key]; ok && now.Sub(at) < c.window {
		return false
	}
	c.seen[key] = now
	return true
}

// authenticateGateway resolves the gateway behind an internal-API request.
//
// Every internal handler repeated this block; centralising it is what gives the
// rejection path a single place to be recorded. It writes the HTTP error itself
// and reports whether the caller may continue.
func (c *gatewayInternalController) authenticateGateway(
	w http.ResponseWriter,
	r *http.Request,
	operation string,
) (gatewayIdentity, bool) {
	log := logger.GetLogger(r.Context())
	clientIP := utils.ClientIP(r)

	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("Unauthorized access attempt - Missing API key", "ip", clientIP, "operation", operation)
		recordGatewayAuthFailure(r, "missing-api-key", "")
		http.Error(w, "API key is required. Provide 'api-key' header.", http.StatusUnauthorized)
		return gatewayIdentity{}, false
	}

	gateway, err := c.gatewayService.VerifyToken(r.Context(), apiKey)
	if err != nil {
		log.Warn("Authentication failed", "ip", clientIP, "operation", operation, "error", err)
		recordGatewayAuthFailure(r, "invalid-api-key", "")
		http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
		return gatewayIdentity{}, false
	}

	gatewayID := gateway.UUID.String()
	// Name the caller now that it is known. The coverage envelope is emitted
	// after the handler returns and coalesces per caller; until this point the
	// request carries only an address, which cannot tell two gateways apart.
	audit.IdentifyActor(r.Context(), audit.ActorGateway, gatewayID, "api-key")

	return gatewayIdentity{ID: gatewayID, OUID: gateway.OUID}, true
}

// gatewayIdentity is the subset of an authenticated gateway the internal
// handlers and their audit records need.
type gatewayIdentity struct {
	ID   string
	OUID string
}
