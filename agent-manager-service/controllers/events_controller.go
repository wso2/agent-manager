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

// Path parameters of the events routes.
const (
	PathParamEventDestinationID  = "destinationId"
	PathParamEventSubscriptionID = "subscriptionId"
)

// EventsController serves event destinations, which only the org manages,
// and the event subscriptions and delivery log of the org, a project or an
// agent. The scope of the latter is fixed per route through ForScope.
type EventsController interface {
	ListEventTypes(w http.ResponseWriter, r *http.Request)
	ListDestinationKinds(w http.ResponseWriter, r *http.Request)
	ListDestinations(w http.ResponseWriter, r *http.Request)
	CreateDestination(w http.ResponseWriter, r *http.Request)
	GetDestination(w http.ResponseWriter, r *http.Request)
	UpdateDestination(w http.ResponseWriter, r *http.Request)
	DeleteDestination(w http.ResponseWriter, r *http.Request)
	RotateDestinationSecret(w http.ResponseWriter, r *http.Request)
	TestDestination(w http.ResponseWriter, r *http.Request)
	ForScope(scope string) EventScopeHandlers
}

// EventScopeHandlers are the handlers of one scope.
type EventScopeHandlers struct {
	// AvailableDestinations lists, without their settings, the destinations
	// the scope's subscriptions can use. Below the org only.
	AvailableDestinations http.HandlerFunc
	ListSubscriptions     http.HandlerFunc
	CreateSubscription    http.HandlerFunc
	GetSubscription       http.HandlerFunc
	UpdateSubscription    http.HandlerFunc
	DeleteSubscription    http.HandlerFunc
	ListDeliveries        http.HandlerFunc
}

type eventsController struct {
	destinations  services.EventDestinationService
	subscriptions services.EventSubscriptionService
}

// NewEventsController creates an EventsController.
func NewEventsController(
	destinations services.EventDestinationService, subscriptions services.EventSubscriptionService,
) EventsController {
	return &eventsController{destinations: destinations, subscriptions: subscriptions}
}

