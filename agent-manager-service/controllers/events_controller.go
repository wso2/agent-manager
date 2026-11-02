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

// Path parameters of the events routes.
const (
	PathParamEventDestinationID  = "destinationId"
	PathParamEventSubscriptionID = "subscriptionId"
)

// EventsController serves event destinations, which only the org manages,
// and the event subscriptions and delivery log of the org, a project or an
// agent. The scope of the latter is fixed per route through ForScope.
type EventsController interface {
	ListDestinationKinds(w http.ResponseWriter, r *http.Request)
	ListDestinations(w http.ResponseWriter, r *http.Request)
	CreateDestination(w http.ResponseWriter, r *http.Request)
	GetDestination(w http.ResponseWriter, r *http.Request)
	UpdateDestination(w http.ResponseWriter, r *http.Request)
	DeleteDestination(w http.ResponseWriter, r *http.Request)
	RotateDestinationSecret(w http.ResponseWriter, r *http.Request)
	TestDestination(w http.ResponseWriter, r *http.Request)
}

// EventScopeHandlers are the handlers of one scope.

type eventsController struct {
	destinations services.EventDestinationService
}

// NewEventsController creates an EventsController.
func NewEventsController(destinations services.EventDestinationService) EventsController {
	return &eventsController{destinations: destinations}
}

func (c *eventsController) ListDestinationKinds(w http.ResponseWriter, _ *http.Request) {
	kinds := c.destinations.Kinds()
	resp := spec.EventDestinationKindListResponse{Kinds: make([]spec.EventDestinationKind, 0, len(kinds))}
	for _, k := range kinds {
		kind := spec.EventDestinationKind{
			Kind:               k.Kind,
			GeneratedSecret:    k.GeneratedSecret,
			SuppliedCredential: k.SuppliedCredential,
			Services:           make([]spec.EventDestinationKindService, 0, len(k.Services)),
		}
		for _, s := range k.Services {
			kind.Services = append(kind.Services, spec.EventDestinationKindService{
				Id: s.ID, Label: s.Label, Category: s.Category, UrlFormat: s.URLFormat, DocsUrl: s.DocsURL,
			})
		}
		resp.Kinds = append(resp.Kinds, kind)
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// canManageDestinations reports whether the caller may change destinations,
// and so see their settings: a destination's settings may carry a
// credential (a webhook URL path, say).
func canManageDestinations(r *http.Request) bool {
	return jwtassertion.HoldsAnyScope(r.Context(), rbac.EventsManage)
}

func (c *eventsController) ListDestinations(w http.ResponseWriter, r *http.Request) {
	list, stats, err := c.destinations.List(r.Context(), middleware.OUIDFromRequest(r))
	if err != nil {
		writeEventsError(w, r, err, "Failed to list event destinations")
		return
	}
	full := canManageDestinations(r)
	resp := spec.EventDestinationListResponse{Destinations: make([]spec.EventDestinationResponse, 0, len(list))}
	for i := range list {
		resp.Destinations = append(resp.Destinations, c.toDestinationResponse(&list[i], stats[list[i].ID], full))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *eventsController) CreateDestination(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeDestinationRequest(w, r)
	if !ok {
		return
	}
	d, secret, err := c.destinations.Create(r.Context(), middleware.OUIDFromRequest(r), in)
	if err != nil {
		writeEventsError(w, r, err, "Failed to add event destination")
		return
	}
	resp := c.toDestinationResponse(d, repositories.DestinationStats{}, true)
	if secret != "" {
		resp.Secret = &secret
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, resp)
}

// destinationID parses the destination path parameter, answering 404 when
// it is not an ID.
func destinationID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(PathParamEventDestinationID))
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Event destination not found")
		return uuid.Nil, false
	}
	return id, true
}

func (c *eventsController) GetDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := destinationID(w, r)
	if !ok {
		return
	}
	ouID := middleware.OUIDFromRequest(r)
	d, err := c.destinations.Get(r.Context(), ouID, id)
	if err != nil {
		writeEventsError(w, r, err, "Failed to get event destination")
		return
	}
	using, err := c.destinations.Usage(r.Context(), ouID, id)
	if err != nil {
		writeEventsError(w, r, err, "Failed to get event destination")
		return
	}
	_, stats, err := c.destinations.List(r.Context(), ouID)
	if err != nil {
		writeEventsError(w, r, err, "Failed to get event destination")
		return
	}
	resp := c.toDestinationResponse(d, stats[d.ID], canManageDestinations(r))
	resp.Subscriptions = make([]spec.EventSubscriptionRef, 0, len(using))
	for i := range using {
		resp.Subscriptions = append(resp.Subscriptions, toSubscriptionRef(&using[i]))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *eventsController) UpdateDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := destinationID(w, r)
	if !ok {
		return
	}
	in, ok := decodeDestinationRequest(w, r)
	if !ok {
		return
	}
	ouID := middleware.OUIDFromRequest(r)
	d, err := c.destinations.Update(r.Context(), ouID, id, in)
	if err != nil {
		writeEventsError(w, r, err, "Failed to update event destination")
		return
	}
	_, stats, err := c.destinations.List(r.Context(), ouID)
	if err != nil {
		writeEventsError(w, r, err, "Failed to update event destination")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, c.toDestinationResponse(d, stats[d.ID], true))
}

