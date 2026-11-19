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

// Package custom is the Shadow Agent Discovery connector for a Custom
// platform: the customer's own agent inventory, a REST API built to
// sa.CustomPlatformOpenAPI. It calls only GET on the connection's Agents URL,
// with the API key as a bearer token, and decodes only the contract's agent
// fields.
package custom

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Call names, as stored in scan results and evidence sources.
const (
	callAgents      = "GET {Agents URL}"
	suffixPageLimit = " (page limit)"
	reasonNoAPIKey  = "no API key"
)

// Bounds (the contract documents them).
const (
	pageSize = 100
	maxPages = 100
	maxID    = 256
)

// Connector scans Custom platforms.
type Connector struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// New returns the Custom platform connector. httpClient is the
// redirect-guarded client (sa.NewHTTPClient); logger receives sanitized
// summaries only.
func New(httpClient *http.Client, logger *slog.Logger) sa.Connector {
	if httpClient == nil {
		httpClient = sa.NewHTTPClient(nil)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Connector{httpClient: httpClient, logger: logger}
}

// Provider returns models.ShadowProviderCustom.
func (c *Connector) Provider() string { return models.ShadowProviderCustom }

func (c *Connector) api(key string) *sa.API {
	return &sa.API{
		HTTP:   c.httpClient,
		Logger: c.logger,
		Header: func(h http.Header) { h.Set("Authorization", "Bearer "+key) },
	}
}

// target returns the connection's Agents URL and API key, or why it cannot
// be called.
func target(conn sa.Connection) (string, string, error) {
	u := strings.TrimSpace(conn.Config.AgentsURL)
	if err := sa.ValidateAgentsURL(u); err != nil {
		return "", "", &sa.CallError{Outcome: models.ShadowCallFailed, Message: err.Error()}
	}
	return u, strings.TrimSpace(conn.Secret.APIKey), nil
}

// The wire types follow the contract. Each agent is decoded on its own, so
// an agent that does not match the contract is left out instead of failing
// the page.
type agentPage struct {
	Agents     *[]json.RawMessage `json:"agents"`
	NextCursor *string            `json:"nextCursor"`
}

type agent struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Source      string            `json:"source"`
	URL         string            `json:"url"`
	Framework   string            `json:"framework"`
	Model       string            `json:"model"`
	Version     string            `json:"version"`
	Status      string            `json:"status"`
	Scope       string            `json:"scope"`
	Owner       *owner            `json:"owner"`
	CreatedAt   string            `json:"createdAt"`
	Labels      map[string]string `json:"labels"`
}

type owner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// page reads one page.
func (c *Connector) page(ctx context.Context, api *sa.API, u string, limit int, cursor string) (agentPage, error) {
	q := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var p agentPage
	if err := api.DoJSON(ctx, sa.Request{Method: http.MethodGet, URL: u, Query: q}, &p); err != nil {
		return p, err
	}
	if p.Agents == nil {
		return p, sa.Unsupported("the response has no agents list")
	}
	return p, nil
}

// Scan lists every agent the Agents URL returns.
func (c *Connector) Scan(ctx context.Context, conn sa.Connection) *sa.Result {
	calls := sa.NewCalls()
	fs := sa.NewFindings()
	read, left := c.listAgents(ctx, conn, calls, fs)
	findings := fs.All()
	for _, f := range findings {
		f.Attributes = sa.SanitizeAttributes(f.Attributes)
		f.DisplayName = sa.SanitizeName(f.DisplayName)
		f.OwnerHint = sa.MinimiseOwnerHint(f.OwnerHint)
	}
	res := &sa.Result{Findings: findings, Calls: calls.Results()}
	c.logger.Info("Shadow agent discovery Custom platform scan finished",
		"connectionId", conn.ID.String(), "agentsRead", read, "agentsLeftOut", left, "agents", len(findings))
	return res
}