func (c *eventsController) ListEventTypes(w http.ResponseWriter, r *http.Request) {
	types, err := c.subscriptions.EventTypes(r.URL.Query().Get("scope"))
	if err != nil {
		writeEventsError(w, r, err, "Failed to list event types")
		return
	}
	resp := spec.EventTypeListResponse{EventTypes: make([]spec.EventType, 0, len(types))}
	for _, t := range types {
		resp.EventTypes = append(resp.EventTypes, spec.EventType{
			Type: t.Name, Scope: string(t.Scope), Category: t.Category, Description: t.Description,
		})
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
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

func (c *eventsController) ForScope(scope string) EventScopeHandlers {
	target := func(r *http.Request) repositories.EventTarget {
		t := repositories.EventTarget{OUID: middleware.OUIDFromRequest(r), Scope: scope}
		if scope != models.EventScopeOrg {
			t.ProjectName = r.PathValue(utils.PathParamProjName)
		}
		if scope == models.EventScopeAgent {
			t.AgentName = r.PathValue(utils.PathParamAgentName)
		}
		return t
	}
	withID := func(fn func(w http.ResponseWriter, r *http.Request, t repositories.EventTarget, id uuid.UUID)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(r.PathValue(PathParamEventSubscriptionID))
			if err != nil {
				utils.WriteErrorResponse(w, http.StatusNotFound, "Event subscription not found")
				return
			}
			fn(w, r, target(r), id)
		}
	}
	// respond writes one subscription with its destination and counts.
	respond := func(w http.ResponseWriter, r *http.Request, t repositories.EventTarget, status int, sub *models.EventSubscription) {
		list, err := c.subscriptions.List(r.Context(), t)
		if err != nil {
			writeEventsError(w, r, err, "Failed to load event subscription")
			return
		}
		d, _ := c.destinations.Get(r.Context(), t.OUID, sub.DestinationID)
		utils.WriteSuccessResponse(w, status, c.toSubscriptionResponse(sub, d, list.DestinationStats, list.Counts[sub.ID]))
	}

	h := EventScopeHandlers{
		ListSubscriptions: func(w http.ResponseWriter, r *http.Request) {
			list, err := c.subscriptions.List(r.Context(), target(r))
			if err != nil {
				writeEventsError(w, r, err, "Failed to list event subscriptions")
				return
			}
			resp := spec.EventSubscriptionListResponse{
				Subscriptions: make([]spec.EventSubscriptionResponse, 0, len(list.Own)),
				Inherited:     make([]spec.EventSubscriptionResponse, 0, len(list.Inherited)),
			}
			for i := range list.Own {
				s := &list.Own[i]
				resp.Subscriptions = append(resp.Subscriptions, c.toSubscriptionResponse(
					s, list.Destinations[s.DestinationID], list.DestinationStats, list.Counts[s.ID]))
			}
			for i := range list.Inherited {
				s := &list.Inherited[i]
				resp.Inherited = append(resp.Inherited, c.toSubscriptionResponse(
					s, list.Destinations[s.DestinationID], list.DestinationStats, list.Counts[s.ID]))
			}
			utils.WriteSuccessResponse(w, http.StatusOK, resp)
		},
		CreateSubscription: func(w http.ResponseWriter, r *http.Request) {
			in, ok := decodeSubscriptionRequest(w, r)
			if !ok {
				return
			}
			t := target(r)
			sub, err := c.subscriptions.Create(r.Context(), t, in)
			if err != nil {
				writeEventsError(w, r, err, "Failed to create event subscription")
				return
			}
			respond(w, r, t, http.StatusCreated, sub)
		},
		GetSubscription: withID(func(w http.ResponseWriter, r *http.Request, t repositories.EventTarget, id uuid.UUID) {
			sub, err := c.subscriptions.Get(r.Context(), t, id)
			if err != nil {
				writeEventsError(w, r, err, "Failed to get event subscription")
				return
			}
			respond(w, r, t, http.StatusOK, sub)
		}),
		UpdateSubscription: withID(func(w http.ResponseWriter, r *http.Request, t repositories.EventTarget, id uuid.UUID) {
			in, ok := decodeSubscriptionRequest(w, r)
			if !ok {
				return
			}
			sub, err := c.subscriptions.Update(r.Context(), t, id, in)
			if err != nil {
				writeEventsError(w, r, err, "Failed to update event subscription")
				return
			}
			respond(w, r, t, http.StatusOK, sub)
		}),
		DeleteSubscription: withID(func(w http.ResponseWriter, r *http.Request, t repositories.EventTarget, id uuid.UUID) {
			if err := c.subscriptions.Delete(r.Context(), t, id); err != nil {
				writeEventsError(w, r, err, "Failed to delete event subscription")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
		ListDeliveries: func(w http.ResponseWriter, r *http.Request) {
			q, ok := parseDeliveryQuery(w, r)
			if !ok {
				return
			}
			log, err := c.subscriptions.Deliveries(r.Context(), target(r), q)
			if err != nil {
				writeEventsError(w, r, err, "Failed to list event deliveries")
				return
			}
			resp := spec.EventDeliveryListResponse{
				Deliveries: make([]spec.EventDeliveryResponse, 0, len(log.Deliveries)),
				Total:      int32(log.Total),
			}
			for i := range log.Deliveries {
				resp.Deliveries = append(resp.Deliveries, c.toDeliveryResponse(&log.Deliveries[i], log))
			}
			utils.WriteSuccessResponse(w, http.StatusOK, resp)
		},
	}
	if scope != models.EventScopeOrg {
		h.AvailableDestinations = func(w http.ResponseWriter, r *http.Request) {
			t := target(r)
			list, stats, err := c.destinations.Available(r.Context(), t.OUID, t.ProjectName)
			if err != nil {
				writeEventsError(w, r, err, "Failed to list event destinations")
				return
			}
			resp := spec.EventDestinationSummaryListResponse{
				Destinations: make([]spec.EventDestinationSummary, 0, len(list)),
			}
			for i := range list {
				resp.Destinations = append(resp.Destinations, c.toDestinationSummary(&list[i], stats[list[i].ID]))
			}
			utils.WriteSuccessResponse(w, http.StatusOK, resp)
		}
	}
	return h
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

func decodeSubscriptionRequest(w http.ResponseWriter, r *http.Request) (services.EventSubscriptionInput, bool) {
	var req spec.EventSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return services.EventSubscriptionInput{}, false
	}
	destination, err := uuid.Parse(req.DestinationId)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "destinationId must be the ID of an event destination")
		return services.EventSubscriptionInput{}, false
	}
	in := services.EventSubscriptionInput{
		Name:          req.Name,
		DestinationID: destination,
		EventTypes:    req.EventTypes,
		Environments:  req.Environments,
		Enabled:       req.Enabled == nil || *req.Enabled,
	}
	if req.Description != nil {
		in.Description = *req.Description
	}
	if in.EventTypes == nil {
		in.EventTypes = []string{}
	}
	if in.Environments == nil {
		in.Environments = []string{}
	}
	return in, true
}

// parseDeliveryQuery reads a delivery log's filters and paging.
func parseDeliveryQuery(w http.ResponseWriter, r *http.Request) (services.DeliveryQuery, bool) {
	query := r.URL.Query()
	q := services.DeliveryQuery{Status: query.Get("status")}
	for name, dst := range map[string]*int{"limit": &q.Limit, "offset": &q.Offset} {
		raw := query.Get(name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || (name == "limit" && n < 1) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, name+" must be a non-negative integer, and limit at least 1")
			return q, false
		}
		*dst = n
	}
	for name, dst := range map[string]**uuid.UUID{"destinationId": &q.DestinationID, "subscriptionId": &q.SubscriptionID} {
		raw := query.Get(name)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, name+" must be an ID")
			return q, false
		}
		*dst = &id
	}
	return q, true
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

