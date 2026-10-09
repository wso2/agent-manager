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

// The dispatcher's internal webhook API. In production the API instance owns
// authorization, audit and org checks; it forwards webhook storage here, so
// only the dispatcher reads and writes its database. The API is not exposed
// to users: it sits on the cluster network and every call carries a key
// shared by the two instances.

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// WebhookInternalAPIKeyHeader carries the shared key.
	WebhookInternalAPIKeyHeader = "X-Dispatcher-Key"
	webhookInternalBase         = "/internal/v1/webhooks"
	maxInternalRequestBytes     = 1 << 20
)

// webhookCreateResponse is the internal API's answer to a create.
type webhookCreateResponse struct {
	Webhook       models.WebhookEndpoint `json:"webhook"`
	SigningSecret string                 `json:"signingSecret"`
}

type webhookSecretResponse struct {
	SigningSecret string `json:"signingSecret"`
}

// webhookAPIError is the internal API's error body.
type webhookAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	webhookErrNotFound     = "not_found"
	webhookErrInvalidInput = "invalid_input"
	webhookErrInternal     = "internal"
)

// NewWebhookInternalAPI serves a store over HTTP for an API instance's
// remote store. apiKey must be non-empty.
func NewWebhookInternalAPI(store WebhookStore, apiKey string, logger *slog.Logger) http.Handler {
	h := &webhookInternalAPI{store: store, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+webhookInternalBase, h.list)
	mux.HandleFunc("POST "+webhookInternalBase, h.create)
	mux.HandleFunc("DELETE "+webhookInternalBase, h.removeUnder)
	mux.HandleFunc("GET "+webhookInternalBase+"/{id}", h.withID(h.get))
	mux.HandleFunc("PUT "+webhookInternalBase+"/{id}", h.withID(h.update))
	mux.HandleFunc("DELETE "+webhookInternalBase+"/{id}", h.withID(h.delete))
	mux.HandleFunc("POST "+webhookInternalBase+"/{id}/rotate-secret", h.withID(h.rotate))
	mux.HandleFunc("POST "+webhookInternalBase+"/{id}/test", h.withID(h.test))
	mux.HandleFunc("GET "+webhookInternalBase+"/{id}/deliveries", h.withID(h.deliveries))

	key := []byte(apiKey)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get(WebhookInternalAPIKeyHeader))
		if len(key) == 0 || subtle.ConstantTimeCompare(got, key) != 1 {
			writeInternalError(w, http.StatusUnauthorized, webhookErrInvalidInput, "invalid dispatcher key")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxInternalRequestBytes)
		mux.ServeHTTP(w, r)
	})
}

type webhookInternalAPI struct {
	store  WebhookStore
	logger *slog.Logger
}

// targetOf reads the scope target from the query string.
func targetOf(r *http.Request) (repositories.WebhookTarget, bool) {
	q := r.URL.Query()
	t := repositories.WebhookTarget{
		OUID:        q.Get("ouId"),
		Scope:       q.Get("scope"),
		ProjectName: q.Get("project"),
		AgentName:   q.Get("agent"),
	}
	switch t.Scope {
	case models.WebhookScopeOrg:
		return t, t.OUID != ""
	case models.WebhookScopeProject:
		return t, t.OUID != "" && t.ProjectName != ""
	case models.WebhookScopeAgent:
		return t, t.OUID != "" && t.ProjectName != "" && t.AgentName != ""
	default:
		return t, false
	}
}

func (h *webhookInternalAPI) withID(
	fn func(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, ok := targetOf(r)
		if !ok {
			writeInternalError(w, http.StatusBadRequest, webhookErrInvalidInput, "invalid webhook target")
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeInternalError(w, http.StatusNotFound, webhookErrNotFound, "webhook not found")
			return
		}
		fn(w, r, t, id)
	}
}

func (h *webhookInternalAPI) list(w http.ResponseWriter, r *http.Request) {
	t, ok := targetOf(r)
	if !ok {
		writeInternalError(w, http.StatusBadRequest, webhookErrInvalidInput, "invalid webhook target")
		return
	}
	list, err := h.store.List(r.Context(), t)
	h.reply(w, list, err)
}

func (h *webhookInternalAPI) create(w http.ResponseWriter, r *http.Request) {
	t, ok := targetOf(r)
	if !ok {
		writeInternalError(w, http.StatusBadRequest, webhookErrInvalidInput, "invalid webhook target")
		return
	}
	var in WebhookEndpointInput
	if !decodeInternal(w, r, &in) {
		return
	}
	e, secret, err := h.store.Create(r.Context(), t, in)
	if err != nil {
		h.reply(w, nil, err)
		return
	}
	h.reply(w, webhookCreateResponse{Webhook: *e, SigningSecret: secret}, nil)
}

func (h *webhookInternalAPI) get(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
	e, err := h.store.Get(r.Context(), t, id)
	h.reply(w, e, err)
}

func (h *webhookInternalAPI) update(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
	var in WebhookEndpointInput
	if !decodeInternal(w, r, &in) {
		return
	}
	e, err := h.store.Update(r.Context(), t, id, in)
	h.reply(w, e, err)
}

func (h *webhookInternalAPI) delete(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
	if err := h.store.Delete(r.Context(), t, id); err != nil {
		h.reply(w, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *webhookInternalAPI) rotate(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
	secret, err := h.store.RotateSecret(r.Context(), t, id)
	if err != nil {
		h.reply(w, nil, err)
		return
	}
	h.reply(w, webhookSecretResponse{SigningSecret: secret}, nil)
}

func (h *webhookInternalAPI) test(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
	result, err := h.store.Test(r.Context(), t, id)
	h.reply(w, result, err)
}

func (h *webhookInternalAPI) deliveries(w http.ResponseWriter, r *http.Request, t repositories.WebhookTarget, id uuid.UUID) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.store.ListDeliveries(r.Context(), t, id, limit)
	h.reply(w, list, err)
}

// removeUnder deletes the webhooks of a deleted project (agent empty) or agent.
func (h *webhookInternalAPI) removeUnder(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ouID, project, agent := q.Get("ouId"), q.Get("project"), q.Get("agent")
	if ouID == "" || project == "" {
		writeInternalError(w, http.StatusBadRequest, webhookErrInvalidInput, "ouId and project are required")
		return
	}
	if agent == "" {
		h.store.RemoveForProject(r.Context(), ouID, project)
	} else {
		h.store.RemoveForAgent(r.Context(), ouID, project, agent)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *webhookInternalAPI) reply(w http.ResponseWriter, body any, err error) {
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	case errors.Is(err, utils.ErrWebhookNotFound):
		writeInternalError(w, http.StatusNotFound, webhookErrNotFound, "webhook not found")
	case errors.Is(err, utils.ErrInvalidInput):
		writeInternalError(w, http.StatusBadRequest, webhookErrInvalidInput, err.Error())
	default:
		h.logger.Error("Webhook internal API call failed", "error", err)
		writeInternalError(w, http.StatusInternalServerError, webhookErrInternal, "internal error")
	}
}

func decodeInternal(w http.ResponseWriter, r *http.Request, into any) bool {
	body, err := io.ReadAll(r.Body)
	if err == nil {
		err = json.Unmarshal(body, into)
	}
	if err != nil {
		writeInternalError(w, http.StatusBadRequest, webhookErrInvalidInput, "invalid request body")
		return false
	}
	return true
}

func writeInternalError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(webhookAPIError{Code: code, Message: message})
}
