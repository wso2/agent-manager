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
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Path parameters of the Shadow Agent Discovery routes.
const (
	PathParamShadowConnectionID = "connectionId"
	PathParamShadowScanID       = "scanId"
	PathParamShadowFindingID    = "findingId"
)

// ShadowAgentController serves Shadow Agent Discovery: platform connections,
// scans, and the unregistered agents they found.
type ShadowAgentController interface {
	ListProviders(w http.ResponseWriter, r *http.Request)
	GetSummary(w http.ResponseWriter, r *http.Request)
	ListConnections(w http.ResponseWriter, r *http.Request)
	CreateConnection(w http.ResponseWriter, r *http.Request)
	GetConnection(w http.ResponseWriter, r *http.Request)
	UpdateConnection(w http.ResponseWriter, r *http.Request)
	DeleteConnection(w http.ResponseWriter, r *http.Request)
	TestConnection(w http.ResponseWriter, r *http.Request)
	ListScans(w http.ResponseWriter, r *http.Request)
	CreateScan(w http.ResponseWriter, r *http.Request)
	GetScan(w http.ResponseWriter, r *http.Request)
	ClearScans(w http.ResponseWriter, r *http.Request)
	ListFindings(w http.ResponseWriter, r *http.Request)
	GetFinding(w http.ResponseWriter, r *http.Request)
	UpdateFinding(w http.ResponseWriter, r *http.Request)
	DeleteFinding(w http.ResponseWriter, r *http.Request)
	DeleteFindings(w http.ResponseWriter, r *http.Request)
}

type shadowAgentController struct {
	svc services.ShadowAgentService
}

// NewShadowAgentController creates a ShadowAgentController.
func NewShadowAgentController(svc services.ShadowAgentService) ShadowAgentController {
	return &shadowAgentController{svc: svc}
}

func (c *shadowAgentController) ListProviders(w http.ResponseWriter, _ *http.Request) {
	providers := sa.Providers()
	resp := spec.ShadowProviderListResponse{
		Providers: make([]spec.ShadowProviderSpec, 0, len(providers)),
		Signals:   []spec.ShadowSignalDefinition{},
	}
	for _, p := range providers {
		ps := spec.ShadowProviderSpec{
			Provider: p.Provider, DisplayName: p.DisplayName, Description: p.Description,
			Endpoints: []spec.ShadowEndpointSpec{}, Fields: []spec.ShadowFieldSpec{},
			Permissions: []spec.ShadowPermissionSpec{}, Docs: append([]string{}, p.Docs...),
			Capabilities: []spec.ShadowCapabilitySpec{},
		}
		if p.SourceHost {
			src := true
			ps.SourceHost = &src
		}
		if choice, ok := sa.CredentialChoiceOf(p.Provider); ok {
			ps.CredentialChoice = &spec.ShadowCredentialChoice{Fields: append([]string{}, choice.Fields...), Message: choice.Message}
		}
		for _, c := range sa.Capabilities(p.Provider) {
			ps.Capabilities = append(ps.Capabilities, spec.ShadowCapabilitySpec{
				Key: c.Key, Title: c.Title, Description: c.Description,
				Signals: append([]string{}, c.Signals...), Needs: append([]string{}, c.Needs...),
			})
		}
		for _, e := range p.Endpoints {
			es := spec.ShadowEndpointSpec{
				Key: e.Key, Label: e.Label, Default: e.Default,
				Placeholders: append([]string{}, e.Placeholders...), Option: optionalString(e.Option),
				FillFrom: optionalString(e.FillFrom),
			}
			if len(e.FillExclude) > 0 {
				es.FillExclude = append([]string{}, e.FillExclude...)
			}
			if len(e.FillValues) > 0 {
				es.FillValues = append([]string{}, e.FillValues...)
			}
			if e.Advanced {
				adv := true
				es.Advanced = &adv
			}
			ps.Endpoints = append(ps.Endpoints, es)
		}
		for _, f := range p.Fields {
			fs := spec.ShadowFieldSpec{
				Key: f.Key, Label: f.Label, Kind: f.Kind, Required: f.Required,
				Placeholder: optionalString(f.Placeholder), Help: optionalString(f.Help), Option: optionalString(f.Option),
			}
			if f.Recommended {
				rec := true
				fs.Recommended = &rec
			}
			fs.Section = optionalString(f.Section)
			if f.Advanced {
				adv := true
				fs.Advanced = &adv
			}
			if f.Kind == sa.FieldCheckbox {
				b := f.DefaultBool
				fs.DefaultBool = &b
			}
			ps.Fields = append(ps.Fields, fs)
		}
		for _, perm := range p.Permissions {
			pp := spec.ShadowPermissionSpec{
				Title: perm.Title, Items: append([]string{}, perm.Items...),
				Policy: optionalString(perm.Policy), Note: optionalString(perm.Note),
				Summary: optionalString(perm.Summary),
			}
			if len(perm.Reads) > 0 {
				pp.Reads = append([]string{}, perm.Reads...)
			}
			ps.Permissions = append(ps.Permissions, pp)
		}
		resp.Providers = append(resp.Providers, ps)
	}
	for _, d := range sa.Defs() {
		resp.Signals = append(resp.Signals, spec.ShadowSignalDefinition{
			Code: d.Code, Provider: d.Provider, Name: d.Name, Condition: d.Condition,
		})
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *shadowAgentController) GetSummary(w http.ResponseWriter, r *http.Request) {
	sum, latest, err := c.svc.Summary(r.Context(), middleware.OUIDFromRequest(r))
	if err != nil {
		writeShadowError(w, r, err, "Failed to summarize shadow agents")
		return
	}
	resp := spec.ShadowAgentSummary{
		TotalFindings: int32(sum.Total), //nolint:gosec // bounded by row counts
		ByProvider:    toInt32Map(sum.ByProvider),
		ByStatus:      toInt32Map(sum.ByStatus),
		Connections:   int32(sum.Connections), //nolint:gosec // bounded by the per-org limit
	}
	if latest != nil {
		s := toShadowScanResponse(latest)
		resp.LatestScan = &s
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// ---- connections ----

func shadowConnectionID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(PathParamShadowConnectionID))
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Connection not found")
		return uuid.Nil, false
	}
	return id, true
}

