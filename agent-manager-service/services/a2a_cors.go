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

package services

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// a2aVersionHeader is the header the gateway requires on every A2A operation.
// A browser can only send it if CORS allows it, so it is always allowed.
const a2aVersionHeader = "A2A-Version"

// withA2AVersionHeader guarantees A2A-Version is an allowed CORS header. It is
// not a default a user can remove: without it no browser A2A client works.
func withA2AVersionHeader(cfg resolvedCORSConfig) resolvedCORSConfig {
	alreadyAllowed := slices.ContainsFunc(cfg.CORSAllowHeaders, func(h string) bool {
		return strings.EqualFold(h, a2aVersionHeader)
	})
	if alreadyAllowed {
		return cfg
	}
	cfg.CORSAllowHeaders = append(slices.Clone(cfg.CORSAllowHeaders), a2aVersionHeader)
	return cfg
}

// resolveCardCORSOverride picks the card CORS override to persist: the
// request's when it carries one (inherit clearing it), otherwise the stored one.
func resolveCardCORSOverride(existing *models.AgentConfig, req *spec.AgentCardCORSConfig) *models.CardCORS {
	if req == nil {
		return existing.CardCORSOverride()
	}
	if req.GetInherit() {
		return nil
	}
	return &models.CardCORS{
		Enabled:          req.GetEnabled(),
		AllowOrigins:     req.AllowOrigin,
		AllowHeaders:     req.AllowHeaders,
		AllowCredentials: req.GetAllowCredentials(),
	}
}

// validateCardCORS checks the request shape first (agentCardCorsConfig is
// A2A-only, and enabled is required unless inheriting), then the resolved
// override (an enabled card needs origins, and cannot pair credentials with "*").
func validateCardCORS(isA2A bool, req *spec.AgentCardCORSConfig, override *models.CardCORS) error {
	if req != nil && !isA2A {
		return fmt.Errorf("%w: agentCardCorsConfig applies only to A2A agents", utils.ErrInvalidInput)
	}
	if req != nil && !req.GetInherit() && req.Enabled == nil {
		return fmt.Errorf("%w: agentCardCorsConfig.enabled is required unless inherit is true", utils.ErrInvalidInput)
	}
	if override == nil || !override.Enabled {
		return nil
	}
	if len(override.AllowOrigins) == 0 {
		return fmt.Errorf("%w: agentCardCorsConfig.allowOrigin must list at least one origin when enabled", utils.ErrInvalidInput)
	}
	if override.AllowCredentials && slices.Contains(override.AllowOrigins, "*") {
		return fmt.Errorf("%w: agentCardCorsConfig.allowCredentials cannot be true when allowOrigin contains \"*\"", utils.ErrInvalidInput)
	}
	return nil
}

// buildCardPolicies is the public card route's policy list. An enabled card
// with no origins (an inherited card whose agent stores none) publishes
// nothing rather than a CORS policy that matches no one.
func buildCardPolicies(card models.CardCORS) []map[string]interface{} {
	if !card.Enabled || len(card.AllowOrigins) == 0 {
		return nil
	}
	return []map[string]interface{}{
		client.CORSPolicy(card.AllowOrigins, models.CardCORSMethods, card.AllowHeaders, card.AllowCredentials),
	}
}
