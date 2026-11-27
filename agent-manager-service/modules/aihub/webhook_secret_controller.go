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

package aihub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// The webhook secrets API, under /m/ai-hub/webhook-secrets: an organization's secrets for the developer portals
// that send it deliveries. A secret's value is returned once, when it is created or rotated; reads return a
// masked value and the hash a delivery names it by.

const (
	maxWebhookSecretBodyBytes = 16 << 10

	codeWebhookSecretNotFound     = "WEBHOOK_SECRET_NOT_FOUND"
	codeWebhookSecretExists       = "WEBHOOK_SECRET_EXISTS"
	codeWebhookSecretVersionLimit = "WEBHOOK_SECRET_VERSION_LIMIT"
	codeWebhookSecretLastVersion  = "WEBHOOK_SECRET_LAST_VERSION"
)

type webhookSecretController struct {
	secrets *webhookSecretService
	core    func() modkit.CoreAPI
	log     *slog.Logger
}

// webhookSecretCreateRequest is a new secret: id is its handle, name its display name, and orgRef the
// org.ref_id its portal sends when that isn't this organization's id.
type webhookSecretCreateRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	OrgRef string `json:"orgRef,omitempty"`
}

type webhookSecretVersionResponse struct {
	Version      int       `json:"version"`
	MaskedSecret string    `json:"maskedSecret"`
	Hash         string    `json:"hash"`
	CreatedBy    string    `json:"createdBy,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type webhookSecretResponse struct {
	ID        string                         `json:"id"`
	Name      string                         `json:"name"`
	OrgRef    string                         `json:"orgRef,omitempty"`
	CreatedBy string                         `json:"createdBy,omitempty"`
	CreatedAt time.Time                      `json:"createdAt"`
	UpdatedAt time.Time                      `json:"updatedAt"`
	Versions  []webhookSecretVersionResponse `json:"versions"`
	// Secret is the value, only in the answer to a create or a rotation.
	Secret string `json:"secret,omitempty"`
}

type webhookSecretListResponse struct {
	List       []webhookSecretResponse `json:"list"`
	Count      int                     `json:"count"`
	Pagination pagination              `json:"pagination"`
}

func webhookSecretResponseFor(s *webhookSecret, value string) webhookSecretResponse {
	resp := webhookSecretResponse{
		ID: s.Handle, Name: s.DisplayName, CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(),
		Versions: make([]webhookSecretVersionResponse, 0, len(s.Versions)), Secret: value,
	}
	if s.OrgRef != nil {
		resp.OrgRef = *s.OrgRef
	}
	if s.CreatedBy != nil {
		resp.CreatedBy = *s.CreatedBy
	}
	for _, v := range s.Versions {
		vr := webhookSecretVersionResponse{
			Version: v.Version, MaskedSecret: v.MaskedSecret, Hash: v.SecretHash, CreatedAt: v.CreatedAt.UTC(),
		}
		if v.CreatedBy != nil {
			vr.CreatedBy = *v.CreatedBy
		}
		resp.Versions = append(resp.Versions, vr)
	}
	return resp
}

func (c *webhookSecretController) createSecret(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := c.writer(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookSecretBodyBytes))
	if err != nil {
		c.writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "The request body is too large")
		return
	}
	var req webhookSecretCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid request body")
		return
	}
	created, value, err := c.secrets.create(r.Context(), org, actor,
		webhookSecretInput{Handle: req.ID, DisplayName: req.Name, OrgRef: req.OrgRef})
	if err != nil {
		c.fail(w, r, err)
		return
	}
	auditSecretVersion(r.Context(), created.Handle, created.DisplayName, newestVersion(created))
	w.Header().Set("Cache-Control", "no-store")
	c.writeJSON(w, http.StatusCreated, webhookSecretResponseFor(created, value))
}

func (c *webhookSecretController) listSecrets(w http.ResponseWriter, r *http.Request) {
	org, _, ok := c.caller(w, r)
	if !ok {
		return
	}
	rows, err := c.secrets.list(r.Context(), org)
	if err != nil {
		c.fail(w, r, err)
		return
	}
	resp := webhookSecretListResponse{List: make([]webhookSecretResponse, 0, len(rows)), Count: len(rows)}
	for i := range rows {
		resp.List = append(resp.List, webhookSecretResponseFor(&rows[i], ""))
	}
	resp.Pagination = pagination{Total: len(rows), Offset: 0, Limit: len(rows)}
	c.writeJSON(w, http.StatusOK, resp)
}

func (c *webhookSecretController) getSecret(w http.ResponseWriter, r *http.Request) {
	org, _, ok := c.caller(w, r)
	if !ok {
		return
	}
	s, err := c.secrets.get(r.Context(), org, r.PathValue("webhookSecretId"))
	if err != nil {
		c.fail(w, r, err)
		return
	}
	c.writeJSON(w, http.StatusOK, webhookSecretResponseFor(s, ""))
}

func (c *webhookSecretController) rotateSecret(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := c.writer(w, r)
	if !ok {
		return
	}
	rotated, value, err := c.secrets.rotate(r.Context(), org, actor, r.PathValue("webhookSecretId"))
	if err != nil {
		c.fail(w, r, err)
		return
	}
	auditSecretVersion(r.Context(), rotated.Handle, rotated.DisplayName, newestVersion(rotated))
	w.Header().Set("Cache-Control", "no-store")
	c.writeJSON(w, http.StatusOK, webhookSecretResponseFor(rotated, value))
}

func (c *webhookSecretController) deleteSecret(w http.ResponseWriter, r *http.Request) {
	org, _, ok := c.writer(w, r)
	if !ok {
		return
	}
	handle := r.PathValue("webhookSecretId")
	if err := c.secrets.delete(r.Context(), org, handle); err != nil {
		c.fail(w, r, err)
		return
	}
	modkit.AuditResource(r.Context(), auditResourceWebhookSecret, handle, handle)
	w.WriteHeader(http.StatusNoContent)
}

func (c *webhookSecretController) deleteSecretVersion(w http.ResponseWriter, r *http.Request) {
	org, _, ok := c.writer(w, r)
	if !ok {
		return
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || version < 1 {
		c.writeError(w, http.StatusBadRequest, codeValidationFailed, "version must be a positive integer")
		return
	}
	handle := r.PathValue("webhookSecretId")
	if err := c.secrets.deleteVersion(r.Context(), org, handle, version); err != nil {
		c.fail(w, r, err)
		return
	}
	auditSecretVersion(r.Context(), handle, handle, version)
	w.WriteHeader(http.StatusNoContent)
}

// auditSecretVersion names, in the request's audit record, the webhook secret by its handle and the version made or
// removed. Never its value or hash.
func auditSecretVersion(ctx context.Context, handle, name string, version int) {
	modkit.AuditResource(ctx, auditResourceWebhookSecret, handle, name)
	if version > 0 {
		modkit.AuditDetail(ctx, auditDetailVersion, version)
	}
}

// newestVersion is a secret's newest version number: the one create and rotate just made.
func newestVersion(s *webhookSecret) int {
	newest := 0
	for _, v := range s.Versions {
		newest = max(newest, v.Version)
	}
	return newest
}

// fail answers a service error: the caller's own mistakes with what they were, anything else as a 500 that is
// logged.
func (c *webhookSecretController) fail(w http.ResponseWriter, r *http.Request, err error) {
	var v *validationError
	switch {
	case errors.As(err, &v):
		c.writeError(w, http.StatusBadRequest, codeValidationFailed, v.Error())
	case errors.Is(err, errWebhookSecretNotFound), errors.Is(err, errWebhookSecretVersionAbsent):
		c.writeError(w, http.StatusNotFound, codeWebhookSecretNotFound, "The webhook secret or version was not found")
	case errors.Is(err, errWebhookSecretExists):
		c.writeError(w, http.StatusConflict, codeWebhookSecretExists, "A webhook secret with this id exists")
	case errors.Is(err, errWebhookSecretVersionsFull):
		c.writeError(w, http.StatusConflict, codeWebhookSecretVersionLimit,
			"The webhook secret keeps no more versions: delete an old version before rotating again")
	case errors.Is(err, errWebhookSecretLastVersion):
		c.writeError(w, http.StatusConflict, codeWebhookSecretLastVersion,
			"The last version of a webhook secret can't be deleted on its own: delete the secret instead")
	default:
		c.log.ErrorContext(r.Context(), "webhook secret request failed", "error", err)
		c.writeError(w, http.StatusInternalServerError, codeInternal, "The request could not be processed")
	}
}

// writer is caller, for a change: it also makes sure the core has the organization.
func (c *webhookSecretController) writer(w http.ResponseWriter, r *http.Request) (org, actor string, ok bool) {
	if org, actor, ok = c.caller(w, r); !ok {
		return "", "", false
	}
	if _, err := c.core().EnsureOrg(r.Context(), org); err != nil {
		c.log.ErrorContext(r.Context(), "could not read the caller's organization", "orgId", org, "error", err)
		c.writeError(w, http.StatusInternalServerError, codeInternal, "Failed to read the organization")
		return "", "", false
	}
	return org, actor, true
}

// caller returns the organization and the acting user of a request, as the core signed the caller in.
func (c *webhookSecretController) caller(w http.ResponseWriter, r *http.Request) (org, actor string, ok bool) {
	org = modkit.OrgID(r.Context())
	if org == "" {
		c.writeError(w, http.StatusUnauthorized, codeUnauthorized, "Invalid or expired credentials.")
		return "", "", false
	}
	return org, modkit.PrincipalFrom(r.Context()).User, true
}

func (c *webhookSecretController) writeError(w http.ResponseWriter, status int, code, message string) {
	c.writeJSON(w, status, errorResponse{Status: "error", Code: code, Message: message})
}

func (c *webhookSecretController) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