func (c *shadowAgentController) ListConnections(w http.ResponseWriter, r *http.Request) {
	list, err := c.svc.ListConnections(r.Context(), middleware.OUIDFromRequest(r))
	if err != nil {
		writeShadowError(w, r, err, "Failed to list connections")
		return
	}
	resp := spec.ShadowConnectionListResponse{Connections: make([]spec.ShadowConnectionResponse, 0, len(list))}
	for i := range list {
		resp.Connections = append(resp.Connections, toShadowConnectionResponse(&list[i]))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func decodeShadowConnectionRequest(w http.ResponseWriter, r *http.Request) (services.ShadowConnectionInput, bool) {
	var req spec.ShadowConnectionRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return services.ShadowConnectionInput{}, false
	}
	in := services.ShadowConnectionInput{
		Name: req.Name, Provider: req.Provider, Settings: req.Settings, RemoveSecrets: req.RemoveSecrets,
	}
	if req.Endpoints != nil {
		in.Endpoints = *req.Endpoints
	}
	if in.Settings == nil {
		in.Settings = map[string]interface{}{}
	}
	return in, true
}

func (c *shadowAgentController) CreateConnection(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeShadowConnectionRequest(w, r)
	if !ok {
		return
	}
	v, err := c.svc.CreateConnection(r.Context(), middleware.OUIDFromRequest(r), in)
	if err != nil {
		writeShadowError(w, r, err, "Failed to add the connection")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, toShadowConnectionResponse(v))
}

func (c *shadowAgentController) GetConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := shadowConnectionID(w, r)
	if !ok {
		return
	}
	v, err := c.svc.GetConnection(r.Context(), middleware.OUIDFromRequest(r), id)
	if err != nil {
		writeShadowError(w, r, err, "Failed to get the connection")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toShadowConnectionResponse(v))
}

func (c *shadowAgentController) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := shadowConnectionID(w, r)
	if !ok {
		return
	}
	in, ok := decodeShadowConnectionRequest(w, r)
	if !ok {
		return
	}
	v, err := c.svc.UpdateConnection(r.Context(), middleware.OUIDFromRequest(r), id, in)
	if err != nil {
		writeShadowError(w, r, err, "Failed to update the connection")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toShadowConnectionResponse(v))
}

