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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var shadowPublishCreatedAt = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// publishFixture is a pending scan of one connection and a repository that
// holds them and records the publish.
type publishFixture struct {
	conn      models.ShadowConnection
	scan      models.ShadowScan
	repo      *repomocks.ShadowAgentRepositoryMock
	published *repositories.ShadowScanPublish
}

func newPublishFixture(provider string, cfg models.ShadowConnectionConfig, opts models.ShadowScanOptions) *publishFixture {
	f := &publishFixture{conn: models.ShadowConnection{
		ID: uuid.New(), OUID: shadowTestOU, Name: "repos", Provider: provider, Config: cfg,
	}}
	f.scan = models.ShadowScan{
		ID: uuid.New(), OUID: shadowTestOU, Status: models.ShadowScanPending,
		ConnectionIDs: []string{f.conn.ID.String()}, Options: opts, CreatedAt: shadowPublishCreatedAt,
	}
	f.repo = shadowRepoWith(f.conn)
	f.repo.GetScanFunc = func(_ context.Context, ouID string, id uuid.UUID) (*models.ShadowScan, error) {
		if ouID != shadowTestOU || id != f.scan.ID {
			return nil, gorm.ErrRecordNotFound
		}
		s := f.scan
		return &s, nil
	}
	f.repo.PublishScanResultsFunc = func(_ context.Context, in repositories.ShadowScanPublish) (int, error) {
		f.published = &in
		return len(in.Apply.Findings), nil
	}
	f.repo.ListOrgFindingsFunc = func(context.Context, string) ([]models.ShadowFinding, error) {
		return []models.ShadowFinding{}, nil
	}
	f.repo.ApplyCorrelationFunc = func(context.Context, string, map[uuid.UUID][]models.ShadowFindingRelation,
		map[uuid.UUID][]models.ShadowFindingSignal,
	) error {
		return nil
	}
	return f
}

func (f *publishFixture) publish(ctx context.Context, req *models.PublishShadowScanRequest) (*models.PublishShadowScanResponse, error) {
	return newPublishSvc(f.repo).PublishRunnerScan(ctx, shadowTestOU, f.scan.ID, req)
}

func (f *publishFixture) upload(ctx context.Context, req *models.PublishShadowScanRequest) (*models.PublishShadowScanResponse, error) {
	return newPublishSvc(f.repo).UploadRunnerScanResults(ctx, shadowTestOU, f.scan.ID, req)
}

// newPublishSvc is a service with source code scans turned on. Publishing
// needs no token issuer.
func newPublishSvc(repo *repomocks.ShadowAgentRepositoryMock) ShadowAgentService {
	return newRunnerShadowSvc(repo, shadowRunner(nil))
}

func codeAgent(id, repo, code string) models.PublishShadowScanAgent {
	return models.PublishShadowScanAgent{
		ExternalID: id, Name: id, Repository: repo,
		Attributes: map[string]any{"reviewedBy": "rules"},
		Evidence:   []models.PublishShadowScanSignal{{Code: code, Source: "requirements.txt"}},
	}
}

