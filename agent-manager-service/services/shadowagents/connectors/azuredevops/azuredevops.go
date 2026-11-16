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

// Package azuredevops is the Azure DevOps source host of Shadow Agent
// Discovery. Its connection test checks, with read-only calls, that the
// personal access token or the Microsoft Entra service principal reaches the
// organization's projects and Azure Repos repositories and can read code.
// The source code analysis that scans the repositories is added separately;
// until then a scan records it as skipped.
package azuredevops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

const (
	// apiVersion is the documented version of the Core and Git APIs used.
	apiVersion = "7.1"
	// entraScope is the token scope for Azure DevOps in Microsoft's service
	// principal guide.
	entraScope = "https://app.vssps.visualstudio.com/.default"
	// maxProjectsChecked bounds the projects a test reads repositories of.
	maxProjectsChecked = 20
	maxJSONBody        = 8 << 20
)

// Connection check names.
const (
	checkSignIn       = "Microsoft Entra sign-in"
	checkProjects     = "Projects (Project and team: read)"
	checkRepositories = "Repositories (Code: read)"
	checkContents     = "Repository contents (Code: read)"
)

// Connector is the Azure DevOps source host.
type Connector struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// New returns the Azure DevOps connector. httpClient is the
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

// Provider returns models.ShadowProviderAzureDevOps.
func (c *Connector) Provider() string { return models.ShadowProviderAzureDevOps }

// Scan records the source code analysis as not available yet. No
// repository is read.
func (c *Connector) Scan(_ context.Context, _ sa.Connection) *sa.Result {
	calls := sa.NewCalls()
	calls.Skip(sa.CallSourceAnalysis, sa.ReasonAnalysisUnavailable)
	return &sa.Result{Calls: calls.Results()}
}

type project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type repository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	Size          int64  `json:"size"`
	IsDisabled    bool   `json:"isDisabled"`
	IsFork        bool   `json:"isFork"`
}

type list[T any] struct {
	Count int `json:"count"`
	Value []T `json:"value"`
}

// client sends Azure DevOps REST calls with the connection's credential.
type client struct {
	api  *sa.API
	base string // https://dev.azure.com/{organization}
}

// get GETs path under the organization and decodes the JSON response.
// Azure DevOps can answer a refused credential with 203 and its sign-in
// page instead of 401; that is reported as a refused credential.
func (cl *client) get(ctx context.Context, path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	q.Set("api-version", apiVersion)
	resp, err := cl.api.Do(ctx, sa.Request{Method: http.MethodGet, URL: cl.base + path, Query: q})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNonAuthoritativeInfo ||
		!strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		return &sa.CallError{
			Outcome: models.ShadowCallAuthFailed, Status: resp.StatusCode,
			Message: "Azure DevOps returned its sign-in page: the credential was not accepted for this organization",
		}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBody+1))
	if err != nil || len(body) > maxJSONBody {
		return &sa.CallError{Outcome: models.ShadowCallFailed, Message: "the response could not be read"}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return sa.Unsupported("the response did not match the documented shape")
	}
	return nil
}

// newClient builds the client for the connection's credential: the PAT as
// Basic auth with an empty user name, or a Microsoft Entra token.
func (c *Connector) newClient(conn sa.Connection) (*client, error) {
	base, err := sa.Expand(sa.EndpointOf(conn, sa.EPAzureDevOps), nil)
	if err == nil && base == "" {
		err = errors.New("the Azure DevOps endpoint is not set")
	}
	if err != nil {
		return nil, err
	}
	org := strings.TrimSpace(conn.Config.Organization)
	if org == "" {
		return nil, errors.New("the organization is not set")
	}
	api := &sa.API{HTTP: c.httpClient, Logger: c.logger}
	if conn.Config.UseEntra {
		login, err := sa.Expand(sa.EndpointOf(conn, sa.EPAzureLogin), nil)
		if err != nil {
			return nil, err
		}
		cred, err := sa.AzureCredential(c.httpClient, login, conn.Config.TenantID, conn.Config.ClientID, conn.Secret.ClientSecret)
		if err != nil {
			return nil, &sa.CallError{Outcome: models.ShadowCallAuthFailed, Message: err.Error()}
		}
		api.Header = suppressRedirect
		api.Authorize = func(ctx context.Context, req *http.Request, _ []byte) error {
			tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{entraScope}})
			if err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+tok.Token)
			return nil
		}
	} else {
		pat := strings.TrimSpace(conn.Secret.PersonalAccessToken)
		if pat == "" {
			return nil, &sa.CallError{Outcome: models.ShadowCallAuthFailed, Message: "no personal access token is stored"}
		}
		basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+pat))
		api.Header = func(h http.Header) {
			suppressRedirect(h)
			h.Set("Authorization", basic)
		}
	}
	return &client{api: api, base: strings.TrimRight(base, "/") + "/" + url.PathEscape(org)}, nil
}

// suppressRedirect asks Azure DevOps to answer 401 rather than redirect to
// its sign-in page when a credential is refused.
func suppressRedirect(h http.Header) {
	h.Set("X-TFS-FedAuthRedirect", "Suppress")
}

