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
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// PathParamWebhookID is the webhook path parameter.
const PathParamWebhookID = "webhookId"

// WebhookController serves webhook endpoints at org, project and agent scope.
// The scope is fixed per route: each handler is bound to one through ForScope.
type WebhookController interface {
	ListEventTypes(w http.ResponseWriter, r *http.Request)
	ForScope(scope string) WebhookScopeHandlers
}

// WebhookScopeHandlers are the handlers for one scope.
type WebhookScopeHandlers struct {
	List           http.HandlerFunc
	Create         http.HandlerFunc
	Get            http.HandlerFunc
	Update         http.HandlerFunc
	Delete         http.HandlerFunc
	RotateSecret   http.HandlerFunc
	Test           http.HandlerFunc
	ListDeliveries http.HandlerFunc
}

type webhookController struct {
	svc services.WebhookService
}

// NewWebhookController creates a WebhookController.
func NewWebhookController(svc services.WebhookService) WebhookController {
	return &webhookController{svc: svc}
}

func (c *webhookController) ListEventTypes(w http.ResponseWriter, r *http.Request) {
	types, err := c.svc.EventTypes(r.URL.Query().Get("scope"))
	if err != nil {
		writeWebhookError(w, r, err, "Failed to list event types")
		return
	}
	resp := spec.WebhookEventTypeListResponse{EventTypes: make([]spec.WebhookEventType, 0, len(types))}
	for _, t := range types {
		resp.EventTypes = append(resp.EventTypes, spec.WebhookEventType{
			Type: t.Name, Scope: string(t.Scope), Category: t.Category, Description: t.Description,
		})
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// webhookManagePermission is the permission that may change a scope's
// webhooks, and so see their full URLs.
var webhookManagePermission = map[string]rbac.Permission{
	models.WebhookScopeOrg:     rbac.AlertingManage,
	models.WebhookScopeProject: rbac.ProjectUpdate,
	models.WebhookScopeAgent:   rbac.AgentUpdate,
}

func (c *webhookController) ForScope(scope string) WebhookScopeHandlers {
	// A webhook URL often carries its credential in the path or query, so
	// callers who can only read a scope's webhooks see scheme and host only.
	canSeeURL := func(r *http.Request) bool {
		return jwtassertion.HoldsAnyScope(r.Context(), webhookManagePermission[scope])
	}
	target := func(r *http.Request) repositories.WebhookTarget {
		t := repositories.WebhookTarget{OUID: middleware.OUIDFromRequest(r), Scope: scope}
		if scope != models.WebhookScopeOrg {
			t.ProjectName = r.PathValue(utils.PathParamProjName)
		}
		if scope == models.WebhookScopeAgent {
			t.AgentName = r.PathValue(utils.PathParamAgentName)
		}
		return t
	}
	withID := func(fn func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(r.PathValue(PathParamWebhookID))
			if err != nil {
				utils.WriteErrorResponse(w, http.StatusNotFound, "Webhook not found")
				return
			}
			fn(w, r, target(r), id)
		}
	}

	return WebhookScopeHandlers{
		List: func(w http.ResponseWriter, r *http.Request) {
			list, err := c.svc.List(r.Context(), target(r))
			if err != nil {
				writeWebhookError(w, r, err, "Failed to list webhooks")
				return
			}
			resp := spec.WebhookListResponse{Webhooks: make([]spec.WebhookResponse, 0, len(list))}
			fullURL := canSeeURL(r)
			for i := range list {
				resp.Webhooks = append(resp.Webhooks, toWebhookResponse(&list[i], "", fullURL))
			}
			utils.WriteSuccessResponse(w, http.StatusOK, resp)
		},
		Create: func(w http.ResponseWriter, r *http.Request) {
			in, ok := decodeWebhookRequest(w, r)
			if !ok {
				return
			}
			e, secret, err := c.svc.Create(r.Context(), target(r), in)
			if err != nil {
				writeWebhookError(w, r, err, "Failed to create webhook")
				return
			}
			utils.WriteSuccessResponse(w, http.StatusCreated, toWebhookResponse(e, secret, true))
		},
		Get: withID(func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
			e, err := c.svc.Get(r.Context(), t, id)
			if err != nil {
				writeWebhookError(w, r, err, "Failed to get webhook")
				return
			}
			utils.WriteSuccessResponse(w, http.StatusOK, toWebhookResponse(e, "", canSeeURL(r)))
		}),
		Update: withID(func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
			in, ok := decodeWebhookRequest(w, r)
			if !ok {
				return
			}
			e, err := c.svc.Update(r.Context(), t, id, in)
			if err != nil {
				writeWebhookError(w, r, err, "Failed to update webhook")
				return
			}
			utils.WriteSuccessResponse(w, http.StatusOK, toWebhookResponse(e, "", true))
		}),
		Delete: withID(func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
			if err := c.svc.Delete(r.Context(), t, id); err != nil {
				writeWebhookError(w, r, err, "Failed to delete webhook")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
		RotateSecret: withID(func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
			secret, err := c.svc.RotateSecret(r.Context(), t, id)
			if err != nil {
				writeWebhookError(w, r, err, "Failed to rotate webhook secret")
				return
			}
			utils.WriteSuccessResponse(w, http.StatusOK, spec.WebhookSecretResponse{SigningSecret: secret})
		}),
		Test: withID(func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
			result, err := c.svc.Test(r.Context(), t, id)
			if err != nil {
				writeWebhookError(w, r, err, "Failed to send test event")
				return
			}
			resp := spec.WebhookTestResponse{Delivered: result.Delivered}
			if result.StatusCode != 0 {
				code := int32(result.StatusCode)
				resp.StatusCode = &code
			}
			if result.Error != "" {
				resp.Error = &result.Error
			}
			utils.WriteSuccessResponse(w, http.StatusOK, resp)
		}),
		ListDeliveries: withID(func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
			limit := 0
			if raw := r.URL.Query().Get("limit"); raw != "" {
				parsed, err := strconv.Atoi(raw)
				if err != nil || parsed < 1 {
					utils.WriteErrorResponse(w, http.StatusBadRequest, "limit must be a positive integer")
					return
				}
				limit = parsed
			}
			list, err := c.svc.ListDeliveries(r.Context(), t, id, limit)
			if err != nil {
				writeWebhookError(w, r, err, "Failed to list webhook deliveries")
				return
			}
			resp := spec.WebhookDeliveryListResponse{Deliveries: make([]spec.WebhookDeliveryResponse, 0, len(list))}
			for i := range list {
				resp.Deliveries = append(resp.Deliveries, toWebhookDeliveryResponse(&list[i]))
			}
			utils.WriteSuccessResponse(w, http.StatusOK, resp)
		}),
	}
}