func (c *eventsController) DeleteDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := destinationID(w, r)
	if !ok {
		return
	}
	if err := c.destinations.Delete(r.Context(), middleware.OUIDFromRequest(r), id); err != nil {
		writeEventsError(w, r, err, "Failed to delete event destination")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *eventsController) RotateDestinationSecret(w http.ResponseWriter, r *http.Request) {
	id, ok := destinationID(w, r)
	if !ok {
		return
	}
	secret, err := c.destinations.RotateSecret(r.Context(), middleware.OUIDFromRequest(r), id)
	if err != nil {
		writeEventsError(w, r, err, "Failed to rotate event destination secret")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, spec.EventDestinationSecretResponse{Secret: secret})
}

func (c *eventsController) TestDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := destinationID(w, r)
	if !ok {
		return
	}
	result, err := c.destinations.Test(r.Context(), middleware.OUIDFromRequest(r), id)
	if err != nil {
		writeEventsError(w, r, err, "Failed to send test event")
		return
	}
	resp := spec.EventDestinationTestResponse{Delivered: result.Delivered}
	if result.StatusCode != 0 {
		code := int32(result.StatusCode)
		resp.StatusCode = &code
	}
	if result.Error != "" {
		resp.Error = &result.Error
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func decodeDestinationRequest(w http.ResponseWriter, r *http.Request) (services.EventDestinationInput, bool) {
	var req spec.EventDestinationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return services.EventDestinationInput{}, false
	}
	cfg, err := json.Marshal(req.Config)
	if err != nil || req.Config == nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "config must be an object")
		return services.EventDestinationInput{}, false
	}
	in := services.EventDestinationInput{
		Name:         req.Name,
		Kind:         req.Kind,
		Config:       cfg,
		Availability: req.Availability,
		Projects:     req.Projects,
	}
	if req.Description != nil {
		in.Description = *req.Description
	}
	if in.Projects == nil {
		in.Projects = []string{}
	}
	return in, true
}

func writeEventsError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, utils.ErrEventDestinationNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Event destination not found")
	case errors.Is(err, utils.ErrEventSubscriptionNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Event subscription not found")
	case errors.Is(err, utils.ErrConflict):
		utils.WriteErrorResponse(w, http.StatusConflict, "Destination changed concurrently; reload and retry")
	case errors.Is(err, utils.ErrEventDestinationInUse):
		utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, utils.ErrEventDestinationNameTaken):
		utils.WriteErrorResponse(w, http.StatusConflict, "An event destination with this name already exists")
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

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (c *eventsController) toDestinationResponse(
	d *models.EventDestination, stats repositories.DestinationStats, fullConfig bool,
) spec.EventDestinationResponse {
	raw := d.Config
	if !fullConfig {
		raw = c.destinations.RedactedConfig(d)
	}
	cfg := map[string]interface{}{}
	_ = json.Unmarshal(raw, &cfg)
	resp := spec.EventDestinationResponse{
		Id:                  d.ID.String(),
		Name:                d.Name,
		Description:         d.Description,
		Kind:                d.Kind,
		Service:             optionalString(c.destinations.Service(d)),
		Config:              cfg,
		Availability:        d.Availability,
		Projects:            d.Projects,
		UsedBy:              int32(stats.UsedBy),
		LastDeliveryAt:      stats.LastDeliveryAt,
		Health:              services.DestinationHealthOf(stats),
		ConsecutiveFailures: int32(stats.ConsecutiveFailures),
		CreatedBy:           d.CreatedBy,
		CreatedAt:           d.CreatedAt,
		UpdatedAt:           d.UpdatedAt,
	}
	if resp.Projects == nil {
		resp.Projects = []string{}
	}
	return resp
}

func (c *eventsController) toDestinationSummary(
	d *models.EventDestination, stats repositories.DestinationStats,
) spec.EventDestinationSummary {
	return spec.EventDestinationSummary{
		Id:      d.ID.String(),
		Name:    d.Name,
		Kind:    d.Kind,
		Service: optionalString(c.destinations.Service(d)),
		Health:  services.DestinationHealthOf(stats),
	}
}

func toSubscriptionRef(s *models.EventSubscription) spec.EventSubscriptionRef {
	return spec.EventSubscriptionRef{
		Id:          s.ID.String(),
		Name:        s.Name,
		Scope:       s.Scope,
		ProjectName: optionalString(s.ProjectName),
		AgentName:   optionalString(s.AgentName),
	}
}

func toDeliveryCounts(byStatus map[string]int) spec.EventDeliveryCounts {
	return spec.EventDeliveryCounts{
		Delivered: int32(byStatus[models.EventDeliveryDelivered]),
		Failed:    int32(byStatus[models.EventDeliveryFailed]),
		Pending:   int32(byStatus[models.EventDeliveryPending]),
		Skipped:   int32(byStatus[models.EventDeliverySkipped]),
	}
}
