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

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Limits of one source code scan publish.
const (
	maxShadowPublishAgents       = 1000
	maxShadowPublishEvidence     = 50
	maxShadowPublishRepositories = 5000
)

// Who confirmed a code agent (the reviewedBy attribute).
const (
	shadowReviewedByRules         = "rules"
	shadowReviewedByAIReview      = "ai_review"
	shadowReviewedByAIReviewJudge = "ai_review_judge"
)

// PublishRunnerScan saves what a source code scan command found for its
// pending scan and finishes the scan. The organization comes from the scan
// token. Every agent must be valid, or nothing is saved.
func (s *shadowAgentService) PublishRunnerScan(
	ctx context.Context, ouID string, scanID uuid.UUID, req *models.PublishShadowScanRequest,
) (*models.PublishShadowScanResponse, error) {
	if err := checkShadowPublishLimits(req); err != nil {
		return nil, err
	}
	scan, err := s.GetScan(ctx, ouID, scanID)
	if err != nil {
		return nil, err
	}
	if len(scan.ConnectionIDs) != 1 {
		return nil, fmt.Errorf("%w: not a source code scan", utils.ErrShadowScanNotFound)
	}
	connID, err := uuid.Parse(scan.ConnectionIDs[0])
	if err != nil {
		return nil, fmt.Errorf("%w: not a source code scan", utils.ErrShadowScanNotFound)
	}
	if err := repositories.ShadowScanNotPendingError(scan); err != nil {
		return nil, err
	}
	c, err := s.loadConnection(ctx, ouID, connID)
	if err != nil {
		return nil, err
	}
	if spec, ok := sa.Provider(c.Provider); !ok || !spec.SourceHost {
		return nil, fmt.Errorf("%w: not a source code scan", utils.ErrShadowScanNotFound)
	}
	out, err := shadowPublishFindings(c.Provider, scan.Options, req.Agents)
	if err != nil {
		return nil, err
	}

	org := c.Config.Organization
	scanned := normaliseShadowRepos(c.Provider, org, req.ScannedRepositories)
	failed := normaliseShadowFailedRepos(c.Provider, org, req.FailedRepositories)
	complete, unfinished := shadowPublishCoverage(c, req.Complete, scanned, failed)
	now := s.now().UTC()
	// A pending scan has no start time of its own: it started when the scan
	// command was created. Its evidence is dated then too, so a later scan of
	// the connection, which started after it, removes what it no longer sees
	// even when this one published last.
	apply := toShadowApply(scan, c, out, scan.CreatedAt, scan.CreatedAt, complete, s.logger)
	apply.UnfinishedScopes = unfinished
	result, status := shadowPublishOutcome(c, scanned, failed, complete)

	// The publish comes from a machine client with an organization's scan
	// token and changes the organization's agents, so it is recorded
	// fail-closed: nothing is saved unrecorded.
	attempt, err := audit.Begin(ctx, audit.ActionShadowScanPublish,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceShadowConnection, c.ID.String(), c.Name),
		audit.SurfaceOpt(audit.SurfacePublisher),
		audit.Actor(audit.ActorService, buildTokenClientID(ctx), ""),
		audit.AuthMethod("shadow-scan-token"),
		audit.Detail("provider", c.Provider),
		audit.Detail("scanId", scan.ID.String()),
		audit.Detail("agents", len(apply.Findings)),
		audit.Detail("scannedRepositories", len(scanned)),
		audit.Detail("failedRepositories", len(failed)),
		audit.Detail("complete", complete),
	)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.PublishScanResults(ctx, repositories.ShadowScanPublish{
		Apply: apply, Status: status, Result: result, FinishedAt: now,
		ExpireCreatedBy: now.Add(-shadowPendingScanLife),
	})
	attempt.Complete(ctx, err)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return nil, utils.ErrShadowScanNotFound
		case errors.Is(err, utils.ErrShadowScanNotPending), errors.Is(err, utils.ErrShadowScanSuperseded):
			return nil, err
		default:
			return nil, fmt.Errorf("failed to save the scan results: %w", err)
		}
	}
	// The results are saved; possible duplicates across connections are
	// recomputed as after a cloud scan, and a failure there is only logged.
	if err := correlateShadowOrg(context.WithoutCancel(ctx), s.repo, ouID, now); err != nil {
		s.logger.Error("Failed to correlate agents across connections", "ouID", ouID, "scanId", scan.ID, "error", err)
	}
	return &models.PublishShadowScanResponse{ScanID: scan.ID.String(), Status: status, FindingsCount: n}, nil
}