func TestShadowPublishRunnerScan_GitHubSavesAgentsAndUnfinishedRepos(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub,
		models.ShadowConnectionConfig{Organization: "acme", Repositories: []string{"Agent-A", "b", "c"}},
		models.ShadowScanOptions{})
	agent := codeAgent("acme/agent-a:src/agent.py", "Agent-A", "github.d1_agent_dependency")
	agent.Attributes["repositoryUrl"] = "https://user:pw@github.com/acme/Agent-A?token=x"
	agent.Attributes["snippet"] = shadowPromptMark
	agent.Evidence[0].Observed = map[string]any{"frameworks": float64(2), "prompt": shadowPromptMark + strings.Repeat("x", 80)}
	rec := &recordingAuditor{}

	resp, err := f.publish(audit.WithRecorder(context.Background(), rec), &models.PublishShadowScanRequest{
		Complete:            true,
		ScannedRepositories: []string{"agent-a", " B ", "agent-a"},
		FailedRepositories:  []models.PublishShadowScanFailedRepository{{Name: "Other", Reason: "clone failed: token=SECRET"}},
		Agents:              []models.PublishShadowScanAgent{agent},
	})
	require.NoError(t, err)
	require.NotNil(t, f.published)

	ap := f.published.Apply
	assert.Equal(t, f.scan.ID, ap.ScanID)
	assert.Equal(t, f.conn.ID, ap.ConnectionID)
	assert.True(t, ap.ScanStartedAt.Equal(shadowPublishCreatedAt), "a pending scan started when it was created")
	assert.True(t, ap.Complete)
	assert.ElementsMatch(t, []string{"other", "c"}, ap.UnfinishedScopes,
		"the failed repository and the configured one the command did not scan")
	require.Len(t, ap.Findings, 1)
	fi := ap.Findings[0]
	assert.Equal(t, models.ShadowKindCodeAgent, fi.Finding.Kind)
	assert.Equal(t, models.ShadowProviderGitHub, fi.Finding.Provider)
	assert.Equal(t, "agent-a", fi.Finding.Scope, "GitHub scopes are lower-cased")
	assert.Equal(t, map[string]any{"reviewedBy": "rules", "repositoryUrl": "https://github.com/acme/Agent-A"},
		fi.Finding.Attributes, "allow-listed attributes, the URL without credentials")
	require.Len(t, fi.Signals, 1)
	assert.NotContains(t, fi.Signals[0].Observed, "prompt")
	assert.True(t, fi.Signals[0].DetectedAt.Equal(shadowPublishCreatedAt),
		"evidence is dated when the scan was created, so a later scan can remove it")
	assert.WithinDuration(t, time.Now().Add(-shadowScanTokenLife), f.published.TokenExpiredCreatedBy, time.Minute,
		"a scan older than the life of its token is refused")

	assert.Equal(t, models.ShadowScanPartial, f.published.Status)
	assert.Equal(t, "partial", f.published.Result.Outcome)
	assert.Equal(t, []string{"agent-a", "b"}, f.published.Result.ScannedRepositories)
	require.Len(t, f.published.Result.FailedRepositories, 1)
	assert.Equal(t, "other", f.published.Result.FailedRepositories[0].Name)
	assert.NotContains(t, f.published.Result.FailedRepositories[0].Message, "SECRET", "reasons are scrubbed")
	assert.Equal(t, &models.PublishShadowScanResponse{ScanID: f.scan.ID.String(), Status: models.ShadowScanPartial, FindingsCount: 1}, resp)

	require.Len(t, rec.events, 2, "intent and outcome")
	assert.Equal(t, audit.ActionShadowScanPublish, rec.events[0].Action)
	assert.Equal(t, audit.OutcomeUnknown, rec.events[0].Outcome)
	assert.Equal(t, audit.OutcomeSuccess, rec.events[1].Outcome)
	assert.Len(t, f.repo.ApplyCorrelationCalls(), 1, "agents are correlated after the publish")
}

func TestShadowPublishRunnerScan_AllRepositoriesLeavesOnlyFailedUnfinished(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub,
		models.ShadowConnectionConfig{Organization: "acme", AllRepositories: true, Repositories: []string{"stale"}},
		models.ShadowScanOptions{})
	_, err := f.publish(auditableCtx(t), &models.PublishShadowScanRequest{
		Complete: true, ScannedRepositories: []string{"a"},
	})
	require.NoError(t, err)
	assert.Empty(t, f.published.Apply.UnfinishedScopes)
	assert.True(t, f.published.Apply.Complete)
	assert.Equal(t, models.ShadowScanSucceeded, f.published.Status)
	assert.Equal(t, "ok", f.published.Result.Outcome)
}

func TestShadowPublishRunnerScan_IncompleteListing(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{AllRepositories: true},
		models.ShadowScanOptions{})
	resp, err := f.publish(auditableCtx(t), &models.PublishShadowScanRequest{Complete: false, ScannedRepositories: []string{"a"}})
	require.NoError(t, err)
	assert.False(t, f.published.Apply.Complete, "a command that could not list every repository marks nothing gone")
	assert.Equal(t, models.ShadowScanPartial, resp.Status, "an incomplete listing is not a success")
	assert.Equal(t, "partial", f.published.Result.Outcome)
	assert.Equal(t, "could not list every repository; no agents were marked gone", f.published.Result.Message)

	// With failed repositories too, both are said.
	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{AllRepositories: true},
		models.ShadowScanOptions{})
	_, err = f.publish(auditableCtx(t), &models.PublishShadowScanRequest{
		ScannedRepositories: []string{"a"},
		FailedRepositories:  []models.PublishShadowScanFailedRepository{{Name: "b", Reason: "x"}},
	})
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanPartial, f.published.Status)
	assert.Equal(t, "1 of 2 repositories could not be scanned; could not list every repository; no agents were marked gone",
		f.published.Result.Message)
}

