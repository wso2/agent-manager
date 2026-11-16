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

package azure

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Foundry bounds.
const (
	// projectEndpointKey is the properties.endpoints key that holds the
	// project endpoint (Learn quickstart CLI query; evidence/foundry.md §2).
	projectEndpointKey = "AI Foundry API"
	agentsPageSize     = 100
	classicKind        = "classic"
)

// Foundry scans Microsoft Foundry Agent Service projects.
type Foundry struct {
	base
}

// NewFoundry returns the Microsoft Foundry Agent Service connector.
// httpClient must be the redirect-guarded client from sa.NewHTTPClient.
func NewFoundry(httpClient *http.Client, logger *slog.Logger) sa.Connector {
	return &Foundry{base: newBase(httpClient, logger)}
}

// Provider implements sa.Connector.
func (c *Foundry) Provider() string { return models.ShadowProviderFoundry }

// project is a Projects_List item (2025-06-01).
type project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Properties struct {
		DisplayName       string            `json:"displayName"`
		Endpoints         map[string]string `json:"endpoints"`
		ProvisioningState string            `json:"provisioningState"`
	} `json:"properties"`
}

// cursorPage is the Foundry data-plane list envelope (data, first_id,
// last_id, has_more; no "object" field).
type cursorPage[T any] struct {
	Data    []T    `json:"data"`
	LastID  string `json:"last_id"`
	HasMore bool   `json:"has_more"`
}

// unixTime decodes a unix-seconds timestamp leniently (number, numeric
// string or null).
type unixTime int64

func (u *unixTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*u = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*u = 0
		return nil //nolint:nilerr // lenient: an unparseable timestamp is treated as absent
	}
	*u = unixTime(int64(f))
	return nil
}

func (u unixTime) time() *time.Time {
	if u <= 0 {
		return nil
	}
	t := time.Unix(int64(u), 0).UTC()
	return &t
}

// agentObject is the v1 AgentObject with only the fields used. The version
// definition is decoded separately so instructions, environment variables,
// MCP headers and authorization are never decoded at all.
type agentObject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Versions struct {
		Latest *struct {
			Version     string          `json:"version"`
			Description string          `json:"description"`
			Status      string          `json:"status"`
			Definition  json.RawMessage `json:"definition"`
		} `json:"latest"`
	} `json:"versions"`
	AgentCard *struct {
		Skills []struct {
			Name string `json:"name"`
		} `json:"skills"`
	} `json:"agent_card"`
}

// agentDefinition is AgentDefinition (discriminator kind) with only the
// non-content fields used.
type agentDefinition struct {
	Kind                   string            `json:"kind"`
	Model                  string            `json:"model"`
	Tools                  []json.RawMessage `json:"tools"`
	ContainerConfiguration *struct {
		Image string `json:"image"`
	} `json:"container_configuration"`
	OtelAgentID string `json:"otel_agent_id"`
}

// toolDef is the identity part of a tool definition (new OpenAI.Tool and
// classic ToolDefinition shapes). Schemas, arguments, headers and
// authorization are not decoded.
type toolDef struct {
	Type                string `json:"type"`
	Name                string `json:"name"`
	ServerLabel         string `json:"server_label"`
	ServerURL           string `json:"server_url"`
	ConnectorID         string `json:"connector_id"`
	TunnelID            string `json:"tunnel_id"`
	ProjectConnectionID string `json:"project_connection_id"`
	Function            *struct {
		Name string `json:"name"`
	} `json:"function"`
	OpenAPI *struct {
		Name string `json:"name"`
	} `json:"openapi"`
	ConnectedAgent *struct {
		Name string `json:"name"`
	} `json:"connected_agent"`
}

// classicAgent is the classic Agent object (CSPEC v1) with only the fields
// used; instructions and tool_resources are never decoded.
type classicAgent struct {
	ID          string            `json:"id"`
	CreatedAt   unixTime          `json:"created_at"`
	Name        *string           `json:"name"`
	Description *string           `json:"description"`
	Model       string            `json:"model"`
	Tools       []json.RawMessage `json:"tools"`
}

// dataPlane calls Foundry project endpoints with an ai.azure.com token.
type dataPlane struct {
	client     jsonGetter
	okCalls    int
	authFailed bool
}

