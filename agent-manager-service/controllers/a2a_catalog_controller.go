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
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const maxA2ACatalogQueryLength = 255

// A2ACatalogController serves the organisation's A2A agent catalogue and how to connect to each agent.
type A2ACatalogController interface {
	ListA2AAgents(w http.ResponseWriter, r *http.Request)
	GetA2ACatalogEntry(w http.ResponseWriter, r *http.Request)
	GetA2AConnect(w http.ResponseWriter, r *http.Request)
}

type a2aCatalogController struct {
	catalogService services.A2ACatalogService
}

// NewA2ACatalogController creates an A2ACatalogController.
func NewA2ACatalogController(catalogService services.A2ACatalogService) A2ACatalogController {
	return &a2aCatalogController{catalogService: catalogService}
}

func (c *a2aCatalogController) ListA2AAgents(w http.ResponseWriter, r *http.Request) {
	filter, msg := parseA2ACatalogFilter(r)
	if msg != "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, msg)
		return
	}
	page, err := c.catalogService.ListA2ACatalog(r.Context(), filter)
	if err != nil {
		handleCommonErrors(w, err, "Failed to list A2A agents")
		return
	}
	agents := make([]spec.A2ACatalogAgent, 0, len(page.Entries))
	for i := range page.Entries {
		agents = append(agents, *toA2ACatalogAgent(&page.Entries[i]))
	}
	utils.WriteSuccessResponse(w, http.StatusOK,
		spec.NewA2ACatalogListResponse(agents, int32(page.Total), int32(page.Limit), int32(page.Offset)))
}

// parseA2ACatalogFilter checks the query parameters in the contract's order and returns the first failure's message.
func parseA2ACatalogFilter(r *http.Request) (models.A2ACatalogFilter, string) {
	query := r.URL.Query()
	limit, ok := parseBoundedInt(query.Get("limit"), utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)
	if !ok {
		return models.A2ACatalogFilter{}, "Invalid limit parameter: must be between 1 and 100"
	}
	// The response echoes the offset as int32, so a larger one is as invalid as a negative one.
	offset, ok := parseBoundedInt(query.Get("offset"), utils.DefaultOffset, utils.MinOffset, math.MaxInt32)
	if !ok {
		return models.A2ACatalogFilter{}, "Invalid offset parameter: must be 0 or greater"
	}
	q := strings.TrimSpace(query.Get("q"))
	if utf8.RuneCountInString(q) > maxA2ACatalogQueryLength {
		return models.A2ACatalogFilter{}, "Invalid q parameter: must be at most 255 characters"
	}
	kind := models.A2ACatalogKind(query.Get("kind"))
	switch kind {
	case "", models.A2ACatalogKindPlatform, models.A2ACatalogKindExternal, models.A2ACatalogKindProxied:
	default:
		return models.A2ACatalogFilter{}, "Invalid kind parameter: must be platform, external or proxied"
	}
	status := models.A2AAgentCardStatus(query.Get("status"))
	switch status {
	case "", models.A2AAgentCardStatusPending, models.A2AAgentCardStatusFetched, models.A2AAgentCardStatusFailed:
	default:
		return models.A2ACatalogFilter{}, "Invalid status parameter: must be pending, fetched or failed"
	}
	return models.A2ACatalogFilter{
		OUID:        middleware.OUIDFromRequest(r),
		Query:       q,
		Project:     query.Get("project"),
		Environment: query.Get("environment"),
		Kind:        kind,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	}, ""
}

// parseBoundedInt returns def for an empty raw value, and false for a non-integer or one outside [lo, hi].
func parseBoundedInt(raw string, def, lo, hi int) (int, bool) {
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, false
	}
	return n, true
}

func (c *a2aCatalogController) GetA2ACatalogEntry(w http.ResponseWriter, r *http.Request) {
	entry, err := c.catalogService.GetA2ACatalogEntry(r.Context(), middleware.OUIDFromRequest(r),
		r.PathValue(utils.PathParamProjName), r.PathValue(utils.PathParamAgentName))
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotInA2ACatalog) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "The agent is not in the A2A catalogue")
			return
		}
		handleCommonErrors(w, err, "Failed to get the A2A catalogue entry")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toA2ACatalogAgent(entry))
}

func (c *a2aCatalogController) GetA2AConnect(w http.ResponseWriter, r *http.Request) {
	ouID, projName, agentName, envName := agentCardPath(r)
	details, err := c.catalogService.GetA2AConnectDetails(r.Context(), ouID, projName, agentName, envName)
	if err != nil {
		writeAgentCardError(w, r, err, "Failed to get the A2A connect details")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, spec.NewA2AConnectResponse(
		string(details.Kind), details.Proxied, details.IsProduction,
		details.GatewayURL, details.CardURL, details.JSONRPCURL,
		*toA2AConnectSecurity(details.Security),
		*spec.NewA2AConnectPublication(details.Publication.Status, details.Publication.LastError)))
}

func toA2ACatalogAgent(entry *models.A2ACatalogEntry) *spec.A2ACatalogAgent {
	environments := make([]spec.A2ACatalogEnvironment, 0, len(entry.Environments))
	for _, env := range entry.Environments {
		out := spec.NewA2ACatalogEnvironment(env.Name, string(env.Status), env.Proxied, env.CardDiffers, env.Matched)
		if env.FetchedAt != nil {
			out.SetFetchedAt(*env.FetchedAt)
		}
		environments = append(environments, *out)
	}
	return spec.NewA2ACatalogAgent(entry.ProjectName, entry.AgentName, string(entry.Kind), entry.Name,
		entry.Description, entry.Version, nonNilStrings(entry.Skills), entry.LeadEnvironment, environments)
}

func toA2AConnectSecurity(security models.A2AConnectSecurity) *spec.A2AConnectSecurity {
	out := spec.NewA2AConnectSecurity(string(security.Mode))
	if security.APIKeyHeader != "" {
		out.SetApiKeyHeader(security.APIKeyHeader)
	}
	if security.Mode == models.A2AConnectSecurityOAuth && security.OAuth != nil {
		issuers := make([]spec.A2AConnectIssuer, 0, len(security.OAuth.Issuers))
		for _, issuer := range security.OAuth.Issuers {
			issuers = append(issuers, *spec.NewA2AConnectIssuer(issuer.Name, issuer.Issuer))
		}
		out.SetOauth(*spec.NewA2AConnectOAuth(issuers, nonNilStrings(security.OAuth.Audiences),
			security.OAuth.HeaderName, security.OAuth.AuthHeaderPrefix))
	}
	return out
}

// nonNilStrings keeps a required JSON array from serialising as null.
func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