func TestShadowPublishRunnerScan_GitHubOrgPrefixIsTheSameRepository(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub,
		models.ShadowConnectionConfig{Organization: "Acme", Repositories: []string{"one", "two", "three"}},
		models.ShadowScanOptions{})
	_, err := f.publish(auditableCtx(t), &models.PublishShadowScanRequest{
		Complete:            true,
		ScannedRepositories: []string{"acme/One", "two"},
		FailedRepositories:  []models.PublishShadowScanFailedRepository{{Name: "ACME/three", Reason: "x"}},
		Agents:              []models.PublishShadowScanAgent{codeAgent("a", "Acme/One", "github.d1_agent_dependency")},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, f.published.Result.ScannedRepositories)
	assert.Equal(t, []string{"three"}, f.published.Apply.UnfinishedScopes, "org/repo and repo are the same repository")
	require.Len(t, f.published.Apply.Findings, 1)
	assert.Equal(t, "one", f.published.Apply.Findings[0].Finding.Scope, "the finding scope matches the lists")
	assert.True(t, f.published.Apply.Complete)
}

func TestShadowPublishRunnerScan_EveryRepositoryFailed(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{AllRepositories: true},
		models.ShadowScanOptions{})
	resp, err := f.publish(auditableCtx(t), &models.PublishShadowScanRequest{
		Complete:           true,
		FailedRepositories: []models.PublishShadowScanFailedRepository{{Name: "a", Reason: "x"}, {Name: "b", Reason: "y"}},
	})
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanFailed, resp.Status)
	assert.Equal(t, "failed", f.published.Result.Outcome)
	assert.NotEqual(t, models.ShadowScanExpiredMessage, f.published.Result.Message)
}

func TestShadowPublishRunnerScan_RejectsInvalidAgents(t *testing.T) {
	withAttrs := func(a models.PublishShadowScanAgent, attrs map[string]any) models.PublishShadowScanAgent {
		a.Attributes = attrs
		return a
	}
	good := codeAgent("acme/a:agent.py", "a", "github.d1_agent_dependency")
	noEvidence := good
	noEvidence.Evidence = nil
	noID := good
	noID.ExternalID = " "
	noRepo := good
	noRepo.Repository = ""
	unknownCode := codeAgent("x", "a", "github.d99_unknown")
	otherProvider := codeAgent("x", "a", "openai.agent_object")
	tooMuchEvidence := good
	tooMuchEvidence.Evidence = make([]models.PublishShadowScanSignal, maxShadowPublishEvidence+1)

	cases := []struct {
		name string
		opts models.ShadowScanOptions
		req  models.PublishShadowScanRequest
	}{
		{"empty externalId", models.ShadowScanOptions{}, models.PublishShadowScanRequest{Agents: []models.PublishShadowScanAgent{noID}}},
		{"no evidence", models.ShadowScanOptions{}, models.PublishShadowScanRequest{Agents: []models.PublishShadowScanAgent{noEvidence}}},
		{"empty repository", models.ShadowScanOptions{}, models.PublishShadowScanRequest{Agents: []models.PublishShadowScanAgent{noRepo}}},
		{"unknown evidence code", models.ShadowScanOptions{}, models.PublishShadowScanRequest{Agents: []models.PublishShadowScanAgent{unknownCode}}},
		{"another provider's code", models.ShadowScanOptions{}, models.PublishShadowScanRequest{Agents: []models.PublishShadowScanAgent{otherProvider}}},
		{"reviewedBy missing", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			Agents: []models.PublishShadowScanAgent{withAttrs(good, map[string]any{})},
		}},
		{"reviewedBy unknown", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			Agents: []models.PublishShadowScanAgent{withAttrs(good, map[string]any{"reviewedBy": "human"})},
		}},
		{"reviewedBy not text", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			Agents: []models.PublishShadowScanAgent{withAttrs(good, map[string]any{"reviewedBy": true})},
		}},
		{"AI review not chosen", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			Agents: []models.PublishShadowScanAgent{withAttrs(good, map[string]any{"reviewedBy": "ai_review"})},
		}},
		{"LLM judge not chosen", models.ShadowScanOptions{AIReview: true}, models.PublishShadowScanRequest{
			Agents: []models.PublishShadowScanAgent{withAttrs(good, map[string]any{"reviewedBy": "ai_review_judge"})},
		}},
		{"a valid agent does not save an invalid one", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			Agents: []models.PublishShadowScanAgent{good, noEvidence},
		}},
		{"repeated externalId", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			Agents: []models.PublishShadowScanAgent{good, good},
		}},
		{"too many agents", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			Agents: make([]models.PublishShadowScanAgent, maxShadowPublishAgents+1),
		}},
		{"too much evidence", models.ShadowScanOptions{}, models.PublishShadowScanRequest{Agents: []models.PublishShadowScanAgent{tooMuchEvidence}}},
		{"too many scanned repositories", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			ScannedRepositories: make([]string, maxShadowPublishRepositories+1),
		}},
		{"too many failed repositories", models.ShadowScanOptions{}, models.PublishShadowScanRequest{
			FailedRepositories: make([]models.PublishShadowScanFailedRepository, maxShadowPublishRepositories+1),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{AllRepositories: true}, tc.opts)
			f.repo.PublishScanResultsFunc = nil // reaching the repository panics
			_, err := f.publish(auditableCtx(t), &tc.req)
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			if tc.name == "repeated externalId" {
				assert.Contains(t, err.Error(), `agent 1 ("acme/a:agent.py") repeats an externalId`)
			}
		})
	}
}

