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
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// ---- manifest checks ----

var (
	auditClasses = map[string]audit.ActionClass{
		"": audit.ClassConfig, "credential": audit.ClassCredential, "identity": audit.ClassIdentity,
		"deployment": audit.ClassDeployment, "config": audit.ClassConfig, "read": audit.ClassRead,
	}
	auditDetailKinds = []audit.FieldKind{
		audit.KindIdentifier, audit.KindName, audit.KindEmail, audit.KindNameList, audit.KindCount,
		audit.KindFlag, audit.KindEnum, audit.KindFingerprint, audit.KindURL,
	}
	detailKeyPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	auditSeverities  = map[string]audit.Severity{
		"info": audit.SeverityInfo, "notice": audit.SeverityNotice,
		"warning": audit.SeverityWarning, "critical": audit.SeverityCritical,
	}
	eventScopes       = []string{"", string(events.ScopeOrg), string(events.ScopeProject), string(events.ScopeAgent)}
	eventNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	actionNamePattern = permissionNamePattern
)

// checkManifest refuses audit actions and event types a module cannot
// declare. It runs when the module activates.
func checkManifest(mf modkit.Manifest) error {
	for _, a := range mf.AuditActions {
		if !actionNamePattern.MatchString(a.Name) {
			return fmt.Errorf("audit action %q must be <action> or <resource>.<action>, each lowercase letters, digits and dashes, starting with a letter", a.Name)
		}
		if _, ok := auditClasses[a.Class]; !ok {
			return fmt.Errorf("audit action %s: unknown class %q", a.Name, a.Class)
		}
		if _, ok := auditSeverities[a.Severity]; !ok && a.Severity != "" {
			return fmt.Errorf("audit action %s: unknown severity %q", a.Name, a.Severity)
		}
		for k, kind := range a.Details {
			if !detailKeyPattern.MatchString(k) {
				return fmt.Errorf("audit action %s: detail key %q must be letters and digits, starting with a lowercase letter", a.Name, k)
			}
			if !slices.Contains(auditDetailKinds, audit.FieldKind(kind)) {
				return fmt.Errorf("audit action %s: detail %s has unknown kind %q", a.Name, k, kind)
			}
		}
	}
	for _, e := range mf.Events {
		if !strings.HasPrefix(e.Name, mf.Name+".") || !eventNamePattern.MatchString(strings.ReplaceAll(e.Name, "-", "_")) {
			return fmt.Errorf("event type %q must be named %s.<event>", e.Name, mf.Name)
		}
		if !slices.Contains(eventScopes, e.Scope) {
			return fmt.Errorf("event type %s: unknown scope %q", e.Name, e.Scope)
		}
	}
	return nil
}

// auditAction resolves the audit action a module names on a route or tool:
// one of its permissions, or one of its declared audit actions.
func (h *Host) auditAction(module, name string) (string, error) {
	mf := h.manifests[module]
	for _, p := range mf.Permissions {
		if p.Name == name {
			return mf.PermissionName(name), nil
		}
	}
	for _, a := range mf.AuditActions {
		if a.Name == name {
			return mf.Name + ":" + name, nil
		}
	}
	return "", fmt.Errorf("audit action %q is neither declared in the manifest's auditActions nor one of its permissions", name)
}

// publishCatalogs makes the audit trail and the webhook catalog know what the
// modules active in v declare: how their audit actions are classified, and
// the webhook event types organizations can subscribe to.
func (h *Host) publishCatalogs(v *view) {
	actions := map[audit.Action]audit.ModuleAction{}
	var types []events.Type
	for _, name := range v.active {
		mf := h.manifests[name]
		for _, a := range mf.AuditActions {
			class := auditClasses[a.Class]
			severity, ok := auditSeverities[a.Severity]
			if !ok {
				severity = audit.SeverityInfo
				if class == audit.ClassCredential || class == audit.ClassIdentity {
					severity = audit.SeverityCritical
				}
			}
			full := mf.Name + ":" + a.Name
			if slices.ContainsFunc(mf.Permissions, func(p modkit.Permission) bool { return p.Name == a.Name }) {
				full = mf.PermissionName(a.Name)
			}
			details := make(map[string]audit.FieldKind, len(a.Details))
			for k, kind := range a.Details {
				details[k] = audit.FieldKind(kind)
			}
			actions[audit.Action(full)] = audit.ModuleAction{Class: class, Severity: severity, Details: details}
		}
		category := mf.DisplayName
		if category == "" {
			category = mf.Name
		}
		for _, e := range mf.Events {
			if e.Scope != "" {
				types = append(types, events.Type{Name: e.Name, Scope: events.Scope(e.Scope), Category: category, Description: e.Description})
			}
		}
	}
	audit.SetModuleActions(actions)
	events.SetModuleTypes(types)
}

// Routes lists the routes an active module registered on the API port, with
// Handle and then HandleAgent, in registration order; nil for a module that
// is not active.
func (h *Host) Routes(module string) []Route {
	return slices.Clone(h.current().routes[module])
}

// ---- MCP tools ----

type toolEntry struct {
	module string
	tool   modkit.MCPTool
	perm   rbac.Permission
	action string
	// auditRead records a read-only tool's calls too.
	auditRead bool
	fn        modkit.MCPToolHandler
}

// MCPTool is a tool an active module adds to the core's MCP server.
type MCPTool struct {
	Module     string
	Tool       modkit.MCPTool
	Permission rbac.Permission
	// Action is the audit action a call is recorded under; a read-only
	// tool's calls are recorded only when AuditRead is set.
	Action    string
	AuditRead bool
	Handler   modkit.MCPToolHandler
}

