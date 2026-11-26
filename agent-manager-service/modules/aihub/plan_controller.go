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
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// The subscription plans API (GET/POST /subscription-plans, GET/PUT/DELETE
// /subscription-plans/{subscriptionPlanId}), served by the module at /api/v1/m/ai-hub/subscription-plans.
// A plan is a document, as Kubernetes resources are: apiVersion, kind, metadata (name is the plan's handle),
// spec (what the caller declares) and status (what the server observes, ignored on input). Bodies are JSON,
// or YAML (plan_document.go). Pricing (paid plans) is not supported.

const (
	maxPlanBodyBytes = 128 << 10
	defaultPageLimit = 20
	minPageLimit     = 1
	maxPageLimit     = 100
	basePath         = "/api/v1/m/" + Name + "/subscription-plans"
)

// Error codes.
const (
	codeValidationFailed = "VALIDATION_FAILED"
	codePayloadTooLarge  = "PAYLOAD_TOO_LARGE"
	codeUnauthorized     = "UNAUTHORIZED"
	codeInternal         = "INTERNAL_ERROR"
	codeUnavailable      = "SERVICE_UNAVAILABLE"
	codePlanNotFound     = "SUBSCRIPTION_PLAN_NOT_FOUND"
	codePlanExists       = "SUBSCRIPTION_PLAN_EXISTS"
	codePlanInUse        = "SUBSCRIPTION_PLAN_IN_USE"
)

type planController struct {
	plans *planService
	core  func() modkit.CoreAPI
	log   *slog.Logger
}

// limitRequest is one entry of the old "limits" field: a request-count window.
type limitRequest struct {
	LimitType        string `json:"limitType,omitempty"`
	TimeUnit         string `json:"timeUnit"`
	TimeAmount       int    `json:"timeAmount,omitempty"`
	LimitCount       int    `json:"limitCount"`
	LimitCountUnit   string `json:"limitCountUnit,omitempty"`
	StopOnQuotaReach *bool  `json:"stopOnQuotaReach,omitempty"`
}

// createRequest is a posted document's spec, with ID from metadata.name.
type createRequest struct {
	ID          string          `json:"-"`
	DisplayName string          `json:"displayName"`
	Description string          `json:"description,omitempty"`
	Conditions  json.RawMessage `json:"conditions,omitempty"`
	Limits      []limitRequest  `json:"limits,omitempty"`
	ExpiryTime  *string         `json:"expiryTime,omitempty"`
	Status      string          `json:"status,omitempty"`
	Pricing     json.RawMessage `json:"pricing,omitempty"`
}

// updateRequest is a PUT document's spec. Only displayName is required; an omitted field keeps the stored
// value.
type updateRequest struct {
	DisplayName string          `json:"displayName"`
	Description *string         `json:"description"`
	Conditions  json.RawMessage `json:"conditions"`
	Limits      *[]limitRequest `json:"limits"`
	ExpiryTime  json.RawMessage `json:"expiryTime"`
	Status      *string         `json:"status"`
	Pricing     json.RawMessage `json:"pricing"`
}