func TestShadowPublishRunnerScan_AcceptsAIReviewWhenChosen(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{AllRepositories: true},
		models.ShadowScanOptions{AIReview: true, LLMJudge: true})
	a := codeAgent("x", "a", "github.d7_tool_calling")
	b := codeAgent("y", "a", "github.d7_tool_calling")
	a.Attributes["reviewedBy"], b.Attributes["reviewedBy"] = "ai_review", "ai_review_judge"
	_, err := f.publish(auditableCtx(t), &models.PublishShadowScanRequest{
		Complete: true, ScannedRepositories: []string{"a"}, Agents: []models.PublishShadowScanAgent{a, b},
	})
	require.NoError(t, err)
	assert.Len(t, f.published.Apply.Findings, 2)
}

func TestShadowPublishRunnerScan_NotFound(t *testing.T) {
	req := &models.PublishShadowScanRequest{Complete: true}

	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	_, err := newPublishSvc(f.repo).PublishRunnerScan(auditableCtx(t), "ou-other", f.scan.ID, req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "another org's scan is not found")

	f.repo.GetScanFunc = func(context.Context, string, uuid.UUID) (*models.ShadowScan, error) {
		return nil, errors.New("db down")
	}
	_, err = f.publish(auditableCtx(t), req)
	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrShadowScanNotFound, "a real error is not masked as not found")

	f = newPublishFixture(models.ShadowProviderOpenAI, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	_, err = f.publish(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "a cloud platform has no source code scan")

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.scan.ConnectionIDs = []string{f.conn.ID.String(), uuid.NewString()}
	_, err = f.publish(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "a scan of several connections is not a source code scan")

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.scan.ConnectionIDs = []string{uuid.NewString()}
	_, err = f.publish(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowConnectionNotFound, "the connection was deleted")

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.PublishScanResultsFunc = func(context.Context, repositories.ShadowScanPublish) (int, error) {
		return 0, gorm.ErrRecordNotFound
	}
	_, err = f.publish(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "deleted while publishing")
}

func TestShadowPublishRunnerScan_Conflicts(t *testing.T) {
	req := &models.PublishShadowScanRequest{Complete: true}

	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.scan.Status = models.ShadowScanSucceeded
	f.repo.PublishScanResultsFunc = nil
	_, err := f.publish(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotPending)
	assert.Contains(t, err.Error(), "already published")

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.scan.Status = models.ShadowScanFailed
	f.scan.Results = []models.ShadowScanConnectionResult{{Outcome: "failed", Message: models.ShadowScanExpiredMessage}}
	f.repo.PublishScanResultsFunc = nil
	_, err = f.publish(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotPending)
	assert.Contains(t, err.Error(), models.ShadowScanExpiredMessage)

	for _, repoErr := range []error{
		utils.ErrShadowScanSuperseded,
		utils.ErrShadowScanTokenExpired,
		fmt.Errorf("%w: already published", utils.ErrShadowScanNotPending),
	} {
		f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
		f.repo.PublishScanResultsFunc = func(context.Context, repositories.ShadowScanPublish) (int, error) {
			return 0, repoErr
		}
		_, err = f.publish(auditableCtx(t), req)
		require.ErrorIs(t, err, repoErr)
	}
}