// get GETs u; the first failing authentication stops further data-plane
// calls (the same token is used for every project).
func (d *dataPlane) get(ctx context.Context, u string, out any) error {
	err := d.client.GetJSON(ctx, u, out)
	if err == nil {
		d.okCalls++
	} else if isAuthFailure(err) && d.okCalls == 0 {
		d.authFailed = true
	}
	return err
}

// cursorList pages a data-plane list with limit/after/has_more. each
// returns false to stop early.
func cursorList[T any](ctx context.Context, d *dataPlane, endpoint, path string, params []qp, maxPages int, idOf func(T) string, each func(T) bool) (int, string, error) {
	n := 0
	after := ""
	for page := 0; ; page++ {
		if page == maxPages {
			return n, "stopped after " + strconv.Itoa(maxPages) + " pages; the results are partial", nil
		}
		if err := ctx.Err(); err != nil {
			return n, "", &sa.CallError{Outcome: models.ShadowCallFailed, Message: "cancelled: " + err.Error()}
		}
		ps := append([]qp{}, params...)
		if after != "" {
			ps = append(ps, qp{"after", after})
		}
		var p cursorPage[T]
		if err := d.get(ctx, buildURL(endpoint, path, ps...), &p); err != nil {
			return n, "", err
		}
		for _, v := range p.Data {
			n++
			if !each(v) {
				return n, "", nil
			}
		}
		if !p.HasMore || len(p.Data) == 0 {
			return n, "", nil
		}
		// Pass last_id back verbatim; the classic API may omit it, then the
		// last item's id is the cursor (evidence/foundry_classic.md C1).
		next := p.LastID
		if next == "" {
			next = idOf(p.Data[len(p.Data)-1])
		}
		if next == "" || next == after {
			return n, "has_more was true but no cursor was returned; the results are partial", nil
		}
		after = next
	}
}

// foundryScan is one scan's state.
type foundryScan struct {
	c        *Foundry
	conn     sa.Connection
	arm      *armClient
	dp       *dataPlane
	calls    *sa.Calls
	findings *sa.Findings
	classic  struct {
		recorded    bool
		unavailable int
	}
}

// Scan implements sa.Connector.
func (c *Foundry) Scan(ctx context.Context, conn sa.Connection) *sa.Result {
	calls := sa.NewCalls()
	res := &sa.Result{}
	s, err := c.newSession(conn)
	if err != nil {
		calls.Record(callSignIn, 0, err)
		res.Calls = calls.Results()
		return res
	}
	fs := &foundryScan{
		c: c, conn: conn, arm: s.arm, calls: calls, findings: sa.NewFindings(),
		dp: &dataPlane{client: sa.NewAzureClient(c.httpClient, s.cred, sa.AzureFoundryScope)},
	}
	subs, stop := s.arm.subscriptions(ctx, conn, calls, c.logger)
	if stop {
		res.Calls = calls.Results()
		return res
	}
	firstARMCall := !conn.Config.AllSubscriptions
	projects := 0
	for _, sub := range subs {
		if ctx.Err() != nil {
			break
		}
		accs, err := s.arm.accounts(ctx, sub, []string{kindAIServices}, calls, c.logger)
		if err != nil && firstARMCall && isAuthFailure(err) {
			res.Calls = calls.Results()
			return res
		}
		firstARMCall = false
		for _, acc := range accs {
			if ctx.Err() != nil {
				break
			}
			projects += fs.scanAccount(ctx, acc)
		}
	}
	fs.finish()
	res.Findings = fs.findings.All()
	res.Calls = calls.Results()
	c.logger.Info("Shadow agent discovery Foundry scan finished",
		"subscriptions", len(subs), "projects", projects, "agents", len(res.Findings))
	return res
}

// finish records the calls that were never made.
func (fs *foundryScan) finish() {
	if !fs.conn.Config.IncludeClassicAgents {
		fs.calls.Skip(callAssistants, "classic agents are not included in this connection")
	} else if !fs.classic.recorded && fs.classic.unavailable > 0 {
		fs.calls.Skip(callAssistants, "classic agents not available")
	}
	if fs.dp.authFailed {
		fs.calls.Skip(callAgents+" (remaining projects)", "not called: authentication to the Foundry data plane failed")
	}
}