// createPlan handles POST /subscription-plans.
func (a *planController) createPlan(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := a.writer(w, r)
	if !ok {
		return
	}
	name, spec, ok := a.readDocument(w, r)
	if !ok {
		return
	}
	var req createRequest
	if err := json.Unmarshal(spec, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid spec")
		return
	}
	req.ID = name
	if len(req.Pricing) > 0 && string(req.Pricing) != "null" {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "spec.pricing is not supported: Agent Manager plans are free")
		return
	}
	if utf8.RuneCountInString(req.ID) > maxHandleChars {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "metadata.name must be at most 40 characters")
		return
	}
	if !handlePattern.MatchString(req.ID) {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed,
			"metadata.name must be lowercase letters, digits and hyphens, starting and ending with a letter or digit")
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "spec.displayName is required")
		return
	}
	if req.Status != "" && req.Status != StatusActive && req.Status != StatusInactive {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid status value; must be ACTIVE or INACTIVE")
		return
	}
	p := &plan{
		Handle: req.ID, Name: strings.TrimSpace(req.DisplayName), Description: strings.TrimSpace(req.Description),
		Status: req.Status, Conditions: []Condition{},
	}
	if len(req.Conditions) > 0 {
		// Sent at all, even as [], limits can't come with conditions.
		if req.Limits != nil {
			a.writeError(w, http.StatusBadRequest, codeValidationFailed, "send conditions or limits, not both")
			return
		}
		conditions, err := decodeConditions(req.Conditions)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		p.Conditions = conditions
	} else {
		requestLimit, msg := limitsToRequestCondition(req.Limits)
		if msg != "" {
			a.writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
			return
		}
		if requestLimit != nil {
			p.Conditions = []Condition{*requestLimit}
		}
	}
	if req.ExpiryTime != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiryTime)
		if err != nil {
			a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid expiryTime format; use RFC3339")
			return
		}
		p.ExpiryTime = &t
	}
	created, err := a.plans.create(r.Context(), org, actor, p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Location", basePath+"/"+created.Handle)
	a.writeDocument(w, r, http.StatusCreated, documentFor(created, nil))
}

// listPlans handles GET /subscription-plans?limit&offset&query&status.
func (a *planController) listPlans(w http.ResponseWriter, r *http.Request) {
	org, _, ok := a.caller(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	opts := listOptions{Limit: defaultPageLimit, Search: strings.TrimSpace(q.Get("query"))}
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Limit = min(max(n, minPageLimit), maxPageLimit)
		}
	}
	if v := strings.TrimSpace(q.Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			opts.Offset = n
		}
	}
	opts.Status = strings.ToUpper(strings.TrimSpace(q.Get("status")))
	if opts.Status != "" && opts.Status != StatusActive && opts.Status != StatusInactive {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "status must be ACTIVE or INACTIVE")
		return
	}
	plans, total, err := a.plans.list(r.Context(), org, opts)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	items := make([]planDocument, 0, len(plans))
	for _, p := range plans {
		items = append(items, documentFor(p, nil))
	}
	a.writeDocument(w, r, http.StatusOK, listResponse{
		List: items, Count: len(items),
		Pagination: pagination{Total: total, Offset: opts.Offset, Limit: opts.Limit},
	})
}