// listAgents follows the pages. It returns how many agents were read and how
// many were left out.
func (c *Connector) listAgents(ctx context.Context, conn sa.Connection, calls *sa.Calls, fs *sa.Findings) (int, int) {
	u, key, err := target(conn)
	if err != nil {
		calls.Record(callAgents, 0, err)
		return 0, 0
	}
	if key == "" {
		calls.Skip(callAgents, reasonNoAPIKey)
		return 0, 0
	}
	api := c.api(key)
	read, invalid, repeated := 0, 0, 0
	cursor := ""
	more := true
	for pages := 0; more && pages < maxPages; pages++ {
		if ctx.Err() != nil {
			calls.Record(callAgents, len(fs.All()), &sa.CallError{
				Outcome: models.ShadowCallFailed, Message: "cancelled: " + ctx.Err().Error(),
			})
			return read, invalid + repeated
		}
		p, err := c.page(ctx, api, u, pageSize, cursor)
		if err != nil {
			calls.Record(callAgents, len(fs.All()), err)
			return read, invalid + repeated
		}
		for _, raw := range *p.Agents {
			read++
			var a agent
			if json.Unmarshal(raw, &a) != nil {
				invalid++
				continue
			}
			id, name := strings.TrimSpace(a.ID), strings.TrimSpace(a.Name)
			if id == "" || name == "" || len(id) > maxID {
				invalid++
				continue
			}
			if _, dup := fs.Lookup(models.ShadowKindPlatformAgent, id); dup {
				repeated++
				continue
			}
			addAgent(fs.Get(models.ShadowKindPlatformAgent, id), a, name)
		}
		next := ""
		if p.NextCursor != nil {
			next = strings.TrimSpace(*p.NextCursor)
		}
		if next != "" && next == cursor {
			calls.Record(callAgents, len(fs.All()), sa.Unsupported("nextCursor did not change"))
			return read, invalid + repeated
		}
		cursor, more = next, next != ""
	}
	calls.Record(callAgents, len(fs.All()), nil)
	if more {
		// The listing stopped at its page bound: it is incomplete, so the
		// call is recorded as failed.
		calls.Record(callAgents+suffixPageLimit, 0, &sa.CallError{
			Outcome: models.ShadowCallFailed,
			Message: fmt.Sprintf("stopped after %d pages; later pages were not read", maxPages),
		})
	}
	if invalid > 0 {
		calls.Note(callAgents, fmt.Sprintf("%d agent(s) left out: no id or name, an id longer than %d characters, "+
			"or fields that do not match the contract.", invalid, maxID))
	}
	if repeated > 0 {
		calls.Note(callAgents, fmt.Sprintf("%d repeated agent id(s) listed once.", repeated))
	}
	return read, invalid + repeated
}

// addAgent fills a finding from a contract agent. Only the contract's
// fields are read; the sanitizer cuts them to their stored lengths.
func addAgent(f *sa.Finding, a agent, name string) {
	f.DisplayName = name
	f.Scope = strings.TrimSpace(a.Scope)
	f.State = strings.TrimSpace(a.Status)
	if a.Owner != nil {
		f.OwnerHint = models.ShadowOwnerHint{Name: a.Owner.Name, Email: a.Owner.Email}
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(a.CreatedAt)); err == nil {
		utc := t.UTC()
		f.ProviderCreatedAt = &utc
	}
	set := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" {
			f.Attributes[k] = v
		}
	}
	set("description", a.Description)
	set("source", a.Source)
	set("agentUrl", sa.RepositoryURL(a.URL))
	set("framework", a.Framework)
	set("model", a.Model)
	set("agentVersion", a.Version)
	if len(a.Labels) > 0 {
		f.Attributes["labels"] = a.Labels
	}
	f.AddSignal(sa.Signal{Code: sa.SigCustomAgent, Source: callAgents})
}

// Test reads one agent with the API key. OK means the Agents URL returned a
// page in the contract's shape.
func (c *Connector) Test(ctx context.Context, conn sa.Connection) models.ShadowConnectionTestInfo {
	info := models.ShadowConnectionTestInfo{Checks: []models.ShadowConnectionCheck{}}
	check := models.ShadowConnectionCheck{Name: "Agents", Unlocks: []string{sa.SigCustomAgent}}
	u, key, err := target(conn)
	switch {
	case err != nil:
		check.Outcome, check.Message = sa.OutcomeOf(err), sa.MessageOf(err)
	case key == "":
		check.Outcome, check.Message = models.ShadowCallSkipped, reasonNoAPIKey
	default:
		_, err = c.page(ctx, c.api(key), u, 1, "")
		check.Outcome, check.Message = sa.OutcomeOf(err), sa.MessageOf(err)
		info.OK = err == nil
	}
	info.Checks = append(info.Checks, check)
	c.logger.Info("Shadow agent discovery Custom platform connection test finished",
		"connectionId", conn.ID.String(), "ok", info.OK)
	return info
}
