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

// Package testmod holds two small modules that exercise every surface of the
// module contract from the tests: routes and permissions, audit and webhook
// route options, agent, environment and project hooks, events, a capability,
// workload tokens, background workers, MCP tools, secrets, the core's lookups
// and its own OpenAPI document. No binary links them; tests pass them to the
// module host explicitly, so they do not register themselves. They import
// nothing of the core but modkit, as a module may.
package testmod

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

const (
	// Name is the module that owns the tags table and the hooks.
	Name = "testmod"
	// ConsumerName is the module that only consumes Name's capability.
	ConsumerName = "testcap"
	// Echo is the capability Name provides.
	Echo = "testmod.echo/v1"
	// WidgetCreated is the event Name publishes.
	WidgetCreated = "testmod.widget_created"
	// WidgetArchived is the webhook event Name's archive route emits.
	WidgetArchived = "testmod.widget_archived"
	// Tool is the MCP tool Name adds.
	Tool = "testmod_count_tags"
	// LegacyPath is the fixed internal path Name serves with
	// HandleInternalAt, and LegacyKey the api-key it checks there.
	LegacyPath = "/api/internal/v1/testmod-legacy/ping"
	LegacyKey  = "testmod-legacy-key"
	// PingScope is the workload scope of Name's internal route.
	PingScope = "ping"
)

//go:embed migrations
var migrations embed.FS

//go:embed api/openapi.yaml
var openAPI []byte

// WorkerRuns counts the times Name's worker started; WorkerRunning is how
// many copies run now.
var (
	WorkerRuns    atomic.Int32
	WorkerRunning atomic.Int32
)

// Echoer is the contract of the Echo capability.
type Echoer interface {
	Echo(s string) string
}

type echoer struct{}

func (echoer) Echo(s string) string { return "echo:" + s }

// Module returns the module that owns tags, hooks and events.
func Module() modkit.Module { return module{} }

// Consumer returns the module that requires Module's capability.
func Consumer() modkit.Module { return consumer{} }

type module struct{}

func (module) Manifest() modkit.Manifest {
	return modkit.Manifest{
		Name:        Name,
		DisplayName: "Test Module",
		Version:     "1.0.0",
		CoreAPI:     modkit.CoreAPIVersion,
		Provides:    []string{Echo},
		Permissions: []modkit.Permission{
			{Name: "widget-read", DisplayName: "Read Widgets", Description: "Read widgets"},
			{Name: "widget-write", DisplayName: "Write Widgets", Description: "Write widgets"},
		},
		RoleGrants: map[string][]string{"Agent Manager Admin": {"widget-read", "widget-write"}},
		Events: []modkit.EventType{
			{Name: WidgetCreated, Description: "A widget was created."},
			{Name: WidgetArchived, Scope: "org", Description: "A widget was archived."},
		},
		AuditActions: []modkit.AuditAction{
			{Name: "widget-archive", Description: "Archive a widget"},
			{Name: "secret-write", Class: "credential", Description: "Store a secret"},
			{Name: "secret-read", Class: "credential", Description: "Read a secret"},
			{Name: "legacy-ping", Description: "Ping the fixed internal path"},
		},
	}
}

func (module) Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

func (module) UI() fs.FS { return nil }

// OpenAPI describes the routes Name mounts with Handle.
func (module) OpenAPI() []byte { return openAPI }

// extension is the agent extension block Name accepts.
type extension struct {
	Tag    string `json:"tag"`
	Reject bool   `json:"reject"`
	// Stall writes a row through the transaction, then outlives every
	// deadline and tries to write again.
	Stall bool `json:"stall"`
	// Announce publishes WidgetCreated from inside the apply hook.
	Announce bool `json:"announce"`
	// Lock locks the agent's row from the apply hook and checks it is the
	// agent being written.
	Lock bool `json:"lock"`
}

// StalledTag is the tag a stalling hook writes before it runs out of time.
const StalledTag = "stalled"

// AfterTimeout reports whether a stalling hook managed to write after its
// deadline; it must never.
var AfterTimeout atomic.Bool

// Environment hook behaviours for EnvironmentMode.
const (
	EnvironmentOK int32 = iota
	// EnvironmentFail writes a row tagged EnvironmentTag, then fails.
	EnvironmentFail
	// EnvironmentHang writes a row tagged EnvironmentTag, then runs a
	// statement that outlasts every deadline.
	EnvironmentHang
)