func checkShadowPublishLimits(req *models.PublishShadowScanRequest) error {
	switch {
	case len(req.Agents) > maxShadowPublishAgents:
		return fmt.Errorf("%w: at most %d agents can be published", utils.ErrInvalidInput, maxShadowPublishAgents)
	case len(req.ScannedRepositories) > maxShadowPublishRepositories:
		return fmt.Errorf("%w: at most %d scanned repositories can be published", utils.ErrInvalidInput,
			maxShadowPublishRepositories)
	case len(req.FailedRepositories) > maxShadowPublishRepositories:
		return fmt.Errorf("%w: at most %d failed repositories can be published", utils.ErrInvalidInput,
			maxShadowPublishRepositories)
	}
	for i := range req.Agents {
		if len(req.Agents[i].Evidence) > maxShadowPublishEvidence {
			return fmt.Errorf("%w: %s has more than %d pieces of evidence", utils.ErrInvalidInput,
				shadowAgentLabel(i, req.Agents[i].ExternalID), maxShadowPublishEvidence)
		}
	}
	return nil
}

// shadowPublishFindings checks every published agent and turns them into
// findings for toShadowApply, which applies the same retention rules as a
// cloud scan. An invalid agent fails the whole publish.
func shadowPublishFindings(
	provider string, opts models.ShadowScanOptions, agents []models.PublishShadowScanAgent,
) (*sa.Result, error) {
	out := &sa.Result{Findings: make([]*sa.Finding, 0, len(agents))}
	seen := make(map[string]bool, len(agents))
	for i := range agents {
		a := &agents[i]
		label := shadowAgentLabel(i, a.ExternalID)
		if strings.TrimSpace(a.ExternalID) == "" {
			return nil, fmt.Errorf("%w: %s has no externalId", utils.ErrInvalidInput, label)
		}
		// The key the agent is stored under (toShadowApply keeps 1024 runes).
		key := sa.Truncate(a.ExternalID, 1024)
		if seen[key] {
			return nil, fmt.Errorf("%w: %s repeats an externalId published earlier", utils.ErrInvalidInput, label)
		}
		seen[key] = true
		if strings.TrimSpace(a.Repository) == "" {
			return nil, fmt.Errorf("%w: %s has no repository", utils.ErrInvalidInput, label)
		}
		if len(a.Evidence) == 0 {
			return nil, fmt.Errorf("%w: %s has no evidence", utils.ErrInvalidInput, label)
		}
		if err := checkShadowReviewedBy(a.Attributes, opts); err != nil {
			return nil, fmt.Errorf("%w: %s %s", utils.ErrInvalidInput, label, err.Error())
		}
		f := &sa.Finding{
			Kind: models.ShadowKindCodeAgent, ExternalID: a.ExternalID, DisplayName: a.Name, Scope: a.Repository,
			Attributes: shadowPublishAttributes(a.Attributes),
		}
		for _, e := range a.Evidence {
			d, ok := sa.Def(e.Code)
			if !ok || d.Provider != provider {
				return nil, fmt.Errorf("%w: %s has evidence code %q, which is not a %s evidence code",
					utils.ErrInvalidInput, label, sa.Truncate(e.Code, 100), provider)
			}
			f.Signals = append(f.Signals, sa.Signal{Code: d.Code, Source: e.Source, Observed: e.Observed})
		}
		out.Findings = append(out.Findings, f)
	}
	return out, nil
}

// checkShadowReviewedBy requires who confirmed the agent, and AI review only
// when the scan was created with it.
func checkShadowReviewedBy(attrs map[string]any, opts models.ShadowScanOptions) error {
	v, _ := attrs["reviewedBy"].(string)
	switch v {
	case shadowReviewedByRules:
		return nil
	case shadowReviewedByAIReview:
		if !opts.AIReview {
			return errors.New("was confirmed by AI review, but the scan was created without AI review")
		}
		return nil
	case shadowReviewedByAIReviewJudge:
		if !opts.LLMJudge {
			return errors.New("was confirmed by the LLM judge, but the scan was created without the LLM judge")
		}
		return nil
	default:
		return errors.New("needs attributes.reviewedBy: rules, ai_review or ai_review_judge")
	}
}

// shadowPublishAttributes copies the attributes with the repository URL
// reduced to its scheme, host and path; toShadowApply applies the
// allow-list.
func shadowPublishAttributes(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	if raw, ok := out["repositoryUrl"].(string); ok {
		if u := sa.RepositoryURL(raw); u != "" {
			out["repositoryUrl"] = u
		} else {
			delete(out, "repositoryUrl")
		}
	}
	return out
}

