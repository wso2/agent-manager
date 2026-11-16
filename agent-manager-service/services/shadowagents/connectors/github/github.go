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

// Package github is the GitHub source host of Shadow Agent Discovery. Its
// connection test checks, with read-only calls, that the fine-grained token
// reaches the owner's repositories and can read their contents. The source
// code analysis that scans the repositories is added separately; until then
// a scan records it as skipped.
package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// apiVersion is the GitHub REST API version the requests are written for.
const apiVersion = "2022-11-28"

// maxSelectedChecked bounds the selected repositories a test reads.
const maxSelectedChecked = 20

// Connection check names.
const (
	checkOwner        = "Organization or user account (GET /users/{owner})"
	checkRepositories = "Repositories the token can read (Metadata: read)"
	checkContents     = "Repository contents (Contents: read)"
)

// Connector is the GitHub source host.
type Connector struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// New returns the GitHub connector. httpClient is the redirect-guarded
// client (sa.NewHTTPClient); logger receives sanitized summaries only.
func New(httpClient *http.Client, logger *slog.Logger) sa.Connector {
	if httpClient == nil {
		httpClient = sa.NewHTTPClient(nil)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Connector{httpClient: httpClient, logger: logger}
}

// Provider returns models.ShadowProviderGitHub.
func (c *Connector) Provider() string { return models.ShadowProviderGitHub }

// Scan records the source code analysis as not available yet. No
// repository is read.
func (c *Connector) Scan(_ context.Context, _ sa.Connection) *sa.Result {
	calls := sa.NewCalls()
	calls.Skip(sa.CallSourceAnalysis, sa.ReasonAnalysisUnavailable)
	return &sa.Result{Calls: calls.Results()}
}

// repo is the part of a GitHub repository object the test reads.
type repo struct {
	Name     string `json:"name"`
	Private  bool   `json:"private"`
	Fork     bool   `json:"fork"`
	Archived bool   `json:"archived"`
	Size     int    `json:"size"`
}

type account struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// Test checks the token with read-only calls: the owner exists, the token
// lists (or reads each selected) repository, and it can read the contents
// of one of them. Nothing a call returns is stored beyond counts.
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
	base, err := sa.Expand(sa.EndpointOf(conn, sa.EPGitHubAPI), nil)
	if err == nil && base == "" {
		err = errors.New("the GitHub API endpoint is not set")
	}
	if err != nil {
		add(checkOwner, "", err)
		return info
	}
	base = strings.TrimRight(base, "/")
	token := strings.TrimSpace(conn.Secret.PersonalAccessToken)
	if token == "" {
		add(checkOwner, "", &sa.CallError{Outcome: models.ShadowCallAuthFailed, Message: "no personal access token is stored"})
		return info
	}
	api := &sa.API{
		HTTP:   c.httpClient,
		Logger: c.logger,
		Header: func(h http.Header) {
			h.Set("Accept", "application/vnd.github+json")
			h.Set("X-GitHub-Api-Version", apiVersion)
			h.Set("Authorization", "Bearer "+token)
		},
	}
	owner := strings.TrimSpace(conn.Config.Organization)

	var acct account
	ownerErr := api.GetJSON(ctx, base+"/users/"+url.PathEscape(owner), nil, &acct)
	if !add(checkOwner, ownerMessage(acct), ownerErr) {
		skip(checkRepositories, "the account could not be read")
		skip(checkContents, "the account could not be read")
		skip(sa.CallSourceAnalysis, sa.ReasonAnalysisUnavailable)
		return info
	}

	var repos []repo
	var reposMsg string
	var reposErr error
	if conn.Config.AllRepositories {
		repos, reposMsg, reposErr = listRepositories(ctx, api, base, acct)
	} else {
		repos, reposMsg, reposErr = readSelected(ctx, api, base, owner, conn.Config.Repositories)
	}
	reposOK := add(checkRepositories, reposMsg, reposErr) && len(repos) > 0
	if reposErr == nil && len(repos) == 0 {
		info.Checks[len(info.Checks)-1].Outcome = models.ShadowCallNotFound
	}

	// The contents check runs on any repository found, even when some
	// selected ones are missing, so the test says what does work.
	contentsOK := false
	target := firstReadable(repos)
	switch {
	case len(repos) == 0:
		skip(checkContents, "no repository to read")
	case target == nil:
		skip(checkContents, "every repository the token reads is empty")
		contentsOK = true
	default:
		var entries []map[string]any
		err := api.GetJSON(ctx, base+"/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(target.Name)+"/contents/", nil, &entries)
		contentsOK = add(checkContents, fmt.Sprintf("read the top-level file list of %s (%s)", target.Name, sa.Plural(len(entries), "entry", "entries")), err)
	}
	skip(sa.CallSourceAnalysis, sa.ReasonAnalysisUnavailable)
	info.OK = reposOK && contentsOK
	c.logger.Info("Shadow agent discovery GitHub connection test finished",
		"connectionId", conn.ID.String(), "ok", info.OK, "checks", len(info.Checks))
	return info
}

func ownerMessage(a account) string {
	switch a.Type {
	case "Organization":
		return "organization " + a.Login
	case "User":
		return "user account " + a.Login
	default:
		return a.Login
	}
}

// listRepositories lists the owner's repositories the token can read (up to
// 100, enough for a test). For a user account it lists the token owner's own
// repositories, private ones included, when the account is the token's;
// otherwise only the account's public repositories are visible.
func listRepositories(ctx context.Context, api *sa.API, base string, acct account) ([]repo, string, error) {
	q := url.Values{"per_page": []string{"100"}}
	path := "/orgs/" + url.PathEscape(acct.Login) + "/repos"
	note := ""
	if acct.Type == "User" {
		var me account
		if err := api.GetJSON(ctx, base+"/user", nil, &me); err != nil {
			return nil, "", err
		}
		if strings.EqualFold(me.Login, acct.Login) {
			path = "/user/repos"
			q.Set("affiliation", "owner")
		} else {
			path = "/users/" + url.PathEscape(acct.Login) + "/repos"
			note = "; the token belongs to " + me.Login + ", so only public repositories of " + acct.Login + " are visible"
		}
	} else {
		q.Set("type", "all")
	}
	var repos []repo
	if err := api.GetJSON(ctx, base+path, q, &repos); err != nil {
		return nil, "", err
	}
	return repos, describe(repos, len(repos) == 100) + note, nil
}

// readSelected reads each selected repository (up to maxSelectedChecked).
func readSelected(ctx context.Context, api *sa.API, base, owner string, names []string) ([]repo, string, error) {
	if len(names) == 0 {
		return nil, "", errors.New("no repository is listed")
	}
	var found []repo
	var missing []string
	for i, name := range names {
		if i == maxSelectedChecked {
			break
		}
		var r repo
		err := api.GetJSON(ctx, base+"/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), nil, &r)
		var ce *sa.CallError
		switch {
		case err == nil:
			found = append(found, r)
		case errors.As(err, &ce) && ce.Outcome == models.ShadowCallNotFound:
			missing = append(missing, name)
		default:
			return nil, "", err
		}
	}
	if len(missing) > 0 {
		return found, "", &sa.CallError{
			Outcome: models.ShadowCallNotFound,
			Message: fmt.Sprintf("%d of %d found; not found, or the token cannot read: %s",
				len(found), len(found)+len(missing), strings.Join(missing, ", ")),
		}
	}
	msg := describe(found, false)
	if len(names) > maxSelectedChecked {
		msg += fmt.Sprintf(" (checked the first %d of %d)", maxSelectedChecked, len(names))
	}
	return found, msg, nil
}

func describe(repos []repo, more bool) string {
	private, skipped := 0, 0
	for _, r := range repos {
		if r.Private {
			private++
		}
		if r.Fork || r.Archived || r.Size == 0 {
			skipped++
		}
	}
	msg := sa.Plural(len(repos), "repository", "repositories")
	if more {
		msg = fmt.Sprintf("%d+ repositories", len(repos))
	}
	msg += fmt.Sprintf(" (%d private)", private)
	if skipped > 0 {
		msg += fmt.Sprintf("; %d skipped by the analysis (fork, archived or empty)", skipped)
	}
	return msg
}

// firstReadable is the first repository the analysis would read: not a
// fork, not archived, not empty.
func firstReadable(repos []repo) *repo {
	for i := range repos {
		if !repos[i].Fork && !repos[i].Archived && repos[i].Size > 0 {
			return &repos[i]
		}
	}
	return nil
}