// getPlan handles GET /subscription-plans/{subscriptionPlanId}. Only this read carries usage, so a client can show
// a change of name, conditions or expiry as refused before it is tried.
func (a *planController) getPlan(w http.ResponseWriter, r *http.Request) {
	org, _, ok := a.caller(w, r)
	if !ok {
		return
	}
	p, err := a.plans.get(r.Context(), org, r.PathValue("subscriptionPlanId"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	use, err := a.plans.getUsage(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeDocument(w, r, http.StatusOK, documentFor(p, &use))
}

// updatePlan handles PUT /subscription-plans/{subscriptionPlanId}. metadata.name must be the plan's handle and
// spec.displayName is required. An omitted description, conditions, status or expiryTime keeps the stored
// value, so a document read with GET can be sent back as it is; "expiryTime": null clears the expiry.
func (a *planController) updatePlan(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := a.writer(w, r)
	if !ok {
		return
	}
	handle := r.PathValue("subscriptionPlanId")
	name, spec, ok := a.readDocument(w, r)
	if !ok {
		return
	}
	if name != handle {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed,
			"metadata.name must be the plan's handle, "+strconv.Quote(handle)+": it can't change")
		return
	}
	var req updateRequest
	if err := json.Unmarshal(spec, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid spec")
		return
	}
	if req.Pricing != nil {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "spec.pricing is not supported: Agent Manager plans are free")
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "spec.displayName is required")
		return
	}
	u := &planUpdate{Name: &req.DisplayName, Description: req.Description}
	if req.ExpiryTime != nil {
		if string(req.ExpiryTime) == "null" {
			u.ClearExpiryTime = true
		} else {
			// Decoded to a string first, so a JSON escape (\u005a for Z) reads as the character it stands for.
			var raw string
			t, err := time.Time{}, json.Unmarshal(req.ExpiryTime, &raw)
			if err == nil {
				t, err = time.Parse(time.RFC3339, raw)
			}
			if err != nil {
				a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid expiryTime format; use RFC3339")
				return
			}
			u.ExpiryTime = &t
		}
	}
	if req.Conditions != nil {
		if req.Limits != nil {
			a.writeError(w, http.StatusBadRequest, codeValidationFailed, "send conditions or limits, not both")
			return
		}
		conditions, err := decodeConditions(req.Conditions)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		u.Conditions = &conditions
	} else if req.Limits != nil {
		requestLimit, msg := limitsToRequestCondition(*req.Limits)
		if msg != "" {
			a.writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
			return
		}
		if requestLimit != nil {
			u.RequestLimit = requestLimit
		} else {
			u.ClearLimit = true
		}
	}
	if req.Status != nil {
		if *req.Status != StatusActive && *req.Status != StatusInactive {
			a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid status value; must be ACTIVE or INACTIVE")
			return
		}
		u.Status = req.Status
	}
	updated, err := a.plans.update(r.Context(), org, handle, actor, u)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeDocument(w, r, http.StatusOK, documentFor(updated, nil))
}

// deprecatePlan handles DELETE /subscription-plans/{subscriptionPlanId}: a plan is never removed, only deprecated
// (a soft delete). It stays, read only, for the subscriptions on it, and the response is the deprecated plan.
// Deprecating a deprecated plan changes nothing.
func (a *planController) deprecatePlan(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := a.caller(w, r)
	if !ok {
		return
	}
	p, err := a.plans.deprecate(r.Context(), org, r.PathValue("subscriptionPlanId"), actor)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeDocument(w, r, http.StatusOK, documentFor(p, nil))
}

// decodeConditions reads the "conditions" field. null or a non-array is refused.
func decodeConditions(raw json.RawMessage) ([]Condition, error) {
	if string(raw) == "null" {
		return nil, invalid("conditions must be an array; send [] for a plan with no conditions")
	}
	var conditions []Condition
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return nil, invalid("conditions must be an array of {id, type, params}")
	}
	return conditions, nil
}

// limitsToRequestCondition turns old "limits" into a request-count condition. Only the
// first limit is validated; later limits that are invalid or repeat a window length are skipped, and blocking over
// the limit is taken from the first limit.
func limitsToRequestCondition(limits []limitRequest) (*Condition, string) {
	if len(limits) == 0 {
		return nil, ""
	}
	if msg := normalizeLimit(&limits[0]); msg != "" {
		return nil, msg
	}
	windows := make([]Window, 0, len(limits))
	seen := map[string]bool{}
	for i := range limits {
		if i > 0 && normalizeLimit(&limits[i]) != "" {
			continue
		}
		key := windowKey(limits[i].TimeAmount, limits[i].TimeUnit)
		if seen[key] || len(windows) == maxConditionWindows {
			continue
		}
		seen[key] = true
		windows = append(windows, Window{Count: limits[i].LimitCount, TimeAmount: limits[i].TimeAmount, TimeUnit: limits[i].TimeUnit})
	}
	onExceed := OnExceedReject
	if limits[0].StopOnQuotaReach != nil && !*limits[0].StopOnQuotaReach {
		onExceed = OnExceedAllow
	}
	params, _ := json.Marshal(Limits{Limits: windows, OnExceed: onExceed})
	return &Condition{ID: ConditionIDRequests, Type: ConditionTypeRequestCount, Params: params}, ""
}

