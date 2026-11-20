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

package modhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/internal/reqorigin"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// Service workloads heartbeat to the core every few seconds with their
// credential. Each heartbeat records the instance, checks the protocol the
// instance speaks, and returns a fresh workload token together with the
// module's endpoints for that workload, so a workload is configured with
// only the heartbeat URL and its credential. While the module is inactive
// heartbeats and internal routes answer 401 module_inactive, and workloads
// stand by.

const (
	// HeartbeatPath is where workloads heartbeat, below /api/internal/v1.
	HeartbeatPath = "/workloads/heartbeat"
	// LiveWindow is how recently an instance must have heartbeated to count
	// as connected.
	LiveWindow = 30 * time.Second
	// instanceRetention is how long an instance that stopped heartbeating
	// is remembered.
	instanceRetention = 24 * time.Hour
	// RestEndpoint names the module's internal root in a heartbeat response.
	RestEndpoint = "rest"
)

// Error codes of the workload plane, in the "error" field of a refusal.
const (
	ErrModuleInactive      = "module_inactive"
	ErrInvalidCredential   = "invalid_credential"
	ErrUnsupportedProtocol = "unsupported_protocol"
	ErrInvalidRequest      = "invalid_request"
	ErrInvalidToken        = "invalid_token"
	ErrInsufficientScope   = "insufficient_scope"
	ErrUnavailable         = "unavailable"
)

// InternalRoot is the path a module's internal routes are mounted under.
func InternalRoot(module string) string {
	return "/m/" + module + "/internal"
}

// WorkloadError is the body of a refused heartbeat or internal request.
type WorkloadError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	// Protocols lists what the module accepts, on unsupported_protocol.
	Protocols []string `json:"protocols,omitempty"`
}

func writeWorkloadError(w http.ResponseWriter, status int, body WorkloadError) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ---- manifest checks ----