func (c *shadowAgentController) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := shadowConnectionID(w, r)
	if !ok {
		return
	}
	if err := c.svc.DeleteConnection(r.Context(), middleware.OUIDFromRequest(r), id); err != nil {
		writeShadowError(w, r, err, "Failed to delete the connection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *shadowAgentController) TestConnection(w http.ResponseWriter, r *http.Request) {
	id, ok := shadowConnectionID(w, r)
	if !ok {
		return
	}
	res, err := c.svc.TestConnection(r.Context(), middleware.OUIDFromRequest(r), id)
	if err != nil {
		writeShadowError(w, r, err, "Failed to test the connection")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toShadowConnectionTest(*res))
}

// ---- scans ----

func (c *shadowAgentController) ListScans(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = n
	}
	list, err := c.svc.ListScans(r.Context(), middleware.OUIDFromRequest(r), limit)
	if err != nil {
		writeShadowError(w, r, err, "Failed to list scans")
		return
	}
	resp := spec.ShadowScanListResponse{Scans: make([]spec.ShadowScanResponse, 0, len(list))}
	for i := range list {
		resp.Scans = append(resp.Scans, toShadowScanResponse(&list[i]))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *shadowAgentController) CreateScan(w http.ResponseWriter, r *http.Request) {
	var req spec.ShadowScanRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}
	ids := make([]uuid.UUID, 0, len(req.ConnectionIds))
	for _, s := range req.ConnectionIds {
		id, err := uuid.Parse(s)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "connectionIds must be connection IDs")
			return
		}
		ids = append(ids, id)
	}
	scan, err := c.svc.CreateScan(r.Context(), middleware.OUIDFromRequest(r), ids)
	if err != nil {
		writeShadowError(w, r, err, "Failed to start the scan")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusAccepted, toShadowScanResponse(scan))
}

func (c *shadowAgentController) GetScan(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue(PathParamShadowScanID))
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Scan not found")
		return
	}
	scan, err := c.svc.GetScan(r.Context(), middleware.OUIDFromRequest(r), id)
	if err != nil {
		writeShadowError(w, r, err, "Failed to get the scan")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toShadowScanResponse(scan))
}

// ClearScans deletes the org's finished scans. A queued or running scan is
// kept, and findings are not changed.
func (c *shadowAgentController) ClearScans(w http.ResponseWriter, r *http.Request) {
	n, err := c.svc.ClearScans(r.Context(), middleware.OUIDFromRequest(r))
	if err != nil {
		writeShadowError(w, r, err, "Failed to clear the scans")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, spec.ShadowScanClearResponse{Deleted: int32(n)}) //nolint:gosec // bounded by row counts
}

// ---- findings ----

func shadowFindingID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(PathParamShadowFindingID))
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
		return uuid.Nil, false
	}
	return id, true
}

func (c *shadowAgentController) ListFindings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repositories.ShadowFindingFilter{
		Provider: q.Get("provider"), Status: q.Get("status"), Search: q.Get("search"), Limit: 50,
	}
	if v := q.Get("connectionId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "connectionId must be a connection ID")
			return
		}
		f.ConnectionID = &id
	}
	if v := q.Get("scanId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "scanId must be a scan ID")
			return
		}
		f.ScanID = &id
	}
	for name, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || (name == "limit" && (n < 1 || n > 200)) {
				utils.WriteErrorResponse(w, http.StatusBadRequest, name+" is out of range")
				return
			}
			*dst = n
		}
	}
	rows, total, err := c.svc.ListFindings(r.Context(), middleware.OUIDFromRequest(r), f)
	if err != nil {
		writeShadowError(w, r, err, "Failed to list the agents")
		return
	}
	resp := spec.ShadowFindingListResponse{
		Findings: make([]spec.ShadowFindingSummary, 0, len(rows)),
		Total:    int32(total),    //nolint:gosec // bounded by row counts
		Limit:    int32(f.Limit),  //nolint:gosec // validated above
		Offset:   int32(f.Offset), //nolint:gosec // validated above
	}
	for i := range rows {
		resp.Findings = append(resp.Findings, toShadowFindingSummary(&rows[i]))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *shadowAgentController) GetFinding(w http.ResponseWriter, r *http.Request) {
	id, ok := shadowFindingID(w, r)
	if !ok {
		return
	}
	v, err := c.svc.GetFinding(r.Context(), middleware.OUIDFromRequest(r), id)
	if err != nil {
		writeShadowError(w, r, err, "Failed to get the agent")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toShadowFindingDetail(v))
}