func shadowAgentLabel(i int, externalID string) string {
	if id := strings.TrimSpace(externalID); id != "" {
		return fmt.Sprintf("agent %d (%q)", i, sa.Truncate(id, 100))
	}
	return fmt.Sprintf("agent %d", i)
}

// normaliseShadowRepos normalises repository names like finding scopes,
// dropping empty and repeated ones.
func normaliseShadowRepos(provider, org string, names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		n = sa.NormaliseScope(provider, org, n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func normaliseShadowFailedRepos(
	provider, org string, repos []models.PublishShadowScanFailedRepository,
) []models.ShadowScanFailedRepository {
	out := make([]models.ShadowScanFailedRepository, 0, len(repos))
	seen := map[string]bool{}
	for _, r := range repos {
		n := sa.NormaliseScope(provider, org, r.Name)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, models.ShadowScanFailedRepository{Name: n, Message: sa.ScrubText(r.Reason)})
	}
	return out
}

// shadowPublishCoverage decides which agents a publish may mark as gone.
// The listing is complete when the command listed every repository and,
// for Azure DevOps, reported every configured project and named no failed
// project without its repository. The unfinished scopes
// are the failed repositories and, for GitHub, the configured repositories
// the command did not report as scanned: their agents keep their presence.
func shadowPublishCoverage(
	c *models.ShadowConnection, listed bool, scanned []string, failed []models.ShadowScanFailedRepository,
) (bool, []string) {
	scannedSet := make(map[string]bool, len(scanned))
	for _, n := range scanned {
		scannedSet[n] = true
	}
	unfinished := make([]string, 0, len(failed))
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			unfinished = append(unfinished, n)
		}
	}
	for _, f := range failed {
		add(f.Name)
	}
	complete := listed
	switch c.Provider {
	case models.ShadowProviderGitHub:
		if !c.Config.AllRepositories {
			for _, r := range c.Config.Repositories {
				if n := sa.NormaliseScope(c.Provider, c.Config.Organization, r); n != "" && !scannedSet[n] {
					add(n)
				}
			}
		}
	case models.ShadowProviderAzureDevOps:
		for _, f := range failed {
			if !strings.Contains(f.Name, "/") {
				// A whole project failed: its repositories are unknown, so
				// its agents' scopes (project/repository) cannot be kept
				// apart. Nothing is marked gone.
				complete = false
			}
		}
		if !c.Config.AllProjects {
			for _, p := range c.Config.Projects {
				if !shadowProjectReported(strings.TrimSpace(p), scanned, failed) {
					// The command skipped a whole project: nothing is marked
					// gone, new agents are still saved.
					complete = false
				}
			}
		}
	}
	return complete, unfinished
}

// shadowProjectReported reports whether a scanned or failed Azure DevOps
// repository (project/repository) is in project.
func shadowProjectReported(project string, scanned []string, failed []models.ShadowScanFailedRepository) bool {
	in := func(repo string) bool {
		p, _, ok := strings.Cut(repo, "/")
		return ok && strings.EqualFold(p, project)
	}
	for _, n := range scanned {
		if in(n) {
			return true
		}
	}
	for _, f := range failed {
		if in(f.Name) {
			return true
		}
	}
	return false
}

// shadowPublishOutcome is the connection's result and the scan's status:
// failed when every repository failed, partial when some failed or the
// listing was not complete (no agent was marked gone), succeeded otherwise.
func shadowPublishOutcome(
	c *models.ShadowConnection, scanned []string, failed []models.ShadowScanFailedRepository, complete bool,
) (models.ShadowScanConnectionResult, string) {
	res := models.ShadowScanConnectionResult{
		ConnectionID: c.ID.String(), ConnectionName: c.Name, Provider: c.Provider,
		Outcome: shadowConnectionOK, Calls: []models.ShadowScanCallResult{},
		ScannedRepositories: scanned, FailedRepositories: failed,
	}
	if len(failed) > 0 && len(scanned) == 0 {
		res.Outcome = shadowConnectionFailed
		res.Message = sa.Plural(len(failed), "repository", "repositories") + " could not be scanned"
		return res, models.ShadowScanFailed
	}
	var notes []string
	if len(failed) > 0 {
		notes = append(notes, fmt.Sprintf("%d of %d repositories could not be scanned", len(failed), len(failed)+len(scanned)))
	}
	if !complete {
		notes = append(notes, "could not list every repository; no agents were marked gone")
	}
	if len(notes) == 0 {
		return res, models.ShadowScanSucceeded
	}
	res.Outcome, res.Message = shadowConnectionPartial, strings.Join(notes, "; ")
	return res, models.ShadowScanPartial
}