func (c *eventsController) toSubscriptionResponse(
	s *models.EventSubscription, d *models.EventDestination,
	stats map[uuid.UUID]repositories.DestinationStats, counts map[string]int,
) spec.EventSubscriptionResponse {
	resp := spec.EventSubscriptionResponse{
		Id:           s.ID.String(),
		Scope:        s.Scope,
		ProjectName:  s.ProjectName,
		AgentName:    s.AgentName,
		Name:         s.Name,
		Description:  s.Description,
		EventTypes:   s.EventTypes,
		Environments: s.Environments,
		Enabled:      s.Enabled,
		CreatedBy:    s.CreatedBy,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		Last24h:      toDeliveryCounts(counts),
	}
	if d != nil {
		resp.Destination = c.toDestinationSummary(d, stats[d.ID])
	} else {
		resp.Destination = spec.EventDestinationSummary{
			Id: s.DestinationID.String(), Name: "Unknown destination", Health: services.DestinationIdle,
		}
	}
	if resp.EventTypes == nil {
		resp.EventTypes = []string{}
	}
	if resp.Environments == nil {
		resp.Environments = []string{}
	}
	return resp
}

func (c *eventsController) toDeliveryResponse(d *models.EventDelivery, log *services.DeliveryLog) spec.EventDeliveryResponse {
	resp := spec.EventDeliveryResponse{
		Id:            d.ID.String(),
		EventId:       d.EventID,
		EventType:     d.EventType,
		EventScope:    d.EventScope,
		ProjectName:   optionalString(d.ProjectName),
		AgentName:     optionalString(d.AgentName),
		Environment:   optionalString(d.Environment),
		Subscriptions: make([]spec.EventSubscriptionRef, 0, len(d.SubscriptionIDs)),
		Status:        d.Status,
		Attempts:      int32(d.Attempts),
		LastError:     optionalString(d.LastError),
		DeliveredAt:   d.DeliveredAt,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
	if dest, ok := log.Destinations[d.DestinationID]; ok {
		summary := c.toDestinationSummary(dest, log.DestinationStats[dest.ID])
		resp.Destination = &summary
	}
	for _, raw := range d.SubscriptionIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		// A subscription deleted since is left out.
		if s, ok := log.Subscriptions[id]; ok {
			resp.Subscriptions = append(resp.Subscriptions, toSubscriptionRef(s))
		}
	}
	if d.ResponseCode != nil {
		code := int32(*d.ResponseCode)
		resp.ResponseCode = &code
	}
	return resp
}