// The console upload saves exactly what the scan publisher saves, with no
// scan token time limit, and is recorded as the signed-in user's.
func TestShadowUploadRunnerScanResults_SavesLikeAPublish(t *testing.T) {
	cfg := models.ShadowConnectionConfig{Organization: "acme", Repositories: []string{"a", "b"}}
	req := &models.PublishShadowScanRequest{
		Complete:            true,
		ScannedRepositories: []string{"a"},
		FailedRepositories:  []models.PublishShadowScanFailedRepository{{Name: "B", Reason: "x"}},
		Agents:              []models.PublishShadowScanAgent{codeAgent("acme/a:agent.py", "a", "github.d1_agent_dependency")},
	}
	published := newPublishFixture(models.ShadowProviderGitHub, cfg, models.ShadowScanOptions{})
	want, err := published.publish(auditableCtx(t), req)
	require.NoError(t, err)

	f := newPublishFixture(models.ShadowProviderGitHub, cfg, models.ShadowScanOptions{})
	f.scan.ID = published.scan.ID
	f.scan.CreatedAt = time.Now().Add(-30 * 24 * time.Hour) // pending scans wait for their results
	rec := &recordingAuditor{}
	got, err := f.upload(audit.WithRecorder(context.Background(), rec), req)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.True(t, f.published.TokenExpiredCreatedBy.IsZero(), "an upload is not limited by a scan token's life")
	assert.Equal(t, published.published.Apply.UnfinishedScopes, f.published.Apply.UnfinishedScopes)
	wantResult := published.published.Result
	wantResult.ConnectionID = f.conn.ID.String()
	assert.Equal(t, wantResult, f.published.Result)
	assert.Equal(t, published.published.Status, f.published.Status)
	assert.Len(t, f.published.Apply.Findings, 1)

	require.Len(t, rec.events, 2, "intent and outcome, fail-closed")
	assert.Equal(t, audit.ActionShadowScanPublish, rec.events[0].Action)
	assert.NotEqual(t, audit.ActorService, rec.events[0].ActorType, "the signed-in user, not a scan token")
	assert.NotEqual(t, audit.SurfacePublisher, rec.events[0].Surface)
	assert.Equal(t, audit.OutcomeSuccess, rec.events[1].Outcome)
}