// MCPTools lists the tools of the active modules, in activation order. The
// MCP server asks on every request, so tools come and go with their modules.
func (h *Host) MCPTools() []MCPTool {
	v := h.current()
	out := make([]MCPTool, 0, len(v.tools))
	for _, t := range v.tools {
		out = append(out, MCPTool{Module: t.module, Tool: t.tool, Permission: t.perm, Action: t.action, AuditRead: t.auditRead, Handler: t.fn})
	}
	return out
}

// ---- secrets ----

// SecretStore is the core's secret store as the host uses it for modules: a
// secret is addressed by its organization, an owner kind and an owner id. A
// nil tx means the store's own connection. Values are stored encrypted.
type SecretStore interface {
	Put(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID, value string) error
	Get(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID) (string, bool, error)
	Delete(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID) error
}

// secretNamespace derives a secret's owner id from its module path, so the
// core's store, which keys secrets by owner, holds module secrets unchanged.
var secretNamespace = uuid.MustParse("6f0c1b0e-4c7a-5d3e-9b1a-6d6f64736563")

var secretPathPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`)

const maxSecretPath = 256

type moduleSecrets struct {
	h      *Host
	module string
	tx     *gorm.DB
}

// owner checks a module path and returns where the store keeps it: owner
// kind mod:<module>, which no core owner kind uses, and an id derived from
// the path.
func (s moduleSecrets) owner(path string) (string, uuid.UUID, error) {
	if len(path) > maxSecretPath || !secretPathPattern.MatchString(path) {
		return "", uuid.Nil, fmt.Errorf("modkit: secret path %q must be segments of letters, digits, '.', '_' and '-', joined by '/', at most %d characters", path, maxSecretPath)
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "." || seg == ".." {
			return "", uuid.Nil, fmt.Errorf("modkit: secret path %q holds a relative segment", path)
		}
	}
	return "mod:" + s.module, uuid.NewSHA1(secretNamespace, []byte(s.module+"/"+path)), nil
}

// conn picks the transaction to use: the one In gave, a hook's, or none.
// write is false for reads, which a read-only hook may make.
func (s moduleSecrets) conn(ctx context.Context, write bool) (*gorm.DB, error) {
	if s.tx != nil {
		return s.tx, nil
	}
	if tx, ok := hookTx(ctx); ok {
		if tx == nil && write {
			return nil, errReadOnlyHook
		}
		return tx, nil
	}
	//nolint:nilnil // no transaction: the store uses its own connection
	return nil, nil
}

func (s moduleSecrets) store() (SecretStore, error) {
	st := s.h.deps().Secrets
	if st == nil {
		return nil, modkit.ErrNotImplemented
	}
	return st, nil
}

func (s moduleSecrets) check(ctx context.Context, orgID, path string, write bool) (SecretStore, *gorm.DB, string, uuid.UUID, error) {
	if own := modkit.OrgID(ctx); own != "" && own != orgID {
		return nil, nil, "", uuid.Nil, modkit.ErrNotFound
	}
	if _, err := uuid.Parse(orgID); err != nil {
		return nil, nil, "", uuid.Nil, modkit.ErrNotFound
	}
	kind, owner, err := s.owner(path)
	if err != nil {
		return nil, nil, "", uuid.Nil, err
	}
	st, err := s.store()
	if err != nil {
		return nil, nil, "", uuid.Nil, err
	}
	tx, err := s.conn(ctx, write)
	return st, tx, kind, owner, err
}

func (s moduleSecrets) Put(ctx context.Context, orgID, path, value string) error {
	if value == "" {
		return errors.New("modkit: a secret value cannot be empty; use Delete")
	}
	st, tx, kind, owner, err := s.check(ctx, orgID, path, true)
	if err != nil {
		return err
	}
	return st.Put(ctx, tx, orgID, kind, owner, value)
}

func (s moduleSecrets) Get(ctx context.Context, orgID, path string) (string, error) {
	st, tx, kind, owner, err := s.check(ctx, orgID, path, false)
	if err != nil {
		return "", err
	}
	v, ok, err := st.Get(ctx, tx, orgID, kind, owner)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", modkit.ErrNotFound
	}
	return v, nil
}

func (s moduleSecrets) Delete(ctx context.Context, orgID, path string) error {
	st, tx, kind, owner, err := s.check(ctx, orgID, path, true)
	if errors.Is(err, modkit.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return st.Delete(ctx, tx, orgID, kind, owner)
}

func (s moduleSecrets) In(tx *gorm.DB) modkit.Secrets {
	s.tx = tx
	return s
}

// ---- core read API before the core attaches ----

func (unimplementedCore) GetProjectByName(context.Context, string, string) (modkit.Project, error) {
	return modkit.Project{}, modkit.ErrNotImplemented
}

func (unimplementedCore) GetAgentByName(context.Context, string, string, string) (modkit.Agent, error) {
	return modkit.Agent{}, modkit.ErrNotImplemented
}

func (unimplementedCore) GetEnvironmentByName(context.Context, string, string) (modkit.Environment, error) {
	return modkit.Environment{}, modkit.ErrNotImplemented
}

func (unimplementedCore) LockAgent(context.Context, *gorm.DB, string, string) (modkit.Agent, error) {
	return modkit.Agent{}, modkit.ErrNotImplemented
}

func (unimplementedCore) EnsureOrg(context.Context, string) (modkit.Org, error) {
	return modkit.Org{}, modkit.ErrNotImplemented
}