// scanAccount lists the account's projects and scans each. It returns the
// number of projects scanned.
func (fs *foundryScan) scanAccount(ctx context.Context, acc *account) int {
	if p := acc.Properties.AllowProjectManagement; p != nil && !*p {
		return 0
	}
	var projs []project
	u := fs.arm.url(acc.ID+"/projects", qp{"api-version", apiCognitive})
	n, stop, err := armList(ctx, fs.arm, u, maxARMPages, func(p project) {
		if p.ID != "" {
			projs = append(projs, p)
		}
	})
	record(fs.calls, fs.c.logger, callProjects, n, stop, err)
	scanned := 0
	for _, p := range projs {
		if ctx.Err() != nil || fs.dp.authFailed {
			break
		}
		ep, err := projectEndpoint(p, fs.arm.base)
		fs.calls.Record(callProjectEP, boolToInt(err == nil), err)
		if err != nil {
			continue
		}
		scanned++
		fs.scanProject(ctx, acc, p, ep)
	}
	return scanned
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// projectEndpoint returns the project's "AI Foundry API" endpoint. It must
// be https, unless the configured ARM endpoint itself is http (a mock or a
// private test cloud configured by the user).
func projectEndpoint(p project, arm *url.URL) (string, error) {
	raw, ok := p.Properties.Endpoints[projectEndpointKey]
	if !ok {
		for k, v := range p.Properties.Endpoints {
			if strings.EqualFold(k, projectEndpointKey) {
				raw, ok = v, true
				break
			}
		}
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return "", &sa.CallError{Outcome: models.ShadowCallNotFound, Message: `the project has no "AI Foundry API" endpoint; it was skipped`}
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return "", sa.Unsupported("the project endpoint is not a valid URL; the project was skipped")
	}
	allowHTTP := strings.EqualFold(arm.Scheme, "http")
	httpsOK := strings.EqualFold(u.Scheme, "https")
	httpOK := allowHTTP && strings.EqualFold(u.Scheme, "http")
	if !httpsOK && !httpOK {
		return "", sa.Unsupported("the project endpoint is not https; the project was skipped")
	}
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

// scanProject lists the project's agents and, when the connection includes
// them, its classic agents.
func (fs *foundryScan) scanProject(ctx context.Context, acc *account, p project, ep string) {
	params := []qp{{"api-version", apiFoundry}, {"limit", strconv.Itoa(agentsPageSize)}, {"order", "asc"}}
	n, stop, err := cursorList(ctx, fs.dp, ep, "/agents", params, maxAgentPages,
		func(a agentObject) string { return a.ID },
		func(a agentObject) bool {
			fs.agentFinding(acc, p, ep, a)
			return true
		})
	record(fs.calls, fs.c.logger, callAgents, n, stop, err)
	if fs.dp.authFailed {
		return
	}
	if fs.conn.Config.IncludeClassicAgents {
		fs.scanClassic(ctx, acc, p, ep)
	}
}

// agentFinding turns an AgentObject into a platform_agent finding.
func (fs *foundryScan) agentFinding(acc *account, p project, ep string, a agentObject) {
	key := firstNonEmpty(a.ID, a.Name)
	if key == "" {
		return
	}
	f := fs.findings.Get(models.ShadowKindPlatformAgent, ep+"/agents/"+key)
	f.DisplayName = sa.SanitizeName(firstNonEmpty(a.Name, a.ID))
	f.Scope = p.ID
	f.State = a.State
	fs.commonAttributes(f, acc, p, ep)
	var def agentDefinition
	if v := a.Versions.Latest; v != nil {
		f.Attributes["agentVersion"] = v.Version
		f.Attributes["status"] = v.Status
		f.Attributes["description"] = v.Description
		if len(v.Definition) > 0 {
			_ = json.Unmarshal(v.Definition, &def) // lenient: an unexpected shape leaves fields empty
		}
	}
	f.Attributes["agentKind"] = def.Kind
	if def.ContainerConfiguration != nil {
		f.Attributes["image"] = def.ContainerConfiguration.Image
	}
	if strings.EqualFold(def.Kind, "external") {
		// Foundry stores no endpoint for external agents; the service flags
		// possible duplicates across providers by name and this ID.
		f.Attributes["otelAgentId"] = firstNonEmpty(def.OtelAgentID, a.Name)
	}
	if a.AgentCard != nil {
		var skills []string
		for _, s := range a.AgentCard.Skills {
			skills = append(skills, s.Name)
		}
		f.Attributes["skills"] = skills
	}
	modelAttributes(f, def.Model)
	f.AddSignal(sa.Signal{
		Code: sa.SigFoundryAgentObject, Source: callAgents,
		Observed: map[string]any{"enabled": strings.EqualFold(a.State, "enabled")},
	})
	toolAttributes(f, def.Tools)
}

// scanClassic lists the project's classic (assistants-style) agents.
// 404 or 410 means the classic API is not available for the project.
func (fs *foundryScan) scanClassic(ctx context.Context, acc *account, p project, ep string) {
	params := []qp{{"api-version", apiFoundry}, {"limit", strconv.Itoa(agentsPageSize)}, {"order", "asc"}}
	n, stop, err := cursorList(ctx, fs.dp, ep, "/assistants", params, maxAgentPages,
		func(a classicAgent) string { return a.ID },
		func(a classicAgent) bool {
			if a.ID == "" {
				return true
			}
			f := fs.findings.Get(models.ShadowKindPlatformAgent, ep+"/assistants/"+a.ID)
			name := a.ID
			if a.Name != nil && strings.TrimSpace(*a.Name) != "" {
				name = *a.Name
			}
			f.DisplayName = sa.SanitizeName(name)
			f.Scope = p.ID
			f.ProviderCreatedAt = a.CreatedAt.time()
			fs.commonAttributes(f, acc, p, ep)
			f.Attributes["agentKind"] = classicKind
			if a.Description != nil {
				f.Attributes["description"] = *a.Description
			}
			modelAttributes(f, a.Model)
			f.AddSignal(sa.Signal{Code: sa.SigFoundryClassicAgent, Source: callAssistants, Observed: map[string]any{}})
			toolAttributes(f, a.Tools)
			return true
		})
	if s := statusOf(err); err != nil && (s == http.StatusNotFound || s == http.StatusGone) {
		fs.classic.unavailable++
		return
	}
	fs.classic.recorded = true
	record(fs.calls, fs.c.logger, callAssistants, n, stop, err)
}

func (fs *foundryScan) commonAttributes(f *sa.Finding, acc *account, p project, ep string) {
	f.Attributes["resourceId"] = acc.ID
	f.Attributes["accountId"] = acc.subscriptionID
	f.Attributes["accountKind"] = acc.Kind
	f.Attributes["region"] = acc.Location
	f.Attributes["project"] = p.Name
	f.Attributes["endpoint"] = sa.Origin(ep)
}

// modelAttributes records the model and, when it names a deployment on the
// agent's account, the deployment. "<connection>/<model>" is a connected
// model on another resource (Learn, connected models): it is kept as the
// model only.
func modelAttributes(f *sa.Finding, model string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	f.Attributes["model"] = model
	if !strings.Contains(model, "/") {
		f.Attributes["deployment"] = model
	}
}

// toolAttributes records tool types and names, and MCP server labels and
// origins.
func toolAttributes(f *sa.Finding, raw []json.RawMessage) {
	tools, mcps := parseTools(raw)
	if len(tools) == 0 {
		return
	}
	f.Attributes["tools"] = tools
	f.Attributes["toolCount"] = len(tools)
	if len(mcps) > 0 {
		f.Attributes["mcpServers"] = mcps
	}
}

// parseTools reduces tool definitions to their type and name, and MCP tools
// to their label and the origin of server_url. connector_id, tunnel_id and
// project_connection_id MCP tools keep the label only.
func parseTools(raw []json.RawMessage) ([]sa.ToolRef, []sa.MCPServerRef) {
	var tools []sa.ToolRef
	var mcps []sa.MCPServerRef
	for _, r := range raw {
		var t toolDef
		if err := json.Unmarshal(r, &t); err != nil || strings.TrimSpace(t.Type) == "" {
			continue
		}
		name := t.Name
		switch {
		case strings.EqualFold(t.Type, "mcp"):
			name = t.ServerLabel
			ref := sa.MCPServerRef{Label: t.ServerLabel}
			if t.ServerURL != "" {
				ref.Origin = sa.Origin(t.ServerURL)
			}
			mcps = append(mcps, ref)
		case t.Function != nil && name == "":
			name = t.Function.Name
		case t.OpenAPI != nil && name == "":
			name = t.OpenAPI.Name
		case t.ConnectedAgent != nil && name == "":
			name = t.ConnectedAgent.Name
		}
		tools = append(tools, sa.ToolRef{Type: t.Type, Name: name})
	}
	return tools, mcps
}

// Foundry connection test check names.
const (
	checkProjects = "Foundry projects (Projects_List)"
	checkAgents   = "Foundry agents (GET {projectEndpoint}/agents)"
	checkClassic  = "Foundry classic agents (GET {projectEndpoint}/assistants)"
)

// Test implements sa.Connector. The connection is OK when the accounts and
// the agents of the first project can be listed.
func (c *Foundry) Test(ctx context.Context, conn sa.Connection) models.ShadowConnectionTestInfo {
	agentSignals := []string{sa.SigFoundryAgentObject}
	all := []string{sa.SigFoundryAgentObject, sa.SigFoundryClassicAgent}
	s, err := c.newSession(conn)
	if err != nil {
		return models.ShadowConnectionTestInfo{Checks: []models.ShadowConnectionCheck{{
			Name: checkSignIn, Outcome: sa.OutcomeOf(err), Message: sa.MessageOf(err), Unlocks: all,
		}}}
	}
	dp := &dataPlane{client: sa.NewAzureClient(c.httpClient, s.cred, sa.AzureFoundryScope)}
	t := newTester()
	_, accs := t.subscriptionAndAccounts(ctx, s.arm, conn, []string{kindAIServices}, all)
	var acc *account
	if len(accs) > 0 {
		acc = accs[0]
	}
	ep := ""
	t.run(checkProjects, agentSignals, acc == nil, "no AI Services account found", func() (int, error) {
		n, _, err := armList(ctx, s.arm, s.arm.url(acc.ID+"/projects", qp{"api-version", apiCognitive}), testMaxPages, func(pr project) {
			if ep != "" {
				return
			}
			if e, perr := projectEndpoint(pr, s.arm.base); perr == nil {
				ep = e
			}
		})
		return n, err
	})
	one := []qp{{"api-version", apiFoundry}, {"limit", "1"}}
	t.run(checkAgents, agentSignals, ep == "", "no project with an \"AI Foundry API\" endpoint found", func() (int, error) {
		var p cursorPage[json.RawMessage]
		err := dp.get(ctx, buildURL(ep, "/agents", one...), &p)
		return len(p.Data), err
	})
	if conn.Config.IncludeClassicAgents {
		t.run(checkClassic, []string{sa.SigFoundryClassicAgent}, ep == "", "no project with an \"AI Foundry API\" endpoint found", func() (int, error) {
			var p cursorPage[json.RawMessage]
			err := dp.get(ctx, buildURL(ep, "/assistants", one...), &p)
			if s := statusOf(err); s == http.StatusNotFound || s == http.StatusGone {
				return 0, errClassicUnavailable
			}
			return len(p.Data), err
		})
		t.markSkipped(checkClassic, errClassicUnavailable)
	}
	info := t.info(checkAccounts)
	// The agents check is required unless there is simply no project yet.
	for _, ch := range t.checks {
		noProjectYet := ch.Outcome == models.ShadowCallSkipped && ep == "" && !t.authFailed
		if ch.Name == checkAgents && ch.Outcome != models.ShadowCallOK && !noProjectYet {
			info.OK = false
		}
	}
	return info
}

var errClassicUnavailable = errors.New("classic agents not available")

// markSkipped turns a check that failed with marker into a skipped check.
func (t *tester) markSkipped(name string, marker error) {
	for i := range t.checks {
		if t.checks[i].Name == name && strings.Contains(t.checks[i].Message, marker.Error()) {
			t.checks[i].Outcome = models.ShadowCallSkipped
			t.checks[i].Message = marker.Error()
		}
	}
}