func normalizeLimit(l *limitRequest) string {
	if l.LimitType == "" {
		l.LimitType = "REQUEST_COUNT"
	} else if l.LimitType != "REQUEST_COUNT" {
		return "limitType: only REQUEST_COUNT is currently supported"
	}
	if !timeUnits[l.TimeUnit] {
		return "timeUnit is required and must be one of: MINUTE, HOUR, DAY, MONTH"
	}
	if l.LimitCount < 1 {
		return "limitCount must be at least 1"
	}
	if l.TimeAmount == 0 {
		l.TimeAmount = 1
	} else if l.TimeAmount < 0 {
		return "timeAmount must be at least 1"
	}
	return ""
}

// writer is caller for a write, which makes sure the organization exists first, as the core's own write
// routes do. It runs before the plan is read, so a failure answers 500 before anything changes.
func (a *planController) writer(w http.ResponseWriter, r *http.Request) (org, actor string, ok bool) {
	if org, actor, ok = a.caller(w, r); !ok {
		return "", "", false
	}
	if _, err := a.core().EnsureOrg(r.Context(), org); err != nil {
		a.log.ErrorContext(r.Context(), "could not read the caller's organization", "orgId", org, "error", err)
		a.writeError(w, http.StatusInternalServerError, codeInternal, "Failed to read the organization")
		return "", "", false
	}
	return org, actor, true
}

// caller returns the organization and the acting user of a request. The core has signed the caller in
// and set the organization from the token; the actor is the token's subject.
func (a *planController) caller(w http.ResponseWriter, r *http.Request) (org, actor string, ok bool) {
	org = modkit.OrgID(r.Context())
	if org == "" {
		a.writeError(w, http.StatusUnauthorized, codeUnauthorized, "Invalid or expired credentials.")
		return "", "", false
	}
	return org, modkit.PrincipalFrom(r.Context()).User, true
}

func (a *planController) readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPlanBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			a.writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "request body exceeds the maximum allowed size")
			return nil, false
		}
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, "Invalid request body")
		return nil, false
	}
	return body, true
}

// errorResponse is the error shape: { status, code, message, trackingId }.
type errorResponse struct {
	Status     string `json:"status"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	TrackingID string `json:"trackingId,omitempty"`
}

// fail maps a service error to a status and code. Unexpected errors are logged with a tracking id that the
// response carries.
func (a *planController) fail(w http.ResponseWriter, r *http.Request, err error) {
	var v *validationError
	var inUse *inUseError
	switch {
	case errors.As(err, &v):
		a.writeError(w, http.StatusBadRequest, codeValidationFailed, v.msg)
	case errors.As(err, &inUse):
		a.writeError(w, http.StatusConflict, codePlanInUse, inUse.msg)
	case errors.Is(err, errPlanNotFound):
		a.writeError(w, http.StatusNotFound, codePlanNotFound, "The specified subscription plan could not be found.")
	case errors.Is(err, errPlanExists):
		a.writeError(w, http.StatusConflict, codePlanExists, "A subscription plan with this name already exists.")
	case errors.Is(err, errPlanDeprecated):
		a.writeError(w, http.StatusConflict, codePlanDeprecated, "The subscription plan is deprecated and can't be changed.")
	default:
		tracking := uuid.NewString()
		a.log.ErrorContext(r.Context(), "subscription plan request failed", "method", r.Method, "path", r.URL.Path,
			"orgId", modkit.OrgID(r.Context()), "trackingId", tracking, "error", err)
		a.writeJSON(w, http.StatusInternalServerError, errorResponse{
			Status: "error", Code: codeInternal,
			Message: "An unexpected error occurred.", TrackingID: tracking,
		})
	}
}

func (a *planController) writeError(w http.ResponseWriter, status int, code, message string) {
	a.writeJSON(w, status, errorResponse{Status: "error", Code: code, Message: message})
}

func (a *planController) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