var (
	workloadNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	protocolPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,62}$`)
)

// checkWorkloads checks a manifest's workload declarations; a module that
// declares them wrongly does not activate.
func checkWorkloads(mf modkit.Manifest) error {
	seen := map[string]bool{}
	for _, wl := range mf.Workloads {
		if !workloadNamePattern.MatchString(wl.Name) {
			return fmt.Errorf("workload name %q must be lowercase letters, digits and dashes, starting with a letter", wl.Name)
		}
		if seen[wl.Name] {
			return fmt.Errorf("workload %q is declared twice", wl.Name)
		}
		seen[wl.Name] = true
		if err := checkScopes(wl.Scopes); err != nil {
			return fmt.Errorf("workload %s: %w", wl.Name, err)
		}
		switch wl.Kind {
		case modkit.WorkloadService:
			if len(wl.Protocols) == 0 {
				return fmt.Errorf("workload %s: a service names the protocols it may speak", wl.Name)
			}
		case modkit.WorkloadJob:
			if len(wl.Protocols) > 0 || len(wl.Endpoints) > 0 || wl.Required {
				return fmt.Errorf("workload %s: a job does not heartbeat, so it takes no protocols, endpoints or required", wl.Name)
			}
		default:
			return fmt.Errorf("workload %s: kind %q is neither %s nor %s", wl.Name, wl.Kind, modkit.WorkloadService, modkit.WorkloadJob)
		}
		for i, p := range wl.Protocols {
			if !protocolPattern.MatchString(p) || slices.Contains(wl.Protocols[:i], p) {
				return fmt.Errorf("workload %s: protocol %q is not valid or listed twice", wl.Name, p)
			}
		}
		names := map[string]bool{}
		for _, ep := range wl.Endpoints {
			if !workloadNamePattern.MatchString(ep.Name) || ep.Name == RestEndpoint || names[ep.Name] {
				return fmt.Errorf("workload %s: endpoint name %q is not valid, reserved or listed twice", wl.Name, ep.Name)
			}
			names[ep.Name] = true
			if !strings.HasPrefix(ep.Path, "/") || strings.ContainsAny(ep.Path, " \t?#") {
				return fmt.Errorf("workload %s: endpoint %s path %q must start with / and hold no spaces, query or fragment", wl.Name, ep.Name, ep.Path)
			}
		}
	}
	return nil
}

// checkEndpoints checks that every endpoint a workload declares is an
// internal route the module registered (a GET route for a WebSocket).
func checkEndpoints(mf modkit.Manifest, internal []Route) error {
	for _, wl := range mf.Workloads {
		for _, ep := range wl.Endpoints {
			ok := slices.ContainsFunc(internal, func(r Route) bool {
				return r.Path == ep.Path && (!ep.WebSocket || r.Method == http.MethodGet)
			})
			if !ok {
				return fmt.Errorf("workload %s: endpoint %s (%s) is not a registered internal route", wl.Name, ep.Name, ep.Path)
			}
		}
	}
	return nil
}

// ---- heartbeat ----

// HeartbeatRequest is the body of a heartbeat.
type HeartbeatRequest struct {
	// Credential is the secret `amp module credential` printed.
	Credential string `json:"credential"`
	// Instance names this process ("gateway-0"); "default" when empty.
	Instance string `json:"instance"`
	// Protocol is the protocol the workload speaks ("amp-gateway/v1").
	Protocol string `json:"protocol"`
	// Version is the workload's own version, for display.
	Version string `json:"version"`
}

// HeartbeatResponse is the reply to an accepted heartbeat.
type HeartbeatResponse struct {
	Token     string    `json:"token"`
	TokenType string    `json:"tokenType"`
	ExpiresAt time.Time `json:"expiresAt"`
	Module    string    `json:"module"`
	Workload  string    `json:"workload"`
	// Endpoints are absolute URLs by name: "rest" is the module's internal
	// root, and the others are those the workload declares. Workloads build
	// every URL from them, re-read them on each heartbeat and reconnect
	// when one changes.
	Endpoints map[string]string `json:"endpoints"`
}

type heartbeatCredential struct {
	ID         string
	Module     string
	Workload   string
	OrgID      *string
	Subject    string
	SecretHash string
}

// workloadInstance is a workload_instances row.
type workloadInstance struct {
	CredentialID string    `gorm:"column:credential_id;primaryKey"`
	Instance     string    `gorm:"column:instance;primaryKey"`
	Module       string    `gorm:"column:module"`
	Workload     string    `gorm:"column:workload"`
	Protocol     string    `gorm:"column:protocol"`
	Version      string    `gorm:"column:version"`
	FirstSeen    time.Time `gorm:"column:first_seen"`
	LastSeen     time.Time `gorm:"column:last_seen"`
}

func (workloadInstance) TableName() string { return "workload_instances" }

func validLabel(s string, max int) bool {
	return len(s) <= max && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) })
}

// ServeHeartbeat serves POST /api/internal/v1/workloads/heartbeat. The
// credential is checked first, so nothing about a module is told to a caller
// without one.
func (h *Host) ServeHeartbeat() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req HeartbeatRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil ||
			req.Credential == "" || req.Protocol == "" ||
			!validLabel(req.Instance, 255) || !validLabel(req.Protocol, 63) || !validLabel(req.Version, 63) {
			writeWorkloadError(w, http.StatusBadRequest, WorkloadError{
				Error:   ErrInvalidRequest,
				Message: "credential and protocol are required; instance, protocol and version hold no spaces",
			})
			return
		}
		if req.Instance == "" {
			req.Instance = "default"
		}
		deny := func(status int, body WorkloadError, credentialID string) {
			audit.Record(r.Context(), audit.ActionWorkloadTokenExchange,
				audit.OutcomeOpt(audit.OutcomeDeny), audit.Status(status),
				audit.Detail("credentialId", credentialID), audit.Detail("reason", body.Error))
			writeWorkloadError(w, status, body)
		}
		cred, err := h.credential(r.Context(), req.Credential)
		if errors.Is(err, errInvalidCredential) {
			id, _, _ := splitCredential(req.Credential)
			deny(http.StatusUnauthorized, WorkloadError{Error: ErrInvalidCredential, Message: "unknown or revoked workload credential"}, id)
			return
		}
		if err != nil {
			h.log.Error("modhost:reading workload credential failed", "error", err)
			writeWorkloadError(w, http.StatusInternalServerError, WorkloadError{Error: ErrUnavailable, Message: "internal error"})
			return
		}
		if !h.activeModule(cred.Module) {
			deny(http.StatusUnauthorized, WorkloadError{
				Error:   ErrModuleInactive,
				Message: cred.Module + " is not active; stand by and retry",
			}, cred.ID)
			return
		}
		spec, err := h.serviceWorkload(cred.Module, cred.Workload)
		if err != nil {
			// The module no longer declares the workload.
			deny(http.StatusUnauthorized, WorkloadError{Error: ErrInvalidCredential, Message: "the credential's workload is not declared"}, cred.ID)
			return
		}
		if !slices.Contains(spec.Protocols, req.Protocol) {
			deny(http.StatusConflict, WorkloadError{
				Error: ErrUnsupportedProtocol,
				Message: fmt.Sprintf("%s %s does not accept protocol %q from %s; deploy a %s it accepts",
					cred.Module, h.manifests[cred.Module].Version, req.Protocol, cred.Workload, cred.Workload),
				Protocols: spec.Protocols,
			}, cred.ID)
			return
		}
		tokens := h.deps().Tokens
		if tokens == nil {
			writeWorkloadError(w, http.StatusServiceUnavailable, WorkloadError{Error: ErrUnavailable, Message: "workload tokens are not available"})
			return
		}
		wl := modkit.Workload{Module: cred.Module, Name: cred.Workload, Instance: req.Instance, Subject: cred.Subject, Scopes: spec.Scopes}
		if cred.OrgID != nil {
			wl.OrgID = *cred.OrgID
		}
		tok, exp, err := tokens.IssueWorkload(wl, DefaultWorkloadTokenTTL)
		if err != nil {
			h.log.Error("modhost:signing workload token failed", "error", err)
			writeWorkloadError(w, http.StatusInternalServerError, WorkloadError{Error: ErrUnavailable, Message: "internal error"})
			return
		}
		endpoints := h.endpoints(r, cred.Module, spec)
		if err := h.recordInstance(r.Context(), cred, wl, req, exp, endpoints); err != nil {
			h.log.Error("modhost:refusing a heartbeat that could not be recorded", "module", cred.Module, "error", err)
			writeWorkloadError(w, http.StatusServiceUnavailable, WorkloadError{Error: ErrUnavailable, Message: "the heartbeat could not be recorded; retry"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(HeartbeatResponse{
			Token: tok, TokenType: "Bearer", ExpiresAt: exp.UTC(),
			Module: cred.Module, Workload: cred.Workload, Endpoints: endpoints,
		})
	})
}

// errInvalidCredential is an unknown, revoked or malformed credential.
var errInvalidCredential = errors.New("invalid workload credential")

// credential returns the live credential a workload presented.
func (h *Host) credential(ctx context.Context, presented string) (*heartbeatCredential, error) {
	id, secret, ok := splitCredential(presented)
	if !ok {
		return nil, errInvalidCredential
	}
	var rows []heartbeatCredential
	if err := h.db.WithContext(ctx).Raw(`SELECT id, module, workload, org_id, subject, secret_hash
		FROM workload_credentials WHERE id = ? AND revoked_at IS NULL`, id).Scan(&rows).Error; err != nil {
		return nil, err
	}
	// Compared as hashes of a random secret: the comparison leaks nothing
	// about the secret itself.
	if len(rows) != 1 || rows[0].SecretHash != hashSecret(secret) {
		return nil, errInvalidCredential
	}
	return &rows[0], nil
}

// recordInstance refreshes the instance's row, or adds it. A new instance is
// audited before its first token is handed back, so every token can be traced
// to a recorded instance and credential; refreshing a known instance, every
// few seconds, is not audited.
func (h *Host) recordInstance(ctx context.Context, cred *heartbeatCredential, wl modkit.Workload, req HeartbeatRequest,
	exp time.Time, endpoints map[string]string,
) error {
	db := h.db.WithContext(ctx)
	now := dbx.Now()
	r := db.Exec(`UPDATE workload_instances SET last_seen = ?, protocol = ?, version = ?
		WHERE credential_id = ? AND instance = ?`, now, req.Protocol, req.Version, cred.ID, req.Instance)
	if r.Error != nil || r.RowsAffected > 0 {
		return r.Error
	}
	opts := []audit.Option{
		audit.Actor(audit.ActorService, "workload:"+cred.Module+":"+cred.Subject, ""),
		audit.Detail("module", cred.Module), audit.Detail("workload", cred.Workload),
		audit.Detail("instance", req.Instance), audit.Detail("credentialId", cred.ID),
		audit.Detail("expiresIn", int64(time.Until(exp).Seconds())),
	}
	if wl.OrgID != "" {
		opts = append(opts, audit.Org(wl.OrgID))
	}
	if err := audit.RecordSync(ctx, audit.ActionWorkloadTokenExchange, opts...); err != nil {
		return err
	}
	if _, err := dbx.InsertIfAbsent(db, &workloadInstance{
		CredentialID: cred.ID, Instance: req.Instance, Module: cred.Module, Workload: cred.Workload,
		Protocol: req.Protocol, Version: req.Version, FirstSeen: now, LastSeen: now,
	}); err != nil {
		return err
	}
	// Instances come and go with restarts; forget the ones long gone.
	if err := db.Exec(`DELETE FROM workload_instances WHERE credential_id = ? AND last_seen < ?`,
		cred.ID, now.Add(-instanceRetention)).Error; err != nil {
		return err
	}
	h.log.Info("modhost:workload instance connected", "module", cred.Module, "workload", cred.Workload,
		"instance", req.Instance, "subject", cred.Subject, "protocol", req.Protocol, "endpoints", endpoints)
	return nil
}

// endpoints are the absolute URLs a workload uses, on the internal server's
// public URL (Deps.InternalURL) or else the origin the heartbeat reached.
func (h *Host) endpoints(r *http.Request, module string, spec modkit.WorkloadSpec) map[string]string {
	base := strings.TrimRight(h.deps().InternalURL, "/")
	if base == "" {
		base = reqorigin.Of(r)
	}
	root := base + InternalRoot(module)
	out := map[string]string{}
	if h.current().internal[module] != nil {
		out[RestEndpoint] = root
	}
	for _, ep := range spec.Endpoints {
		u := root + ep.Path
		if ep.WebSocket {
			u = webSocketURL(u)
		}
		out[ep.Name] = u
	}
	return out
}

// webSocketURL gives an http(s) URL the matching ws(s) scheme.
func webSocketURL(u string) string {
	if rest, ok := strings.CutPrefix(u, "https://"); ok {
		return "wss://" + rest
	}
	if rest, ok := strings.CutPrefix(u, "http://"); ok {
		return "ws://" + rest
	}
	return u
}

// ---- health ----

// Module health states.
const (
	HealthOK       = "ok"
	HealthDegraded = "degraded"
	HealthInactive = "inactive"
)

// InstanceStatus is a connected workload instance.
type InstanceStatus struct {
	Instance string    `json:"instance"`
	Subject  string    `json:"subject"`
	Protocol string    `json:"protocol"`
	Version  string    `json:"version,omitempty"`
	LastSeen time.Time `json:"lastSeen"`
}

// WorkloadStatus is a declared workload and its connected instances.
type WorkloadStatus struct {
	Name      string              `json:"name"`
	Kind      modkit.WorkloadKind `json:"kind"`
	Required  bool                `json:"required,omitempty"`
	Instances []InstanceStatus    `json:"instances"`
}

// Health is a module's health: ok, degraded or inactive, why, its workloads
// and whatever the module reports itself (modkit.HealthReporter).
type Health struct {
	Module    string           `json:"module"`
	Status    string           `json:"status"`
	Reasons   []string         `json:"reasons,omitempty"`
	Workloads []WorkloadStatus `json:"workloads,omitempty"`
	Detail    map[string]any   `json:"detail,omitempty"`
}

// healthTimeout bounds a module's own health report.
const healthTimeout = 5 * time.Second

// Health reports a compiled-in module's health from the shared state, so
// every replica (and `amp module health`) gives the same answer: an active
// module is degraded when it failed to activate here or a required service
// workload has no instance connected (LiveWindow).
func (h *Host) Health(ctx context.Context, module string) (Health, error) {
	mf, ok := h.manifests[module]
	if !ok {
		return Health{}, &ResolveError{Msg: module + " is not compiled into this binary"}
	}
	h.mu.RLock()
	st, failed := stateOf(module, h.states), h.failed[module]
	h.mu.RUnlock()
	out := Health{Module: module, Status: HealthOK}
	for _, wl := range mf.Workloads {
		out.Workloads = append(out.Workloads, WorkloadStatus{Name: wl.Name, Kind: wl.Kind, Required: wl.Required, Instances: []InstanceStatus{}})
	}
	if st != StateActive {
		out.Status = HealthInactive
		out.Reasons = []string{module + " is " + st}
		return out, nil
	}
	var rows []struct {
		Workload, Instance, Subject, Protocol, Version string
		LastSeen                                       time.Time
	}
	if err := h.db.WithContext(ctx).Raw(`SELECT i.workload, i.instance, c.subject, i.protocol, i.version, i.last_seen
		FROM workload_instances i JOIN workload_credentials c ON c.id = i.credential_id
		WHERE i.module = ? AND i.last_seen > ? AND c.revoked_at IS NULL
		ORDER BY i.workload, i.instance, c.subject`, module, dbx.Now().Add(-LiveWindow)).Scan(&rows).Error; err != nil {
		return Health{}, fmt.Errorf("reading workload instances: %w", err)
	}
	for i := range out.Workloads {
		ws := &out.Workloads[i]
		for _, r := range rows {
			if r.Workload == ws.Name {
				ws.Instances = append(ws.Instances, InstanceStatus{
					Instance: r.Instance, Subject: r.Subject,
					Protocol: r.Protocol, Version: r.Version, LastSeen: r.LastSeen.UTC(),
				})
			}
		}
		if ws.Kind == modkit.WorkloadService && ws.Required && len(ws.Instances) == 0 {
			out.Status = HealthDegraded
			out.Reasons = append(out.Reasons, "no "+ws.Name+" instance is connected")
		}
	}
	if failed != nil {
		out.Status = HealthDegraded
		out.Reasons = append(out.Reasons, "failed to activate: "+failed.Error())
	}
	if hr, ok := h.mods[module].(modkit.HealthReporter); ok {
		out.Detail = h.reported(ctx, module, hr)
	}
	return out, nil
}

// reported runs a module's own health report within healthTimeout.
func (h *Host) reported(ctx context.Context, module string, hr modkit.HealthReporter) (detail map[string]any) {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			detail = map[string]any{"error": fmt.Sprintf("health report panicked: %v", p)}
		}
	}()
	rt := h.runtime(module, h.current())
	rt.pinned = true
	return hr.Health(ctx, rt)
}