// Test checks the credential with read-only calls: (Entra) the sign-in, the
// projects, the repositories of the projects and the top-level items of one
// repository. Nothing a call returns is stored beyond counts.
func (c *Connector) Test(ctx context.Context, conn sa.Connection) models.ShadowConnectionTestInfo {
	info := models.ShadowConnectionTestInfo{Checks: []models.ShadowConnectionCheck{}}
	add := func(name, message string, err error) bool {
		check := models.ShadowConnectionCheck{Name: name, Outcome: sa.OutcomeOf(err), Message: sa.MessageOf(err)}
		if err == nil {
			check.Message = message
		}
		info.Checks = append(info.Checks, check)
		return err == nil
	}
	skip := func(name, reason string) {
		info.Checks = append(info.Checks, models.ShadowConnectionCheck{Name: name, Outcome: models.ShadowCallSkipped, Message: reason})
	}
	skipRest := func(reason string, names ...string) {
		for _, n := range names {
			skip(n, reason)
		}
		skip(sa.CallSourceAnalysis, sa.ReasonAnalysisUnavailable)
	}
	first := checkProjects
	if conn.Config.UseEntra {
		first = checkSignIn
	}
	cl, err := c.newClient(conn)
	if err != nil {
		add(first, "", err)
		return info
	}
	if conn.Config.UseEntra {
		// One token request up front, so a refused secret is reported as the
		// sign-in rather than as every call failing.
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cl.base, nil)
		if err := cl.api.Authorize(ctx, req, nil); err != nil {
			add(checkSignIn, "", &sa.CallError{Outcome: models.ShadowCallAuthFailed, Message: sa.ScrubText(err.Error())})
			skipRest("the service principal could not sign in", checkProjects, checkRepositories, checkContents)
			return info
		}
		add(checkSignIn, "token issued for Azure DevOps", nil)
	}

	// Some selected projects can be missing: the found ones are still
	// checked, so the test says what does work.
	projects, msg, err := c.projects(ctx, cl, conn)
	projectsOK := add(checkProjects, msg, err)
	if !projectsOK && len(projects) == 0 {
		skipRest("the projects could not be read", checkRepositories, checkContents)
		return info
	}
	if len(projects) == 0 {
		info.Checks[len(info.Checks)-1].Outcome = models.ShadowCallNotFound
		info.Checks[len(info.Checks)-1].Message = "the credential reads no project in this organization"
		skipRest("no project to read", checkRepositories, checkContents)
		return info
	}

	var target *repository
	var targetProject project
	repos, disabled := 0, 0
	var reposErr error
	checked := 0
	for _, p := range projects {
		if checked == maxProjectsChecked {
			break
		}
		checked++
		var rl list[repository]
		if err := cl.get(ctx, "/"+url.PathEscape(p.ID)+"/_apis/git/repositories", nil, &rl); err != nil {
			reposErr = err
			break
		}
		for i := range rl.Value {
			r := rl.Value[i]
			repos++
			if r.IsDisabled {
				disabled++
				continue
			}
			if target == nil && r.DefaultBranch != "" && r.Size > 0 {
				target, targetProject = &r, p
			}
		}
	}
	reposMsg := fmt.Sprintf("%s in %d of %s", sa.Plural(repos, "repository", "repositories"), checked,
		sa.Plural(len(projects), "project", "projects"))
	if disabled > 0 {
		reposMsg += fmt.Sprintf("; %d disabled, skipped by the analysis", disabled)
	}
	if !add(checkRepositories, reposMsg, reposErr) {
		skipRest("the repositories could not be read", checkContents)
		return info
	}
	contentsOK := true
	if target == nil {
		skip(checkContents, "every repository the credential reads is empty or disabled")
	} else {
		var items list[json.RawMessage]
		q := url.Values{"scopePath": []string{"/"}, "recursionLevel": []string{"OneLevel"}}
		err := cl.get(ctx, "/"+url.PathEscape(targetProject.ID)+"/_apis/git/repositories/"+url.PathEscape(target.ID)+"/items", q, &items)
		contentsOK = add(checkContents, fmt.Sprintf("read the top-level items of %s/%s (%s)", targetProject.Name, target.Name,
			sa.Plural(len(items.Value), "entry", "entries")), err)
	}
	skip(sa.CallSourceAnalysis, sa.ReasonAnalysisUnavailable)
	info.OK = projectsOK && contentsOK
	c.logger.Info("Shadow agent discovery Azure DevOps connection test finished",
		"connectionId", conn.ID.String(), "ok", info.OK, "checks", len(info.Checks))
	return info
}

// projects lists the organization's projects the credential can read (the
// first page, up to 100), or reads each selected project.
func (c *Connector) projects(ctx context.Context, cl *client, conn sa.Connection) ([]project, string, error) {
	if conn.Config.AllProjects {
		var pl list[project]
		if err := cl.get(ctx, "/_apis/projects", nil, &pl); err != nil {
			return nil, "", err
		}
		return pl.Value, sa.Plural(len(pl.Value), "project", "projects"), nil
	}
	if len(conn.Config.Projects) == 0 {
		return nil, "", errors.New("no project is listed")
	}
	var found []project
	var missing []string
	for _, name := range conn.Config.Projects {
		var p project
		err := cl.get(ctx, "/_apis/projects/"+url.PathEscape(name), nil, &p)
		var ce *sa.CallError
		switch {
		case err == nil:
			found = append(found, p)
		case errors.As(err, &ce) && ce.Outcome == models.ShadowCallNotFound:
			missing = append(missing, name)
		default:
			return nil, "", err
		}
	}
	if len(missing) > 0 {
		return found, "", &sa.CallError{
			Outcome: models.ShadowCallNotFound,
			Message: fmt.Sprintf("%d of %d found; not found, or the credential cannot read: %s",
				len(found), len(conn.Config.Projects), strings.Join(missing, ", ")),
		}
	}
	return found, fmt.Sprintf("%d of %d listed projects", len(found), len(conn.Config.Projects)), nil
}