func TestShadowUploadRunnerScanResults_Refusals(t *testing.T) {
	req := &models.PublishShadowScanRequest{Complete: true}

	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.PublishScanResultsFunc = nil // reaching the repository panics
	_, err := newPublishSvc(f.repo).UploadRunnerScanResults(auditableCtx(t), "ou-other", f.scan.ID, req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "another org's scan is not found")

	f = newPublishFixture(models.ShadowProviderOpenAI, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.PublishScanResultsFunc = nil
	_, err = f.upload(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "a cloud platform has no source code scan")

	for _, msg := range []string{models.ShadowScanCancelledMessage, models.ShadowScanExpiredMessage} {
		f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
		f.scan.Status = models.ShadowScanFailed
		f.scan.Results = []models.ShadowScanConnectionResult{{Outcome: "failed", Message: msg}}
		f.repo.PublishScanResultsFunc = nil
		_, err = f.upload(auditableCtx(t), req)
		require.ErrorIs(t, err, utils.ErrShadowScanNotPending)
		assert.Contains(t, err.Error(), msg)
	}

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.PublishScanResultsFunc = func(context.Context, repositories.ShadowScanPublish) (int, error) {
		return 0, utils.ErrShadowScanSuperseded
	}
	_, err = f.upload(auditableCtx(t), req)
	require.ErrorIs(t, err, utils.ErrShadowScanSuperseded, "an older scan uploading after a newer one published")

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.PublishScanResultsFunc = nil
	_, err = f.upload(auditableCtx(t), &models.PublishShadowScanRequest{
		Agents: make([]models.PublishShadowScanAgent, maxShadowPublishAgents+1),
	})
	require.ErrorIs(t, err, utils.ErrInvalidInput, "the publish limits apply")

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.GetScanFunc = nil
	_, err = newShadowSvc(f.repo).UploadRunnerScanResults(auditableCtx(t), shadowTestOU, f.scan.ID, req)
	require.ErrorIs(t, err, ErrShadowScanPublishDisabled)

	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.PublishScanResultsFunc = nil
	_, err = f.upload(context.Background(), req)
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
}

func TestShadowCancelRunnerScan(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.CancelPendingScanFunc = func(_ context.Context, ouID string, id uuid.UUID, _ time.Time) (bool, error) {
		assert.Equal(t, shadowTestOU, ouID)
		assert.Equal(t, f.scan.ID, id)
		f.scan.Status = models.ShadowScanFailed
		f.scan.Results = []models.ShadowScanConnectionResult{{Outcome: "failed", Message: models.ShadowScanCancelledMessage}}
		return true, nil
	}
	got, err := newPublishSvc(f.repo).CancelRunnerScan(context.Background(), shadowTestOU, f.scan.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanFailed, got.Status)
	require.Len(t, f.repo.CancelPendingScanCalls(), 1)

	// Cancelled already, or published: refused before any change.
	f.repo.CancelPendingScanFunc = nil
	_, err = newPublishSvc(f.repo).CancelRunnerScan(context.Background(), shadowTestOU, f.scan.ID)
	require.ErrorIs(t, err, utils.ErrShadowScanNotPending)
	assert.Contains(t, err.Error(), models.ShadowScanCancelledMessage)

	_, err = newPublishSvc(f.repo).CancelRunnerScan(context.Background(), "ou-other", f.scan.ID)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "another org's scan is not found")

	// Its results arrived between the read and the cancel.
	f = newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.CancelPendingScanFunc = func(context.Context, string, uuid.UUID, time.Time) (bool, error) {
		f.scan.Status = models.ShadowScanSucceeded
		return false, nil
	}
	_, err = newPublishSvc(f.repo).CancelRunnerScan(context.Background(), shadowTestOU, f.scan.ID)
	require.ErrorIs(t, err, utils.ErrShadowScanNotPending)
	assert.Contains(t, err.Error(), "already published")
}

// While source code scans are turned off, publishing is refused before the
// scan is read or anything is saved.
func TestShadowPublishRunnerScan_TurnedOff(t *testing.T) {
	require.ErrorIs(t, ErrShadowScanPublishDisabled, utils.ErrNotFound)
	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.GetScanFunc = nil // reaching the repository panics
	f.repo.PublishScanResultsFunc = nil
	req := &models.PublishShadowScanRequest{Complete: true, Agents: []models.PublishShadowScanAgent{
		codeAgent("a", "repo-a", "github.d1_agent_dependency"),
	}}

	for name, svc := range map[string]ShadowAgentService{
		"turned off": newRunnerShadowSvc(f.repo, NewShadowScanRunner(config.Config{}, discardLogger())),
		"zero value": newShadowSvc(f.repo),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.PublishRunnerScan(auditableCtx(t), shadowTestOU, f.scan.ID, req)
			require.ErrorIs(t, err, ErrShadowScanPublishDisabled)
			assert.Nil(t, f.published, "nothing is saved")
		})
	}
}

func TestShadowPublishRunnerScan_RefusesWithoutAuditTrail(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.PublishScanResultsFunc = nil // reaching the repository panics
	_, err := f.publish(context.Background(), &models.PublishShadowScanRequest{Complete: true})
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
}

func TestShadowPublishRunnerScan_CorrelationFailureIsNotFatal(t *testing.T) {
	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{}, models.ShadowScanOptions{})
	f.repo.ListOrgFindingsFunc = func(context.Context, string) ([]models.ShadowFinding, error) {
		return nil, errors.New("db down")
	}
	_, err := f.publish(auditableCtx(t), &models.PublishShadowScanRequest{Complete: true})
	require.NoError(t, err)
}