func decodeWebhookRequest(w http.ResponseWriter, r *http.Request) (services.WebhookEndpointInput, bool) {
	var req spec.WebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return services.WebhookEndpointInput{}, false
	}
	in := services.WebhookEndpointInput{
		Name:       req.Name,
		URL:        req.Url,
		EventTypes: req.EventTypes,
		Enabled:    req.Enabled == nil || *req.Enabled,
	}
	if req.Description != nil {
		in.Description = *req.Description
	}
	in.Environments = req.Environments
	if in.EventTypes == nil {
		in.EventTypes = []string{}
	}
	return in, true
}

func writeWebhookError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, utils.ErrWebhookNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Webhook not found")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, audit.ErrRecorderUnavailable):
		logger.GetLogger(r.Context()).Error(fallback, "error", err)
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "Audit trail unavailable; the change was not applied")
	default:
		logger.GetLogger(r.Context()).Error(fallback, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallback)
	}
}

// maskWebhookURL keeps scheme and host and hides the rest.
func maskWebhookURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	masked := u.Scheme + "://" + u.Host
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		masked += "/…"
	}
	return masked
}

func toWebhookResponse(e *models.WebhookEndpoint, secret string, fullURL bool) spec.WebhookResponse {
	types := e.EventTypes
	if types == nil {
		types = []string{}
	}
	resp := spec.WebhookResponse{
		Id:          e.ID.String(),
		Scope:       e.Scope,
		Name:        e.Name,
		Description: e.Description,
		Url:         e.URL,
		EventTypes:  types,
		Enabled:     e.Enabled,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
	resp.Environments = e.Environments
	if resp.Environments == nil {
		resp.Environments = []string{}
	}
	if !fullURL {
		resp.Url = maskWebhookURL(e.URL)
	}
	if secret != "" {
		resp.SigningSecret = &secret
	}
	return resp
}

func toWebhookDeliveryResponse(d *models.WebhookDelivery) spec.WebhookDeliveryResponse {
	resp := spec.WebhookDeliveryResponse{
		Id:          d.ID.String(),
		EventId:     d.EventID,
		EventType:   d.EventType,
		Status:      d.Status,
		Attempts:    int32(d.Attempts),
		DeliveredAt: d.DeliveredAt,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
	if d.ResponseCode != nil {
		code := int32(*d.ResponseCode)
		resp.ResponseCode = &code
	}
	if d.LastError != "" {
		msg := d.LastError
		resp.LastError = &msg
	}
	return resp
}