// EnvironmentMode sets what the environment apply hook does.
var EnvironmentMode atomic.Int32

// DeletePublishErr is what Publish returned when the delete validate hook
// tried it, for an agent tagged "publish-on-delete".
var DeletePublishErr atomic.Pointer[error]

// slowStatement runs one statement that takes far longer than any hook
// deadline, so the hook is stopped in the middle of it.
func slowStatement(db *gorm.DB) error {
	if db.Name() == "postgres" {
		return db.Exec(`SELECT pg_sleep(60)`).Error
	}
	return db.Exec(`WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c LIMIT 10000000000) SELECT count(*) FROM c`).Error
}

// EnvironmentTag marks the row a failing environment hook writes.
const EnvironmentTag = "env-hook"

// flaky counts deliveries of "flaky" widget events, which fail the first time.
var flaky sync.Map

func parse(raw json.RawMessage) (extension, error) {
	var ext extension
	if len(raw) == 0 {
		return ext, nil
	}
	if err := json.Unmarshal(raw, &ext); err != nil {
		return ext, modkit.Reject("extensions.%s is not valid: %v", Name, err)
	}
	return ext, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (module) Activate(_ context.Context, rt modkit.Runtime) error {
	rt.Provide(Echo, echoer{})

	rt.Hook(modkit.ResourceAgent, modkit.Validate, func(ctx context.Context, req modkit.HookRequest) error {
		if req.Action == modkit.ActionDelete {
			// Outside the transaction: read only.
			var tags []string
			if err := rt.DB().WithContext(ctx).Raw(`SELECT tag FROM mod_testmod_agent_tags WHERE agent_id = ?`, req.ID).
				Scan(&tags).Error; err != nil {
				return err
			}
			if len(tags) == 1 && tags[0] == "undeletable" {
				return &modkit.Error{Status: http.StatusConflict, Message: "the agent is tagged undeletable"}
			}
			if len(tags) == 1 && tags[0] == "publish-on-delete" {
				err := rt.Publish(ctx, WidgetCreated, map[string]string{"name": "from-delete-validate"})
				DeletePublishErr.Store(&err)
			}
			return nil
		}
		ext, err := parse(req.Extension)
		if err != nil {
			return err
		}
		if ext.Stall {
			if err := req.Tx.Exec(`INSERT INTO mod_testmod_agent_tags (agent_id, org_id, tag) VALUES (?, ?, ?)`,
				"01900000-0000-7000-8000-0000000000aa", req.OrgID, StalledTag).Error; err != nil {
				return err
			}
			<-ctx.Done()
			time.Sleep(200 * time.Millisecond)
			if req.Tx.WithContext(context.Background()).Exec(`UPDATE mod_testmod_agent_tags SET tag = ? WHERE tag = ?`,
				"late", StalledTag).Error == nil {
				AfterTimeout.Store(true)
			}
			return ctx.Err()
		}
		if ext.Reject {
			return modkit.Reject("extensions.%s: rejected on request", Name)
		}
		if len(ext.Tag) > 64 {
			return modkit.Reject("extensions.%s.tag is longer than 64 characters", Name)
		}
		return nil
	})
	rt.Hook(modkit.ResourceAgent, modkit.Apply, func(ctx context.Context, req modkit.HookRequest) error {
		ext, err := parse(req.Extension)
		if err != nil {
			return err
		}
		if ext.Announce {
			// Publishing with the hook's ctx goes through the core transaction.
			if err := rt.Publish(ctx, WidgetCreated, map[string]string{"name": "from-hook"}); err != nil {
				return err
			}
		}
		if ext.Lock {
			a, err := rt.Core().LockAgent(ctx, req.Tx, req.OrgID, req.ID)
			if err != nil {
				return err
			}
			if a.Name != req.Object["name"] {
				return errors.New("LockAgent returned another agent")
			}
		}
		if ext.Tag == "" {
			return nil
		}
		// Hooks get copies: this change is seen by nobody else.
		req.Object["name"] = "changed-by-testmod"
		if err := req.Tx.Exec(`DELETE FROM mod_testmod_agent_tags WHERE agent_id = ?`, req.ID).Error; err != nil {
			return err
		}
		return req.Tx.Exec(`INSERT INTO mod_testmod_agent_tags (agent_id, org_id, tag) VALUES (?, ?, ?)`,
			req.ID, req.OrgID, ext.Tag).Error
	})
	rt.Hook(modkit.ResourceAgent, modkit.Finalize, func(_ context.Context, req modkit.HookRequest) error {
		var tags []string
		if err := req.Tx.Raw(`SELECT tag FROM mod_testmod_agent_tags WHERE agent_id = ?`, req.ID).Scan(&tags).Error; err != nil {
			return err
		}
		if err := req.Tx.Exec(`DELETE FROM mod_testmod_agent_tags WHERE agent_id = ?`, req.ID).Error; err != nil {
			return err
		}
		switch {
		case len(tags) == 1 && tags[0] == "finalize-fails":
			// A failing statement after a partial write: both are undone,
			// and on PostgreSQL the core transaction must survive it.
			// Finalize cannot refuse, so the delete goes ahead.
			_ = req.Tx.Exec(`SELECT no_such_column FROM mod_testmod_agent_tags`).Error
			return errors.New("finalize failed on request")
		case len(tags) == 1 && tags[0] == "finalize-hangs":
			return slowStatement(req.Tx)
		}
		return nil
	})
	rt.Hook(modkit.ResourceEnvironment, modkit.Apply, func(_ context.Context, req modkit.HookRequest) error {
		mode := EnvironmentMode.Load()
		if mode == EnvironmentOK {
			return nil
		}
		// Written, then undone with the hook's savepoint.
		if err := req.Tx.Exec(`INSERT INTO mod_testmod_agent_tags (agent_id, org_id, tag) VALUES (?, ?, ?)`,
			req.ID, req.OrgID, EnvironmentTag).Error; err != nil {
			return err
		}
		if mode == EnvironmentHang {
			return slowStatement(req.Tx)
		}
		return errors.New("environment hook failed on request")
	})

	// A pinned project cannot be deleted; an unpinned one takes its pins'
	// leftovers with it. The cleanup is undone when the delete is refused.
	rt.Hook(modkit.ResourceProject, modkit.Finalize, func(_ context.Context, req modkit.HookRequest) error {
		var pins int64
		if err := req.Tx.Raw(`SELECT count(*) FROM mod_testmod_project_pins WHERE project_id = ?`, req.ID).Scan(&pins).Error; err != nil {
			return err
		}
		if err := req.Tx.Exec(`DELETE FROM mod_testmod_project_pins WHERE project_id = ?`, req.ID).Error; err != nil {
			return err
		}
		if pins > 0 {
			return &modkit.Error{Status: http.StatusConflict, Message: "the project is pinned by testmod"}
		}
		return nil
	})

	rt.Go("ticker", func(ctx context.Context) error {
		WorkerRuns.Add(1)
		WorkerRunning.Add(1)
		defer WorkerRunning.Add(-1)
		<-ctx.Done()
		return nil
	})

	rt.MCPTool(modkit.MCPTool{
		Name: Tool, Title: "Count tags", Description: "Counts the agents testmod has tagged in the organization.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tag":{"type":"string"}}}`), ReadOnly: true,
	}, "widget-read", func(ctx context.Context, args json.RawMessage) (any, error) {
		var in struct {
			Tag string `json:"tag"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, modkit.Reject("arguments: %v", err)
		}
		if in.Tag == "fail" {
			return nil, modkit.Reject("failing on request")
		}
		q := rt.DB().WithContext(ctx).Table("mod_testmod_agent_tags").Where("org_id = ?", modkit.OrgID(ctx))
		if in.Tag != "" {
			q = q.Where("tag = ?", in.Tag)
		}
		var n int64
		if err := q.Count(&n).Error; err != nil {
			return nil, err
		}
		return map[string]any{"orgId": modkit.OrgID(ctx), "tags": n}, nil
	})

	record := func(ctx context.Context, e modkit.Event) error {
		subject := e.Subject
		if e.Type == modkit.EventAgentCreated {
			// The name from the event data: a hook that changed its copy
			// of the object must not have changed this.
			var a struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(e.Data, &a)
			subject = a.Name
		}
		if e.Type == WidgetCreated {
			var w struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(e.Data, &w)
			subject = w.Name
			if w.Name == "flaky" {
				n, _ := flaky.LoadOrStore(e.ID, new(atomic.Int32))
				if n.(*atomic.Int32).Add(1) == 1 {
					return errors.New("flaky handler fails its first delivery")
				}
			}
		}
		var n int64
		db := rt.DB().WithContext(ctx)
		if err := db.Raw(`SELECT count(*) FROM mod_testmod_received WHERE event_id = ? AND event_type = ?`, e.ID, e.Type).
			Scan(&n).Error; err != nil || n > 0 {
			return err
		}
		return db.Exec(`INSERT INTO mod_testmod_received (event_id, event_type, org_id, subject) VALUES (?, ?, ?, ?)`,
			e.ID, e.Type, e.OrgID, subject).Error
	}
	for _, t := range []string{modkit.EventAgentCreated, modkit.EventAgentDeleted, modkit.EventEnvironmentCreated, WidgetCreated} {
		rt.Subscribe(t, record)
	}

	rt.Handle(http.MethodGet, "/tags", "widget-read", func(w http.ResponseWriter, r *http.Request) {
		org := modkit.OrgID(r.Context())
		agents, err := rt.Core().ListAgents(r.Context(), org, "")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		type tagged struct {
			Agent string `json:"agent"`
			Tag   string `json:"tag"`
		}
		out := []tagged{}
		for _, listed := range agents {
			a, err := rt.Core().GetAgent(r.Context(), org, listed.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
				return
			}
			var tags []string
			if err := rt.DB().WithContext(r.Context()).Raw(`SELECT tag FROM mod_testmod_agent_tags WHERE agent_id = ? AND org_id = ?`,
				a.ID, org).Scan(&tags).Error; err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
				return
			}
			for _, t := range tags {
				out = append(out, tagged{Agent: a.Name, Tag: t})
			}
		}
		writeJSON(w, http.StatusOK, out)
	})
	rt.Handle(http.MethodGet, "/whoami", "", func(w http.ResponseWriter, r *http.Request) {
		org, err := rt.Core().GetOrg(r.Context(), modkit.OrgID(r.Context()))
		if err != nil && !errors.Is(err, modkit.ErrNotFound) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		envs, _ := rt.Core().ListEnvironments(r.Context(), modkit.OrgID(r.Context()))
		projects, _ := rt.Core().ListProjects(r.Context(), modkit.OrgID(r.Context()))
		var names []string
		for _, p := range projects {
			got, err := rt.Core().GetProject(r.Context(), modkit.OrgID(r.Context()), p.ID)
			if err == nil {
				names = append(names, got.Name)
			}
		}
		_, foreign := rt.Core().GetProject(r.Context(), "01900000-0000-7000-8000-000000000000", "01900000-0000-7000-8000-000000000001")
		writeJSON(w, http.StatusOK, map[string]any{
			"orgId": modkit.OrgID(r.Context()), "orgHandle": org.Handle, "environments": len(envs),
			"projects": names, "foreignNotFound": errors.Is(foreign, modkit.ErrNotFound),
			"canWrite": rt.Can(r.Context(), "widget-write"), "canReadAgents": rt.Can(r.Context(), "agent:read"),
		})
	})
	rt.Handle(http.MethodPost, "/widgets", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		err := rt.DB().WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
			return rt.PublishTx(r.Context(), tx, WidgetCreated, map[string]string{"name": body.Name})
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"name": body.Name})
	})
	rt.Handle(http.MethodGet, "/received", "widget-read", func(w http.ResponseWriter, r *http.Request) {
		type received struct {
			EventType string  `json:"eventType"`
			Subject   *string `json:"subject"`
		}
		var out []received
		if err := rt.DB().WithContext(r.Context()).Raw(`SELECT event_type, subject FROM mod_testmod_received WHERE org_id = ? ORDER BY event_type`,
			modkit.OrgID(r.Context())).Scan(&out).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	rt.Handle(http.MethodPost, "/workloads", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		tok, err := rt.IssueToken(r.Context(), modkit.TokenRequest{Subject: "run:direct", Scopes: []string{PingScope}})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		cred, err := rt.IssueCredential(r.Context(), modkit.CredentialRequest{Subject: "run:exchanged", Scopes: []string{PingScope}})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"token": tok, "credentialId": cred.ID, "secret": cred.Secret})
	})

	rt.Handle(http.MethodPost, "/workloads/revoke", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CredentialID string `json:"credentialId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := rt.RevokeCredential(r.Context(), body.CredentialID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	rt.Handle(http.MethodPost, "/widgets/{name}/archive", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"name": r.PathValue("name"), "state": "archived"})
	}, modkit.Audit("widget-archive"), modkit.Emits(WidgetArchived))

	rt.Handle(http.MethodPut, "/secrets/{name}", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Value string `json:"value"`
			Org   string `json:"org"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		org := modkit.OrgID(r.Context())
		if body.Org != "" {
			org = body.Org
		}
		if err := rt.Secrets().Put(r.Context(), org, "notes/"+r.PathValue("name"), body.Value); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, modkit.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"message": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}, modkit.Audit("secret-write"))
	rt.Handle(http.MethodGet, "/secrets/{name}", "widget-read", func(w http.ResponseWriter, r *http.Request) {
		v, err := rt.Secrets().Get(r.Context(), modkit.OrgID(r.Context()), "notes/"+r.PathValue("name"))
		if errors.Is(err, modkit.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "no such secret"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"value": v})
	}, modkit.Audit("secret-read"))
	rt.Handle(http.MethodDelete, "/secrets/{name}", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		if err := rt.Secrets().Delete(r.Context(), modkit.OrgID(r.Context()), "notes/"+r.PathValue("name")); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}, modkit.Audit("secret-write"))

	rt.Handle(http.MethodGet, "/lookup", "widget-read", func(w http.ResponseWriter, r *http.Request) {
		ctx, org, q := r.Context(), modkit.OrgID(r.Context()), r.URL.Query()
		out := map[string]string{}
		if p, err := rt.Core().GetProjectByName(ctx, org, q.Get("project")); err == nil {
			out["project"] = p.ID
		}
		if a, err := rt.Core().GetAgentByName(ctx, org, q.Get("project"), q.Get("agent")); err == nil {
			out["agent"] = a.ID
		}
		if e, err := rt.Core().GetEnvironmentByName(ctx, org, q.Get("environment")); err == nil {
			out["environment"] = e.ID
		}
		writeJSON(w, http.StatusOK, out)
	})
	rt.Handle(http.MethodPost, "/org", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		o, err := rt.Core().EnsureOrg(r.Context(), modkit.OrgID(r.Context()))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": o.ID, "handle": o.Handle})
	})
	rt.Handle(http.MethodPost, "/projects/{name}/pin", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		ctx, org := r.Context(), modkit.OrgID(r.Context())
		p, err := rt.Core().GetProjectByName(ctx, org, r.PathValue("name"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": err.Error()})
			return
		}
		if err := rt.DB().WithContext(ctx).Exec(`INSERT INTO mod_testmod_project_pins (project_id, org_id) VALUES (?, ?)`, p.ID, org).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	rt.Handle(http.MethodDelete, "/projects/{name}/pin", "widget-write", func(w http.ResponseWriter, r *http.Request) {
		ctx, org := r.Context(), modkit.OrgID(r.Context())
		p, err := rt.Core().GetProjectByName(ctx, org, r.PathValue("name"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": err.Error()})
			return
		}
		if err := rt.DB().WithContext(ctx).Exec(`DELETE FROM mod_testmod_project_pins WHERE project_id = ?`, p.ID).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// A route at a fixed internal path, which authenticates its callers
	// itself, as the AI Gateway's controller routes do.
	legacy := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != LegacyKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "invalid api-key"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"method": r.Method})
	}
	rt.HandleInternalAt(http.MethodGet, LegacyPath, legacy)
	rt.HandleInternalAt(http.MethodPost, LegacyPath, legacy, modkit.Audit("legacy-ping"))

	rt.HandleInternal(http.MethodGet, "/ping", PingScope, func(w http.ResponseWriter, r *http.Request) {
		wl, _ := modkit.WorkloadFrom(r.Context())
		writeJSON(w, http.StatusOK, map[string]string{"subject": wl.Subject, "orgId": modkit.OrgID(r.Context())})
	})
	return nil
}

type consumer struct{}

func (consumer) Manifest() modkit.Manifest {
	return modkit.Manifest{Name: ConsumerName, Version: "1.0.0", CoreAPI: modkit.CoreAPIVersion, Requires: []string{Echo}}
}

func (consumer) Migrations() fs.FS { return nil }
func (consumer) UI() fs.FS         { return nil }

func (consumer) Activate(_ context.Context, rt modkit.Runtime) error {
	rt.Handle(http.MethodGet, "/echo", "", func(w http.ResponseWriter, r *http.Request) {
		e, ok := modkit.Lookup[Echoer](rt, Echo)
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "no echo provider"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"echo": e.Echo(r.URL.Query().Get("s"))})
	})
	return nil
}