func TestToShadowApply_NormalisesGitHubScopes(t *testing.T) {
	out := &sa.Result{Findings: []*sa.Finding{{
		Kind: models.ShadowKindCodeAgent, ExternalID: "x", Scope: "  My-Repo ",
		Signals: []sa.Signal{{Code: "github.d2_agent_import"}},
	}}}
	scan := &models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU}
	now := time.Now().UTC()
	gh := toShadowApply(scan, &models.ShadowConnection{ID: uuid.New(), Provider: models.ShadowProviderGitHub},
		out, now, now, true, slog.New(slog.DiscardHandler))
	require.Len(t, gh.Findings, 1)
	assert.Equal(t, "my-repo", gh.Findings[0].Finding.Scope)

	out.Findings[0].Scope = "ACME/My-Repo"
	withOrg := &models.ShadowConnection{
		ID: uuid.New(), Provider: models.ShadowProviderGitHub, Config: models.ShadowConnectionConfig{Organization: "acme"},
	}
	gh = toShadowApply(scan, withOrg, out, now, now, true, slog.New(slog.DiscardHandler))
	assert.Equal(t, "my-repo", gh.Findings[0].Finding.Scope, "the connection's org prefix is dropped")
	out.Findings[0].Scope = "other/My-Repo"
	gh = toShadowApply(scan, withOrg, out, now, now, true, slog.New(slog.DiscardHandler))
	assert.Equal(t, "other/my-repo", gh.Findings[0].Finding.Scope, "another owner is kept")
}

// Attributes a scan command publishes arrive decoded from JSON; lists and
// objects are stored in the same typed forms the cloud connectors store.
func TestShadowPublishRunnerScan_StoresListAndMapAttributes(t *testing.T) {
	body := `{"complete":true,"scannedRepositories":["a"],"agents":[{
		"externalId":"x","name":"agent","repository":"a",
		"attributes":{
			"reviewedBy":"rules",
			"tools":[{"type":"function","name":"search","schema":{"secret":"` + shadowPromptMark + `"}},{"type":"mcp"}],
			"mcpServers":[{"label":"files","origin":"https://user:pw@mcp.example.com/sse?token=x"}],
			"skills":["triage","summarise"],
			"labels":{"team":"ops","env":"dev"},
			"protocolType":["A2A",3],
			"framework":["not","a","string"]
		},
		"evidence":[{"code":"github.d4_mcp_client_code","source":"agent.py"}]}]}`
	var req models.PublishShadowScanRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))

	f := newPublishFixture(models.ShadowProviderGitHub, models.ShadowConnectionConfig{AllRepositories: true},
		models.ShadowScanOptions{})
	_, err := f.publish(auditableCtx(t), &req)
	require.NoError(t, err)
	require.Len(t, f.published.Apply.Findings, 1)
	attrs := f.published.Apply.Findings[0].Finding.Attributes

	assert.Equal(t, []sa.ToolRef{{Type: "function", Name: "search"}, {Type: "mcp"}}, attrs["tools"],
		"type and name only")
	assert.Equal(t, []sa.MCPServerRef{{Label: "files", Origin: "https://mcp.example.com"}}, attrs["mcpServers"],
		"the origin only")
	assert.Equal(t, []string{"triage", "summarise"}, attrs["skills"])
	assert.Equal(t, map[string]string{"team": "ops", "env": "dev"}, attrs["labels"])
	assert.NotContains(t, attrs, "protocolType", "a list that is not all strings is dropped")
	assert.NotContains(t, attrs, "framework", "a list is dropped where the platforms store a string")
	assert.Equal(t, "rules", attrs["reviewedBy"])

	stored, err := json.Marshal(attrs)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), shadowPromptMark)
	assert.NotContains(t, string(stored), "token=x")
}

func TestShadowPublishAttributeValue_DropsMalformedShapes(t *testing.T) {
	cases := map[string]struct {
		key string
		raw string
	}{
		"a tool that is not an object":     {"tools", `["search"]`},
		"a tool name that is not text":     {"tools", `[{"type":"function","name":7}]`},
		"an MCP server that is not object": {"mcpServers", `[["files"]]`},
		"an MCP origin that is not text":   {"mcpServers", `[{"label":"files","origin":true}]`},
		"a label value that is not text":   {"labels", `{"team":"ops","count":2}`},
		"a list with an object in it":      {"skills", `["triage",{"x":1}]`},
		"a list where a string belongs":    {"framework", `["langgraph"]`},
		"an object where a string belongs": {"model", `{"name":"gpt"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var v any
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &v))
			_, ok := shadowPublishAttributeValue(tc.key, v)
			assert.False(t, ok)
		})
	}
	v, ok := shadowPublishAttributeValue("toolCount", float64(2))
	assert.True(t, ok, "scalars pass through to the sanitiser")
	assert.InDelta(t, 2.0, v, 0)
}