func (c *shadowAgentController) UpdateFinding(w http.ResponseWriter, r *http.Request) {
	id, ok := shadowFindingID(w, r)
	if !ok {
		return
	}
	var req spec.ShadowFindingUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	v, err := c.svc.UpdateFindingStatus(r.Context(), middleware.OUIDFromRequest(r), id, req.Status)
	if err != nil {
		writeShadowError(w, r, err, "Failed to update the agent")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toShadowFindingDetail(v))
}

// DeleteFinding deletes an agent on every connection that found it.
func (c *shadowAgentController) DeleteFinding(w http.ResponseWriter, r *http.Request) {
	id, ok := shadowFindingID(w, r)
	if !ok {
		return
	}
	if err := c.svc.DeleteFinding(r.Context(), middleware.OUIDFromRequest(r), id); err != nil {
		writeShadowError(w, r, err, "Failed to delete the agent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteFindings deletes every agent of the org. Platforms and scans are kept.
func (c *shadowAgentController) DeleteFindings(w http.ResponseWriter, r *http.Request) {
	n, err := c.svc.DeleteFindings(r.Context(), middleware.OUIDFromRequest(r))
	if err != nil {
		writeShadowError(w, r, err, "Failed to delete the agents")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, spec.ShadowFindingClearResponse{Deleted: int32(n)}) //nolint:gosec // bounded by row counts
}

// ---- errors ----

func writeShadowError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, utils.ErrShadowConnectionNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Connection not found")
	case errors.Is(err, utils.ErrShadowScanNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Scan not found")
	case errors.Is(err, utils.ErrShadowFindingNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
	case errors.Is(err, utils.ErrShadowConnectionNameTaken):
		utils.WriteErrorResponse(w, http.StatusConflict, "A connection with this name already exists")
	case errors.Is(err, utils.ErrShadowScanInProgress):
		utils.WriteErrorResponse(w, http.StatusConflict, "A scan is already queued or running")
	case errors.Is(err, utils.ErrShadowNoConnections):
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Add a platform before running a scan")
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

// ---- mapping ----

func toInt32Map(m map[string]int) map[string]int32 {
	out := make(map[string]int32, len(m))
	for k, v := range m {
		out[k] = int32(v) //nolint:gosec // bounded by row counts
	}
	return out
}

func toShadowConnectionResponse(v *services.ShadowConnectionView) spec.ShadowConnectionResponse {
	c := v.Connection
	resp := spec.ShadowConnectionResponse{
		Id: c.ID.String(), Name: c.Name, Provider: c.Provider,
		Endpoints: c.Config.Endpoints, Settings: v.Settings, SecretHints: c.SecretHints,
		EndpointWarnings: v.EndpointWarnings, LastTestedAt: c.LastTestedAt,
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if resp.Endpoints == nil {
		resp.Endpoints = map[string]string{}
	}
	if resp.EndpointWarnings == nil {
		resp.EndpointWarnings = []string{}
	}
	if c.LastTestResult != nil {
		t := toShadowConnectionTest(*c.LastTestResult)
		resp.LastTest = &t
	}
	return resp
}

func toShadowConnectionTest(t models.ShadowConnectionTestInfo) spec.ShadowConnectionTest {
	out := spec.ShadowConnectionTest{Ok: t.OK, Checks: make([]spec.ShadowConnectionCheck, 0, len(t.Checks))}
	for _, ch := range t.Checks {
		out.Checks = append(out.Checks, spec.ShadowConnectionCheck{
			Name: ch.Name, Outcome: ch.Outcome, Message: optionalString(ch.Message), Unlocks: ch.Unlocks,
		})
	}
	return out
}

func toShadowScanResponse(s *models.ShadowScan) spec.ShadowScanResponse {
	resp := spec.ShadowScanResponse{
		Id: s.ID.String(), Status: s.Status,
		ConnectionIds: s.ConnectionIDs, Results: make([]spec.ShadowScanConnectionResult, 0, len(s.Results)),
		FindingsCount: int32(s.FindingsCount), //nolint:gosec // bounded by row counts
		RequestedBy:   s.RequestedBy, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, CreatedAt: s.CreatedAt,
	}
	if resp.ConnectionIds == nil {
		resp.ConnectionIds = []string{}
	}
	for _, r := range s.Results {
		cr := spec.ShadowScanConnectionResult{
			ConnectionId: r.ConnectionID, ConnectionName: r.ConnectionName, Provider: r.Provider,
			Outcome: r.Outcome, Message: optionalString(r.Message),
			FindingsCount: int32(r.FindingsCount), //nolint:gosec // bounded by row counts
			Calls:         make([]spec.ShadowScanCallResult, 0, len(r.Calls)),
		}
		for _, call := range r.Calls {
			cr.Calls = append(cr.Calls, spec.ShadowScanCallResult{
				Call: call.Call, Outcome: call.Outcome, Message: optionalString(call.Message),
				Items: int32(call.Items), //nolint:gosec // bounded by provider page counts
			})
		}
		resp.Results = append(resp.Results, cr)
	}
	return resp
}

func toShadowOwnerHint(h models.ShadowOwnerHint) spec.ShadowOwnerHint {
	return spec.ShadowOwnerHint{
		Name: optionalString(h.Name), Email: optionalString(h.Email), Username: optionalString(h.Username),
		Type: optionalString(h.Type), Source: optionalString(h.Source), CreatedAt: h.CreatedAt,
	}
}

func toShadowFindingSummary(row *repositories.ShadowFindingRow) spec.ShadowFindingSummary {
	f := row.ShadowFinding
	foundBy := make([]spec.ShadowFindingSource, 0, len(row.FoundBy))
	for _, src := range row.FoundBy {
		foundBy = append(foundBy, spec.ShadowFindingSource{
			FindingId: src.FindingID, ConnectionId: src.ConnectionID, ConnectionName: src.ConnectionName, Present: src.Present,
		})
	}
	return spec.ShadowFindingSummary{
		Id: f.ID.String(), ConnectionId: f.ConnectionID.String(), ConnectionName: row.ConnectionName,
		Provider: f.Provider, Kind: f.Kind, ExternalId: f.ExternalID, DisplayName: f.DisplayName,
		Scope: f.Scope, State: f.State, Status: f.Status,
		OwnerHint: toShadowOwnerHint(f.OwnerHint), ProviderCreatedAt: f.ProviderCreatedAt,
		FirstSeenAt: f.FirstSeenAt, LastSeenAt: f.LastSeenAt, Present: f.Present, FoundBy: foundBy,
	}
}

func toShadowFindingDetail(v *services.ShadowFindingView) spec.ShadowFindingDetail {
	sum := toShadowFindingSummary(&v.Row)
	f := v.Row.ShadowFinding
	out := spec.ShadowFindingDetail{
		Id: sum.Id, ConnectionId: sum.ConnectionId, ConnectionName: sum.ConnectionName, Provider: sum.Provider,
		Kind: sum.Kind, ExternalId: sum.ExternalId, DisplayName: sum.DisplayName, Scope: sum.Scope, State: sum.State,
		Status: sum.Status, OwnerHint: sum.OwnerHint, ProviderCreatedAt: sum.ProviderCreatedAt,
		FirstSeenAt: sum.FirstSeenAt, LastSeenAt: sum.LastSeenAt, Present: sum.Present, FoundBy: sum.FoundBy,
		Attributes: sa.AllowedAttributes(f.Attributes), Related: make([]spec.ShadowFindingRelation, 0, len(v.Related)),
		Signals: make([]spec.ShadowFindingSignal, 0, len(v.Signals)),
	}
	for _, rel := range v.Related {
		out.Related = append(out.Related, spec.ShadowFindingRelation{
			FindingId: rel.FindingID, Relation: rel.Relation, Reason: optionalString(rel.Reason),
			DisplayName: optionalString(rel.DisplayName), Provider: optionalString(rel.Provider),
		})
	}
	for _, sig := range v.Signals {
		// The name and condition come from the catalog, so they always say
		// what this version of Agent Manager checks.
		d, ok := sa.Def(sig.Code)
		if !ok {
			continue
		}
		observed := sig.Observed
		if observed == nil {
			observed = map[string]any{}
		}
		out.Signals = append(out.Signals, spec.ShadowFindingSignal{
			Code: sig.Code, Name: d.Name, Source: sig.Source, Condition: d.Condition, Observed: observed, DetectedAt: sig.DetectedAt,
		})
	}
	return out
}
