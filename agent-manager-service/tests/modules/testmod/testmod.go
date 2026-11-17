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
// module contract from the tests: routes and permissions, agent and
// environment hooks, events, a capability, and workload tokens. No binary
// links them; tests pass them to the module host explicitly, so they do not
// register themselves.
package testmod

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"

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
	// PingScope is the workload scope of Name's internal route.
	PingScope = "ping"
)

//go:embed migrations
var migrations embed.FS

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
		Name:     Name,
		Version:  "1.0.0",
		CoreAPI:  modkit.CoreAPIVersion,
		Provides: []string{Echo},
		Permissions: []modkit.Permission{
			{Name: "widget:read", Description: "Read widgets"},
			{Name: "widget:write", Description: "Write widgets"},
		},
		RoleGrants: map[string][]string{"Agent Manager Admin": {"widget:read", "widget:write"}},
		Events:     []string{WidgetCreated},
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

// extension is the agent extension block Name accepts.
type extension struct {
	Tag    string `json:"tag"`
	Reject bool   `json:"reject"`
}

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

	rt.Hook(modkit.ResourceAgent, modkit.Validate, func(_ context.Context, req modkit.HookRequest) error {
		ext, err := parse(req.Extension)
		if err != nil {
			return err
		}
		if ext.Reject {
			return modkit.Reject("extensions.%s: rejected on request", Name)
		}
		if len(ext.Tag) > 64 {
			return modkit.Reject("extensions.%s.tag is longer than 64 characters", Name)
		}
		return nil
	})
	rt.Hook(modkit.ResourceAgent, modkit.Apply, func(_ context.Context, req modkit.HookRequest) error {
		ext, err := parse(req.Extension)
		if err != nil || ext.Tag == "" {
			return err
		}
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
		if len(tags) == 1 && tags[0] == "undeletable" {
			return &modkit.Error{Status: http.StatusConflict, Message: "the agent is tagged undeletable"}
		}
		return req.Tx.Exec(`DELETE FROM mod_testmod_agent_tags WHERE agent_id = ?`, req.ID).Error
	})
	rt.Hook(modkit.ResourceEnvironment, modkit.Apply, func(context.Context, modkit.HookRequest) error { return nil })

	record := func(ctx context.Context, e modkit.Event) error {
		var n int64
		db := rt.DB().WithContext(ctx)
		if err := db.Raw(`SELECT count(*) FROM mod_testmod_received WHERE event_id = ? AND event_type = ?`, e.ID, e.Type).
			Scan(&n).Error; err != nil || n > 0 {
			return err
		}
		return db.Exec(`INSERT INTO mod_testmod_received (event_id, event_type, org_id, subject) VALUES (?, ?, ?, ?)`,
			e.ID, e.Type, e.OrgID, e.Subject).Error
	}
	for _, t := range []string{modkit.EventAgentCreated, modkit.EventAgentDeleted, modkit.EventEnvironmentCreated, WidgetCreated} {
		rt.Subscribe(t, record)
	}

	rt.Handle(http.MethodGet, "/tags", "widget:read", func(w http.ResponseWriter, r *http.Request) {
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
			"canWrite": rt.Can(r.Context(), "widget:write"), "canReadAgents": rt.Can(r.Context(), "agent:read"),
		})
	})
	rt.Handle(http.MethodPost, "/widgets", "widget:write", func(w http.ResponseWriter, r *http.Request) {
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
	rt.Handle(http.MethodGet, "/received", "widget:read", func(w http.ResponseWriter, r *http.Request) {
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
	rt.Handle(http.MethodPost, "/workloads", "widget:write", func(w http.ResponseWriter, r *http.Request) {
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
