//go:build integration

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

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func newShadowConnection(t *testing.T, repo ShadowAgentRepository, ouID, name string) *models.ShadowConnection {
	t.Helper()
	c := &models.ShadowConnection{
		ID: uuid.New(), OUID: ouID, Name: name, Provider: models.ShadowProviderOpenAI,
		Config:          models.ShadowConnectionConfig{Endpoints: map[string]string{"api": "https://api.openai.com/v1"}},
		SecretEncrypted: []byte("ciphertext"), SecretHints: map[string]string{},
	}
	require.NoError(t, repo.CreateConnection(context.Background(), c))
	t.Cleanup(func() { _, _ = repo.DeleteConnection(context.Background(), ouID, c.ID) })
	return c
}

// newShadowScan records a finished scan, so evidence and scan links can point at it.
func newShadowScan(t *testing.T, repo ShadowAgentRepository, ouID string) uuid.UUID {
	t.Helper()
	s := &models.ShadowScan{ID: uuid.New(), OUID: ouID, Status: models.ShadowScanSucceeded, ConnectionIDs: []string{}}
	require.NoError(t, repo.CreateScan(context.Background(), s))
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM shadow_scans WHERE id = ?", s.ID) })
	return s.ID
}

// agentInput is an OpenAI agent as a connector reports it, with its evidence.
func agentInput(externalID, name string, at time.Time) ShadowFindingInput {
	return ShadowFindingInput{
		Finding: models.ShadowFinding{
			Provider: models.ShadowProviderOpenAI, Kind: models.ShadowKindPlatformAgent, ExternalID: externalID,
			DisplayName: name, Scope: "proj_1", State: "active", Attributes: map[string]any{"model": "gpt-4.1"},
		},
		Signals: []models.ShadowFindingSignal{{Code: "openai.agent_object", Source: "GET /v1/agents", DetectedAt: at}},
	}
}

func TestShadowApplyConnectionResultIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newShadowConnection(t, repo, ou, "conn")
	scan1 := time.Now().UTC().Truncate(time.Microsecond)
	in := ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: scan1, Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_1", "Bot", scan1), agentInput("agent_2", "Helper", scan1)},
	}
	in.Findings[0].Finding.OwnerHint = models.ShadowOwnerHint{Email: "alice@example.com"}
	n, err := repo.ApplyConnectionResult(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// The same scan again: nothing is duplicated.
	_, err = repo.ApplyConnectionResult(ctx, in)
	require.NoError(t, err)
	rows, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, rows, 2)
	assert.Equal(t, "Bot", rows[0].DisplayName, "seen at the same time, so ordered by name")
	bot := rows[0]
	assert.Equal(t, "conn", bot.ConnectionName)
	assert.True(t, bot.Present)
	assert.Empty(t, bot.LinkedAgent, "not onboarded")
	assert.Equal(t, "alice@example.com", bot.OwnerHint.Email)
	require.Len(t, bot.FoundBy, 1)
	assert.Equal(t, c.ID.String(), bot.FoundBy[0].ConnectionID)
	assert.Equal(t, bot.ID.String(), bot.FoundBy[0].FindingID)
	signals, err := repo.ListSignals(ctx, ou, bot.ID)
	require.NoError(t, err)
	require.Len(t, signals, 1)
	assert.Equal(t, "GET /v1/agents", signals[0].Source)

	// A later scan whose listing has no owner clears it: the owner is only
	// what the listing reports now.
	in2 := in
	in2.ScanID, in2.ScanStartedAt = newShadowScan(t, repo, ou), scan1.Add(time.Hour)
	in2.Findings = []ShadowFindingInput{agentInput("agent_1", "Bot", scan1.Add(time.Hour)), agentInput("agent_2", "Helper", scan1.Add(time.Hour))}
	_, err = repo.ApplyConnectionResult(ctx, in2)
	require.NoError(t, err)
	got, err := repo.GetFinding(ctx, ou, bot.ID)
	require.NoError(t, err)
	assert.Empty(t, got.OwnerHint.Email)
	assert.True(t, got.LastSeenAt.Equal(scan1.Add(time.Hour)))
	assert.True(t, got.FirstSeenAt.Equal(scan1))

	// Other orgs see nothing.
	_, total, err = repo.ListFindings(ctx, "ou-"+uuid.NewString(), ShadowFindingFilter{Limit: 50})
	require.NoError(t, err)
	assert.Zero(t, total)
	_, err = repo.GetFinding(ctx, "ou-"+uuid.NewString(), bot.ID)
	assert.True(t, IsShadowNotFound(err))
}

// The list shows the most recently seen agents first, and agents seen at the
// same time by name, so pages do not overlap.
func TestShadowListFindingsMostRecentlySeenFirst(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	older, newer := newShadowConnection(t, repo, ou, "older"), newShadowConnection(t, repo, ou, "newer")
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: older.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_b", "bravo", t0), agentInput("agent_a", "Alpha", t0)},
	})
	require.NoError(t, err)
	t1 := t0.Add(time.Hour)
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: newer.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t1, Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_z", "Zulu", t1)},
	})
	require.NoError(t, err)

	rows, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 3, total)
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.DisplayName)
	}
	assert.Equal(t, []string{"Zulu", "Alpha", "bravo"}, names, "newest first, then by name ignoring case")

	for i, want := range names {
		page, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 1, Offset: i})
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, want, page[0].DisplayName, "page %d", i)
	}
}

// An agent a complete listing no longer returns is marked as no longer
// present, and hidden; an incomplete listing marks nothing; finding the
// agent again brings it back.
func TestShadowPresenceFollowsCompleteListings(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newShadowConnection(t, repo, ou, "conn")
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	scan1 := newShadowScan(t, repo, ou)
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: scan1, ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_1", "Bot", t0), agentInput("agent_2", "Gone", t0)},
	})
	require.NoError(t, err)

	// An incomplete scan that misses agent_2 does not hide it.
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0.Add(time.Hour), Complete: false,
		Findings: []ShadowFindingInput{agentInput("agent_1", "Bot", t0.Add(time.Hour))},
	})
	require.NoError(t, err)
	_, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	// A complete scan that misses agent_2 hides it.
	scan3 := newShadowScan(t, repo, ou)
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: scan3, ScanStartedAt: t0.Add(2 * time.Hour), Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_1", "Bot", t0.Add(2*time.Hour))},
	})
	require.NoError(t, err)
	rows, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "agent_1", rows[0].ExternalID)
	sum, err := repo.Summary(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, 1, sum.Total, "the summary counts only agents still listed")

	// The first scan's own list still shows it, as no longer present.
	rows, _, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{ScanID: &scan1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, r.ExternalID == "agent_1", r.Present, r.ExternalID)
	}
	rows, _, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{ScanID: &scan3, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, rows, 1)

	// Listed again: present again.
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0.Add(3 * time.Hour), Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_1", "Bot", t0.Add(3*time.Hour)), agentInput("agent_2", "Gone", t0.Add(3*time.Hour))},
	})
	require.NoError(t, err)
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
}

// Evidence a complete listing did not see again is removed; correlation
// evidence is kept.
func TestShadowStaleEvidenceRemovedOnCompleteListing(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newShadowConnection(t, repo, ou, "conn")
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	first := agentInput("rt", "Runtime", t0)
	first.Finding.Kind = models.ShadowKindRuntime
	first.Signals = append(first.Signals, models.ShadowFindingSignal{
		Code: "bedrock.registry_agent_record", Source: "Agent Registry ListRegistryRecords", DetectedAt: t0,
	})
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0, Complete: true, Findings: []ShadowFindingInput{first},
	})
	require.NoError(t, err)
	rows, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	id := rows[0].ID
	require.NoError(t, repo.ApplyCorrelation(ctx, ou, nil, map[uuid.UUID][]models.ShadowFindingSignal{
		id: {{Code: "foundry.external_possible_duplicate", DetectedAt: t0}},
	}))

	// The registry record is gone: an incomplete scan keeps its evidence.
	again := agentInput("rt", "Runtime", t0.Add(time.Hour))
	again.Finding.Kind = models.ShadowKindRuntime
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0.Add(time.Hour), Complete: false,
		Findings: []ShadowFindingInput{again},
	})
	require.NoError(t, err)
	signals, err := repo.ListSignals(ctx, ou, id)
	require.NoError(t, err)
	assert.Len(t, signals, 3)

	// A complete scan removes it, and keeps the correlation evidence.
	again.Signals[0].DetectedAt = t0.Add(2 * time.Hour)
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0.Add(2 * time.Hour), Complete: true,
		Findings: []ShadowFindingInput{again},
	})
	require.NoError(t, err)
	signals, err = repo.ListSignals(ctx, ou, id)
	require.NoError(t, err)
	codes := []string{}
	for _, s := range signals {
		codes = append(codes, s.Code)
	}
	assert.ElementsMatch(t, []string{"openai.agent_object", "foundry.external_possible_duplicate"}, codes)
}

// codeAgentInput is a code agent a source code scan found in a repository,
// with its evidence.
func codeAgentInput(externalID, repo string, at time.Time) ShadowFindingInput {
	return ShadowFindingInput{
		Finding: models.ShadowFinding{
			Provider: models.ShadowProviderGitHub, Kind: models.ShadowKindCodeAgent, ExternalID: externalID,
			DisplayName: externalID, Scope: repo, Attributes: map[string]any{},
		},
		Signals: []models.ShadowFindingSignal{{Code: "github.d2_agent_import", Source: "agent.py", DetectedAt: at}},
	}
}

// A complete source code scan that could not finish a repository leaves that
// repository's agents present and keeps their evidence; agents of the
// repositories it finished follow the listing as before.
func TestShadowUnfinishedScopesKeepTheirAgents(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := &models.ShadowConnection{
		ID: uuid.New(), OUID: ou, Name: "repos", Provider: models.ShadowProviderGitHub,
		Config:          models.ShadowConnectionConfig{Endpoints: map[string]string{"api": "https://api.github.com"}},
		SecretEncrypted: []byte("ciphertext"), SecretHints: map[string]string{},
	}
	require.NoError(t, repo.CreateConnection(ctx, c))
	t.Cleanup(func() { _, _ = repo.DeleteConnection(context.Background(), ou, c.ID) })
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{
			codeAgentInput("acme/a:kept", "acme/a", t0), codeAgentInput("acme/a:gone", "acme/a", t0),
			codeAgentInput("acme/b:agent", "acme/b", t0),
		},
	})
	require.NoError(t, err)

	t1 := t0.Add(time.Hour)
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t1, Complete: true,
		Findings:         []ShadowFindingInput{codeAgentInput("acme/a:kept", "acme/a", t1)},
		UnfinishedScopes: []string{"acme/b"},
	})
	require.NoError(t, err)

	rows, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	present := map[string]ShadowFindingRow{}
	for _, r := range rows {
		present[r.ExternalID] = r
	}
	assert.Len(t, present, 2)
	assert.Contains(t, present, "acme/a:kept", "returned again")
	assert.NotContains(t, present, "acme/a:gone", "a finished repository no longer has it")
	require.Contains(t, present, "acme/b:agent", "its repository was not finished")
	signals, err := repo.ListSignals(ctx, ou, present["acme/b:agent"].ID)
	require.NoError(t, err)
	assert.Len(t, signals, 1, "the unfinished repository's evidence is kept")
}

// The same agent (provider, kind, external ID) found through two connections
// is one agent: listed once with both connections, counted once, onboarded once.
func TestShadowAgentsAreDeduplicatedAcrossConnections(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	a := newShadowConnection(t, repo, ou, "Alpha")
	b := newShadowConnection(t, repo, ou, "Beta")
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: a.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_shared", "Shared", t0), agentInput("agent_a", "Only A", t0)},
	})
	require.NoError(t, err)
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: b.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0.Add(time.Minute), Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_shared", "Shared", t0.Add(time.Minute))},
	})
	require.NoError(t, err)

	rows, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total, "two agents, not three rows")
	require.Len(t, rows, 2)
	var shared ShadowFindingRow
	for _, r := range rows {
		if r.ExternalID == "agent_shared" {
			shared = r
		}
	}
	assert.Equal(t, b.ID, shared.ConnectionID, "the connection that saw it last")
	require.Len(t, shared.FoundBy, 2)
	assert.Equal(t, "Alpha", shared.FoundBy[0].ConnectionName)
	assert.Equal(t, "Beta", shared.FoundBy[1].ConnectionName)
	sum, err := repo.Summary(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, 2, sum.Total)
	assert.Equal(t, 2, sum.ByProvider[models.ShadowProviderOpenAI])
	assert.Equal(t, 2, sum.Connections)

	// Filter by connection: only the connection's own rows, still one per agent.
	rows, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{ConnectionID: &a.ID, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	for _, r := range rows {
		assert.Equal(t, a.ID, r.ConnectionID, "a connection filter lists that connection's own rows")
	}
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Provider: models.ShadowProviderBedrock, Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total)

	// Onboarding links the agent on every connection, whichever row is used.
	at := t0.Add(time.Minute)
	require.NoError(t, repo.OnboardFinding(ctx, ou, shared.FoundBy[0].findingUUID(t), "default", "shared-agent", "user-1", at))
	for _, src := range shared.FoundBy {
		got, err := repo.GetFinding(ctx, ou, src.findingUUID(t))
		require.NoError(t, err)
		assert.Equal(t, "default", got.LinkedProject, src.ConnectionName)
		assert.Equal(t, "shared-agent", got.LinkedAgent, src.ConnectionName)
		assert.Equal(t, "user-1", got.OnboardedBy)
		require.NotNil(t, got.OnboardedAt)
		assert.True(t, got.OnboardedAt.Equal(at))
		assert.Len(t, got.FoundBy, 2)
	}
	err = repo.OnboardFinding(ctx, ou, shared.FoundBy[1].findingUUID(t), "default", "other", "user-2", at)
	assert.ErrorIs(t, err, utils.ErrShadowAlreadyOnboarded, "an agent is onboarded once, through any connection")
	on, off := true, false
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Onboarded: &on, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Onboarded: &off, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	sum, err = repo.Summary(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, 2, sum.Total)
	assert.Equal(t, 1, sum.Onboarded)

	// A third connection that finds the agent later inherits its onboarding.
	c := newShadowConnection(t, repo, ou, "Gamma")
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0.Add(2 * time.Minute), Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_shared", "Shared", t0.Add(2*time.Minute))},
	})
	require.NoError(t, err)
	rows, _, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Search: "shared", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "shared-agent", rows[0].LinkedAgent)
	assert.Len(t, rows[0].FoundBy, 3)
	var unlinked int64
	require.NoError(t, db.GetDB().Raw(`SELECT COUNT(*) FROM shadow_findings WHERE ou_id = ? AND external_id = 'agent_shared' AND linked_agent = ''`, ou).Scan(&unlinked).Error)
	assert.Zero(t, unlinked, "every connection's row carries the link")

	assert.True(t, IsShadowNotFound(repo.OnboardFinding(ctx, ou, uuid.New(), "default", "x", "u", at)))
}

func (s ShadowFindingSource) findingUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s.FindingID)
	require.NoError(t, err)
	return id
}

func TestShadowScansOneActivePerOrgAndLeases(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	s1 := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanQueued, ConnectionIDs: []string{}}
	require.NoError(t, repo.CreateScan(ctx, s1))
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM shadow_scans WHERE ou_id = ?", ou) })
	err := repo.CreateScan(ctx, &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanQueued, ConnectionIDs: []string{}})
	assert.ErrorIs(t, err, utils.ErrShadowScanInProgress)

	now := time.Now().UTC()
	var claimed []models.ShadowScan
	for i := 0; i < 50 && len(claimed) == 0; i++ {
		got, err := repo.ClaimDueScans(ctx, now, time.Minute, 100)
		require.NoError(t, err)
		for _, s := range got {
			if s.ID == s1.ID {
				claimed = append(claimed, s)
			} else {
				// Another test's scan: release it.
				db.GetDB().Exec("UPDATE shadow_scans SET status = 'queued', claimed_until = NULL WHERE id = ?", s.ID)
			}
		}
	}
	require.Len(t, claimed, 1)
	assert.Equal(t, models.ShadowScanRunning, claimed[0].Status)
	assert.Equal(t, 1, claimed[0].Attempts)

	again, err := repo.ClaimDueScans(ctx, now, time.Minute, 100)
	require.NoError(t, err)
	for _, s := range again {
		assert.NotEqual(t, s1.ID, s.ID, "a leased scan is not claimed twice")
	}
	expired, err := repo.ClaimDueScans(ctx, now.Add(2*time.Minute), time.Minute, 100)
	require.NoError(t, err)
	var reclaimed bool
	for _, s := range expired {
		if s.ID == s1.ID {
			reclaimed = true
			assert.Equal(t, 2, s.Attempts)
		}
	}
	assert.True(t, reclaimed, "a scan whose lease ran out is claimed again")

	finished := time.Now().UTC()
	s1.Status, s1.FinishedAt = models.ShadowScanSucceeded, &finished
	s1.Results = []models.ShadowScanConnectionResult{{ConnectionID: "x", Outcome: "ok", Calls: []models.ShadowScanCallResult{}}}
	ok, err := repo.FinishScan(ctx, s1)
	require.NoError(t, err)
	assert.True(t, ok, "a running scan is finished")
	s2 := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanQueued, ConnectionIDs: []string{"c"}}
	require.NoError(t, repo.CreateScan(ctx, s2), "a new scan can be queued once the last one finished")
	latest, err := repo.LatestScan(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, s2.ID, latest.ID)
}

// A pending scan waits for its scan command: the worker never claims it, it
// does not block a cloud scan, Clear scans keeps it, and after an hour it
// fails with a result per connection.
func TestShadowPendingScans(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newShadowConnection(t, repo, ou, "GitHub repos")
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM shadow_scans WHERE ou_id = ?", ou) })
	pending := &models.ShadowScan{
		ID: uuid.New(), OUID: ou, Status: models.ShadowScanPending, ConnectionIDs: []string{c.ID.String()},
		Options: models.ShadowScanOptions{AIReview: true},
	}
	require.NoError(t, repo.CreateScan(ctx, pending))
	got, err := repo.GetScan(ctx, ou, pending.ID)
	require.NoError(t, err)
	assert.True(t, got.Options.AIReview, "the options are stored")

	queued := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanQueued, ConnectionIDs: []string{}}
	require.NoError(t, repo.CreateScan(ctx, queued), "a pending scan does not block a cloud scan")
	later := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanPending, ConnectionIDs: []string{}}
	require.NoError(t, repo.CreateScan(ctx, later))
	latest, err := repo.LatestScan(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, queued.ID, latest.ID, "a newer pending scan does not hide the cloud scan in progress")
	claimed, err := repo.ClaimDueScans(ctx, time.Now().UTC(), time.Minute, 100)
	require.NoError(t, err)
	for _, s := range claimed {
		assert.NotEqual(t, pending.ID, s.ID, "the worker never claims a pending scan")
	}
	db.GetDB().Exec("UPDATE shadow_scans SET status = 'succeeded' WHERE id = ?", queued.ID)

	n, err := repo.DeleteFinishedScans(ctx, ou)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "only the finished scan is cleared")
	_, err = repo.GetScan(ctx, ou, pending.ID)
	require.NoError(t, err, "Clear scans keeps a pending scan")

	now := time.Now().UTC().Truncate(time.Microsecond)
	cancelled, err := repo.CancelPendingScan(ctx, "ou-"+uuid.NewString(), pending.ID, now)
	require.NoError(t, err)
	assert.False(t, cancelled, "another org cannot cancel the scan")
	got, err = repo.GetScan(ctx, ou, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanPending, got.Status)

	cancelled, err = repo.CancelPendingScan(ctx, ou, pending.ID, now)
	require.NoError(t, err)
	assert.True(t, cancelled)
	cancelled, err = repo.CancelPendingScan(ctx, ou, pending.ID, now)
	require.NoError(t, err)
	assert.False(t, cancelled, "a scan that is no longer pending is not cancelled again")
	got, err = repo.GetScan(ctx, ou, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanFailed, got.Status)
	require.NotNil(t, got.FinishedAt)
	require.Len(t, got.Results, 1)
	assert.Equal(t, c.ID.String(), got.Results[0].ConnectionID)
	assert.Equal(t, "GitHub repos", got.Results[0].ConnectionName)
	assert.Equal(t, models.ShadowScanFailed, got.Results[0].Outcome)
	assert.Equal(t, models.ShadowScanCancelledMessage, got.Results[0].Message)
	assert.Empty(t, got.Results[0].Calls)
	stillQueued, err := repo.CancelPendingScan(ctx, ou, queued.ID, now)
	require.NoError(t, err)
	assert.False(t, stillQueued, "only a pending scan is cancelled")
}

// A queued or running cloud scan can be cancelled: it fails at once, so a new
// scan can start, and the worker's progress and finish no longer change it.
func TestShadowCancelActiveScans(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	a := newShadowConnection(t, repo, ou, "OpenAI a")
	b := newShadowConnection(t, repo, ou, "OpenAI b")
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM shadow_scans WHERE ou_id = ?", ou) })
	now := time.Now().UTC().Truncate(time.Microsecond)

	queued := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanQueued, ConnectionIDs: []string{a.ID.String(), b.ID.String()}}
	require.NoError(t, repo.CreateScan(ctx, queued))
	cancelled, err := repo.CancelActiveScan(ctx, "ou-"+uuid.NewString(), queued.ID, now)
	require.NoError(t, err)
	assert.False(t, cancelled, "another org cannot cancel the scan")
	cancelled, err = repo.CancelActiveScan(ctx, ou, queued.ID, now)
	require.NoError(t, err)
	assert.True(t, cancelled)
	got, err := repo.GetScan(ctx, ou, queued.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanFailed, got.Status)
	require.NotNil(t, got.FinishedAt)
	require.Len(t, got.Results, 2)
	for i, c := range []*models.ShadowConnection{a, b} {
		assert.Equal(t, c.ID.String(), got.Results[i].ConnectionID)
		assert.Equal(t, c.Name, got.Results[i].ConnectionName)
		assert.Equal(t, models.ShadowScanFailed, got.Results[i].Outcome)
		assert.Equal(t, models.ShadowConnectionCancelledMessage, got.Results[i].Message)
	}
	cancelled, err = repo.CancelActiveScan(ctx, ou, queued.ID, now)
	require.NoError(t, err)
	assert.False(t, cancelled, "a finished scan is not cancelled again")

	running := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanRunning, ConnectionIDs: []string{a.ID.String(), b.ID.String()}}
	require.NoError(t, repo.CreateScan(ctx, running), "the cancelled scan no longer blocks a new one")
	pendingCancelled, err := repo.CancelPendingScan(ctx, ou, running.ID, now)
	require.NoError(t, err)
	assert.False(t, pendingCancelled, "a running scan is not a pending one")
	started := now.Add(-time.Minute)
	finishedAt := now.Add(-time.Second)
	progress := []models.ShadowScanConnectionResult{
		{
			ConnectionID: a.ID.String(), ConnectionName: a.Name, Provider: a.Provider, Outcome: "ok", FindingsCount: 2,
			Calls: []models.ShadowScanCallResult{{Call: "GET /v1/agents", Outcome: "ok", Items: 2}}, StartedAt: &started, FinishedAt: &finishedAt,
		},
		{
			ConnectionID: b.ID.String(), ConnectionName: b.Name, Provider: b.Provider, Outcome: models.ShadowConnectionRunning,
			Calls: []models.ShadowScanCallResult{{Call: "GET /v1/agents", Outcome: "ok", Items: 1}}, StartedAt: &started,
		},
	}
	ok, err := repo.UpdateScanProgress(ctx, running.ID, progress)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = repo.ScanRunning(ctx, running.ID)
	require.NoError(t, err)
	assert.True(t, ok)

	cancelled, err = repo.CancelActiveScan(ctx, ou, running.ID, now)
	require.NoError(t, err)
	assert.True(t, cancelled)
	got, err = repo.GetScan(ctx, ou, running.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanFailed, got.Status)
	require.Len(t, got.Results, 2)
	assert.Equal(t, "ok", got.Results[0].Outcome, "a finished connection keeps its result")
	assert.Equal(t, 2, got.Results[0].FindingsCount)
	assert.Equal(t, models.ShadowScanFailed, got.Results[1].Outcome)
	assert.Equal(t, models.ShadowConnectionCancelledMessage, got.Results[1].Message)
	assert.Len(t, got.Results[1].Calls, 1, "the calls made so far are kept")
	require.NotNil(t, got.Results[1].StartedAt)
	assert.Nil(t, got.Results[1].FinishedAt)
	assert.Equal(t, 2, got.FindingsCount, "the agents of the connection that finished are counted")

	ok, err = repo.ScanRunning(ctx, running.ID)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = repo.UpdateScanProgress(ctx, running.ID, []models.ShadowScanConnectionResult{})
	require.NoError(t, err)
	assert.False(t, ok, "progress no longer changes a cancelled scan")
	ok, err = repo.FinishScan(ctx, &models.ShadowScan{
		ID: running.ID, OUID: ou, Status: models.ShadowScanSucceeded,
		Results: []models.ShadowScanConnectionResult{}, FinishedAt: &now,
	})
	require.NoError(t, err)
	assert.False(t, ok, "the worker does not overwrite a cancelled scan")
	got, err = repo.GetScan(ctx, ou, running.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanFailed, got.Status)
	assert.Len(t, got.Results, 2)

	// A connection's agents and final result are saved together.
	done := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanRunning, ConnectionIDs: []string{a.ID.String(), b.ID.String()}}
	require.NoError(t, repo.CreateScan(ctx, done))
	n, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: a.ID, ScanID: done.ID, ScanStartedAt: now,
		Findings:    []ShadowFindingInput{agentInput("agent_a", "A", now)},
		Result:      &models.ShadowScanConnectionResult{ConnectionID: a.ID.String(), Outcome: "ok", Calls: []models.ShadowScanCallResult{}},
		ResultIndex: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	got, err = repo.GetScan(ctx, ou, done.ID)
	require.NoError(t, err)
	require.Len(t, got.Results, 2)
	assert.Equal(t, "ok", got.Results[0].Outcome)
	assert.Equal(t, 1, got.Results[0].FindingsCount)
	assert.Equal(t, models.ShadowConnectionQueued, got.Results[1].Outcome, "a connection with no result yet is queued")
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: b.ID, ScanID: done.ID, ScanStartedAt: now,
		Result:      &models.ShadowScanConnectionResult{ConnectionID: b.ID.String(), Outcome: "ok", Calls: []models.ShadowScanCallResult{}},
		ResultIndex: 1,
	})
	require.NoError(t, err)
	cancelled, err = repo.CancelActiveScan(ctx, ou, done.ID, now)
	require.NoError(t, err)
	assert.False(t, cancelled, "a scan whose connections all finished is left to its worker")
	got, err = repo.GetScan(ctx, ou, done.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanRunning, got.Status)
	ok, err = repo.FinishScan(ctx, &models.ShadowScan{
		ID: done.ID, OUID: ou, Status: models.ShadowScanSucceeded, Results: got.Results, FindingsCount: 1, FinishedAt: &now,
	})
	require.NoError(t, err)
	require.True(t, ok)

	// A result saved after the cancel committed writes nothing.
	late := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanRunning, ConnectionIDs: []string{b.ID.String()}}
	require.NoError(t, repo.CreateScan(ctx, late))
	cancelled, err = repo.CancelActiveScan(ctx, ou, late.ID, now)
	require.NoError(t, err)
	require.True(t, cancelled)
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: b.ID, ScanID: late.ID, ScanStartedAt: now, Complete: true,
		Findings:    []ShadowFindingInput{agentInput("agent_b", "B", now)},
		Result:      &models.ShadowScanConnectionResult{ConnectionID: b.ID.String(), Outcome: "ok", Calls: []models.ShadowScanCallResult{}},
		ResultIndex: 0,
	})
	require.ErrorIs(t, err, utils.ErrShadowScanNotRunning)
	_, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{ScanID: &late.ID, Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total, "nothing is applied after the cancel")
	got, err = repo.GetScan(ctx, ou, late.ID)
	require.NoError(t, err)
	require.Len(t, got.Results, 1)
	assert.Equal(t, models.ShadowConnectionCancelledMessage, got.Results[0].Message)
	assert.Zero(t, got.FindingsCount)
}

// Clearing deletes only finished scans with their per-scan agent lists;
// the agents and their evidence stay, and nothing points at a deleted scan.
func TestShadowDeleteFinishedScans(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newShadowConnection(t, repo, ou, "conn")
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM shadow_scans WHERE ou_id = ?", ou) })
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	done := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanQueued, ConnectionIDs: []string{c.ID.String()}}
	require.NoError(t, repo.CreateScan(ctx, done))
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: done.ID, ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_1", "Bot", t0)},
	})
	require.NoError(t, err)
	require.NoError(t, db.GetDB().Exec("UPDATE shadow_scans SET status = 'running' WHERE id = ?", done.ID).Error)
	done.Status = models.ShadowScanSucceeded
	ok, err := repo.FinishScan(ctx, done)
	require.NoError(t, err)
	require.True(t, ok)
	running := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanRunning, ConnectionIDs: []string{}}
	require.NoError(t, repo.CreateScan(ctx, running))

	n, err := repo.DeleteFinishedScans(ctx, ou)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	scans, err := repo.ListScans(ctx, ou, 10)
	require.NoError(t, err)
	require.Len(t, scans, 1)
	assert.Equal(t, running.ID, scans[0].ID, "a running scan is kept")

	rows, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, total, "the agent stays")
	assert.Nil(t, rows[0].LastScanID, "nothing points at a deleted scan")
	signals, err := repo.ListSignals(ctx, ou, rows[0].ID)
	require.NoError(t, err)
	require.Len(t, signals, 1)
	assert.Nil(t, signals[0].ScanID)
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{ScanID: &done.ID, Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total, "the per-scan list went with the scan")

	other, err := repo.DeleteFinishedScans(ctx, "ou-"+uuid.NewString())
	require.NoError(t, err)
	assert.Zero(t, other)
}

func TestShadowSearchAndConnectionCascade(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newShadowConnection(t, repo, ou, "conn")
	t0 := time.Now().UTC()
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_k", "50%_off agent", t0)},
	})
	require.NoError(t, err)

	// Search treats % and _ literally, and also matches the platform ID and scope.
	for q, want := range map[string]int{"50%_off": 1, "%_": 1, "OFF AGENT": 1, "agent_k": 1, "proj_1": 1, "5_%": 0, "k%t": 0} {
		got, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Search: q, Limit: 10})
		require.NoError(t, err)
		assert.Len(t, got, want, q)
	}

	deleted, err := repo.DeleteConnection(ctx, ou, c.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	_, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total, "agents are deleted with their connection")
}

func TestShadowCorrelationReplacesItsRelations(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newShadowConnection(t, repo, ou, "conn")
	t0 := time.Now().UTC()
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{agentInput("x", "External", t0), agentInput("y", "Other", t0)},
	})
	require.NoError(t, err)
	all, err := repo.ListOrgFindings(ctx, ou)
	require.NoError(t, err)
	require.Len(t, all, 2)
	xID, yID := all[0].ID, all[1].ID
	if all[0].ExternalID != "x" {
		xID, yID = yID, xID
	}

	apply := func() {
		require.NoError(t, repo.ApplyCorrelation(ctx, ou,
			map[uuid.UUID][]models.ShadowFindingRelation{xID: {{FindingID: yID.String(), Relation: "possible_duplicate"}}},
			map[uuid.UUID][]models.ShadowFindingSignal{xID: {{Code: "foundry.external_possible_duplicate", DetectedAt: t0}}}))
	}
	apply()
	apply()
	got, err := repo.GetFinding(ctx, ou, xID)
	require.NoError(t, err)
	require.Len(t, got.Related, 1, "applied twice, kept once")
	assert.Equal(t, "possible_duplicate", got.Related[0].Relation)
	assert.Equal(t, ShadowCorrelationSource, got.Related[0].Source)
	signals, err := repo.ListSignals(ctx, ou, xID)
	require.NoError(t, err)
	assert.Len(t, signals, 2)

	require.NoError(t, repo.ApplyCorrelation(ctx, ou, nil, nil))
	got, err = repo.GetFinding(ctx, ou, xID)
	require.NoError(t, err)
	assert.Empty(t, got.Related)
	signals, err = repo.ListSignals(ctx, ou, xID)
	require.NoError(t, err)
	assert.Len(t, signals, 1, "only the listing's own evidence stays")
}

// Deleting an agent removes it on every connection that found it, with its
// evidence and per-scan links; deleting all keeps connections and scans.
func TestShadowDeleteAgents(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	other := "ou-" + uuid.NewString()
	a := newShadowConnection(t, repo, ou, "Alpha")
	b := newShadowConnection(t, repo, ou, "Beta")
	o := newShadowConnection(t, repo, other, "Other org")
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	scanA := newShadowScan(t, repo, ou)
	apply := func(ouID string, c *models.ShadowConnection, scanID uuid.UUID, ids ...string) {
		t.Helper()
		in := ShadowConnectionApply{OUID: ouID, ConnectionID: c.ID, ScanID: scanID, ScanStartedAt: t0, Complete: true}
		for _, id := range ids {
			in.Findings = append(in.Findings, agentInput(id, "Agent "+id, t0))
		}
		_, err := repo.ApplyConnectionResult(ctx, in)
		require.NoError(t, err)
	}
	apply(ou, a, scanA, "shared", "only_a")
	apply(ou, b, newShadowScan(t, repo, ou), "shared")
	apply(other, o, newShadowScan(t, repo, other), "shared")

	rows, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	var shared ShadowFindingRow
	for _, r := range rows {
		if r.ExternalID == "shared" {
			shared = r
		}
	}
	require.Len(t, shared.FoundBy, 2)

	// Another org cannot delete it, and an unknown ID deletes nothing.
	found, err := repo.DeleteFinding(ctx, other, shared.ID)
	require.NoError(t, err)
	assert.False(t, found)
	found, err = repo.DeleteFinding(ctx, ou, uuid.New())
	require.NoError(t, err)
	assert.False(t, found)

	// One delete removes both connections' rows of the agent.
	found, err = repo.DeleteFinding(ctx, ou, shared.ID)
	require.NoError(t, err)
	assert.True(t, found)
	var left int64
	require.NoError(t, db.GetDB().Raw(`SELECT COUNT(*) FROM shadow_findings WHERE ou_id = ? AND external_id = 'shared'`, ou).Scan(&left).Error)
	assert.Zero(t, left, "every row of the agent is deleted")
	require.NoError(t, db.GetDB().Raw(`SELECT COUNT(*) FROM shadow_finding_signals s JOIN shadow_findings f ON f.id = s.finding_id
		WHERE f.ou_id = ? AND f.external_id = 'shared'`, ou).Scan(&left).Error)
	assert.Zero(t, left)
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	_, total, err = repo.ListFindings(ctx, other, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "the other org's agent is kept")

	// Delete all: the org's agents go, its connections and scans stay.
	n, err := repo.DeleteFindings(ctx, ou)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{ScanID: &scanA, Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total)
	conns, err := repo.ListConnections(ctx, ou)
	require.NoError(t, err)
	assert.Len(t, conns, 2)
	_, err = repo.GetScan(ctx, ou, scanA)
	require.NoError(t, err, "scans are kept")
	n, err = repo.DeleteFindings(ctx, ou)
	require.NoError(t, err)
	assert.Zero(t, n)
	_, total, err = repo.ListFindings(ctx, other, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "the other org's agent is still kept")
}

// newSourceConnection records a GitHub connection, which stores no secret.
func newSourceConnection(t *testing.T, repo ShadowAgentRepository, ouID, name string) *models.ShadowConnection {
	t.Helper()
	c := &models.ShadowConnection{
		ID: uuid.New(), OUID: ouID, Name: name, Provider: models.ShadowProviderGitHub,
		Config:      models.ShadowConnectionConfig{Endpoints: map[string]string{"api": "https://api.github.com"}},
		SecretHints: map[string]string{},
	}
	require.NoError(t, repo.CreateConnection(context.Background(), c))
	t.Cleanup(func() { _, _ = repo.DeleteConnection(context.Background(), ouID, c.ID) })
	return c
}

// newPendingScan records a pending source code scan of one connection.
func newPendingScan(t *testing.T, repo ShadowAgentRepository, ouID string, connID uuid.UUID) *models.ShadowScan {
	t.Helper()
	s := &models.ShadowScan{
		ID: uuid.New(), OUID: ouID, Status: models.ShadowScanPending, ConnectionIDs: []string{connID.String()},
	}
	require.NoError(t, repo.CreateScan(context.Background(), s))
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM shadow_scans WHERE id = ?", s.ID) })
	return s
}

// scanPublish is what a scan command publishes for a pending scan: findings,
// the repositories it scanned and those it failed on.
func scanPublish(s *models.ShadowScan, c *models.ShadowConnection, findings []ShadowFindingInput,
	scanned []string, failed []string,
) ShadowScanPublish {
	result := models.ShadowScanConnectionResult{
		ConnectionID: c.ID.String(), ConnectionName: c.Name, Provider: c.Provider, Outcome: "ok",
		Calls: []models.ShadowScanCallResult{}, ScannedRepositories: scanned,
	}
	status := models.ShadowScanSucceeded
	for _, f := range failed {
		result.FailedRepositories = append(result.FailedRepositories, models.ShadowScanFailedRepository{Name: f, Message: "clone failed"})
		result.Outcome, status = "partial", models.ShadowScanPartial
	}
	return ShadowScanPublish{
		Apply: ShadowConnectionApply{
			OUID: s.OUID, ConnectionID: c.ID, ScanID: s.ID, ScanStartedAt: s.CreatedAt, Complete: true,
			Findings: findings, UnfinishedScopes: failed,
		},
		Status: status, Result: result, FinishedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

// A publish finishes its pending scan with the results and saves the agents
// in one transaction, marking gone only agents of repositories it finished;
// the scan then takes no second publish.
func TestShadowPublishScanResultsFinishesThePendingScan(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newSourceConnection(t, repo, ou, "repos")
	t0 := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	_, err := repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0, Complete: true,
		Findings: []ShadowFindingInput{
			codeAgentInput("a:kept", "a", t0), codeAgentInput("a:gone", "a", t0), codeAgentInput("b:agent", "b", t0),
		},
	})
	require.NoError(t, err)

	pending := newPendingScan(t, repo, ou, c.ID)
	now := time.Now().UTC()
	in := scanPublish(pending, c, []ShadowFindingInput{codeAgentInput("a:kept", "a", now)}, []string{"a"}, []string{"b"})
	n, err := repo.PublishScanResults(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := repo.GetScan(ctx, ou, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanPartial, got.Status)
	assert.Equal(t, 1, got.FindingsCount)
	require.NotNil(t, got.StartedAt)
	assert.True(t, got.StartedAt.Equal(got.CreatedAt), "a pending scan started when it was created")
	require.NotNil(t, got.FinishedAt)
	require.Len(t, got.Results, 1)
	assert.Equal(t, 1, got.Results[0].FindingsCount)
	assert.Equal(t, []string{"a"}, got.Results[0].ScannedRepositories)
	assert.Equal(t, []models.ShadowScanFailedRepository{{Name: "b", Message: "clone failed"}}, got.Results[0].FailedRepositories)

	rows, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	present := map[string]bool{}
	for _, r := range rows {
		present[r.ExternalID] = true
	}
	assert.Equal(t, map[string]bool{"a:kept": true, "b:agent": true}, present,
		"the scanned repository's missing agent is gone, the failed repository's agent stays")
	rows, _, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{ScanID: &pending.ID, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, rows, 1, "the scan lists the agent it published")

	_, err = repo.PublishScanResults(ctx, in)
	require.ErrorIs(t, err, utils.ErrShadowScanNotPending)
	assert.Contains(t, err.Error(), "already published")

	missing := in
	missing.Apply.ScanID = uuid.New()
	_, err = repo.PublishScanResults(ctx, missing)
	assert.True(t, IsShadowNotFound(err), "an unknown scan")
	otherOrg := in
	otherOrg.Apply.OUID = "ou-" + uuid.NewString()
	_, err = repo.PublishScanResults(ctx, otherOrg)
	assert.True(t, IsShadowNotFound(err), "another org's scan")
}

// A cancelled scan, or one an earlier version expired, takes no publish, and
// does not stop an older pending scan of the same connection from publishing.
func TestShadowPublishScanResultsRefusesACancelledOrExpiredScan(t *testing.T) {
	for _, msg := range []string{models.ShadowScanCancelledMessage, models.ShadowScanExpiredMessage} {
		t.Run(msg, func(t *testing.T) {
			ctx := context.Background()
			repo := NewShadowAgentRepository(db.GetDB())
			ou := "ou-" + uuid.NewString()
			c := newSourceConnection(t, repo, ou, "repos")
			ended := newPendingScan(t, repo, ou, c.ID)
			now := time.Now().UTC().Truncate(time.Microsecond)
			cancelled, err := repo.CancelPendingScan(ctx, ou, ended.ID, now)
			require.NoError(t, err)
			require.True(t, cancelled)
			if msg == models.ShadowScanExpiredMessage {
				// As the scan worker of an earlier version left it.
				require.NoError(t, db.GetDB().Exec(`UPDATE shadow_scans SET results = jsonb_set(results, '{0,message}', to_jsonb(?::text))
					WHERE id = ?`, msg, ended.ID).Error)
			}

			_, err = repo.PublishScanResults(ctx, scanPublish(ended, c, nil, []string{"a"}, nil))
			require.ErrorIs(t, err, utils.ErrShadowScanNotPending)
			assert.Contains(t, err.Error(), msg)
			got, err := repo.GetScan(ctx, ou, ended.ID)
			require.NoError(t, err)
			assert.Equal(t, models.ShadowScanFailed, got.Status, "the result is kept")

			older := newPendingScan(t, repo, ou, c.ID)
			require.NoError(t, db.GetDB().Exec("UPDATE shadow_scans SET created_at = ? WHERE id = ?",
				got.CreatedAt.Add(-time.Minute), older.ID).Error)
			older.CreatedAt = got.CreatedAt.Add(-time.Minute)
			n, err := repo.PublishScanResults(ctx, scanPublish(older, c, []ShadowFindingInput{codeAgentInput("a:1", "a", now)},
				[]string{"a"}, nil))
			require.NoError(t, err, "a newer scan that ended without results published nothing")
			assert.Equal(t, 1, n)
		})
	}
}

// An older scan cannot publish after a newer scan of the same connection
// did; it writes nothing and stays pending.
func TestShadowPublishScanResultsRefusesAfterANewerScan(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newSourceConnection(t, repo, ou, "repos")
	other := newSourceConnection(t, repo, ou, "other repos")
	older := newPendingScan(t, repo, ou, c.ID)
	otherScan := newPendingScan(t, repo, ou, other.ID)
	newer := newPendingScan(t, repo, ou, c.ID)
	now := time.Now().UTC()

	_, err := repo.PublishScanResults(ctx, scanPublish(newer, c, []ShadowFindingInput{codeAgentInput("a:new", "a", now)},
		[]string{"a"}, nil))
	require.NoError(t, err)

	_, err = repo.PublishScanResults(ctx, scanPublish(older, c, []ShadowFindingInput{codeAgentInput("a:old", "a", now)},
		[]string{"a"}, nil))
	require.ErrorIs(t, err, utils.ErrShadowScanSuperseded)
	got, err := repo.GetScan(ctx, ou, older.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanPending, got.Status)
	rows, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "a:new", rows[0].ExternalID, "the refused publish wrote nothing")

	_, err = repo.PublishScanResults(ctx, scanPublish(otherScan, other, nil, []string{"x"}, nil))
	require.NoError(t, err, "a newer scan of another connection does not count")
}

// A pending scan older than the life of its token takes no publish with a
// scan token, and stays pending for a console upload.
func TestShadowPublishScanResultsRefusesAScanPastItsTokenLife(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newSourceConnection(t, repo, ou, "repos")
	pending := newPendingScan(t, repo, ou, c.ID)
	got, err := repo.GetScan(ctx, ou, pending.ID)
	require.NoError(t, err)

	in := scanPublish(got, c, nil, []string{"a"}, nil)
	in.TokenExpiredCreatedBy = got.CreatedAt
	_, err = repo.PublishScanResults(ctx, in)
	require.ErrorIs(t, err, utils.ErrShadowScanTokenExpired)
	got, err = repo.GetScan(ctx, ou, pending.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanPending, got.Status, "the scan keeps waiting for an upload")

	in.TokenExpiredCreatedBy = got.CreatedAt.Add(-time.Microsecond)
	_, err = repo.PublishScanResults(ctx, in)
	require.NoError(t, err, "a scan created after the cut-off can still be published")
}

// Evidence a publish saves is dated when its scan was created, so a newer
// scan of the connection removes what it no longer sees even when the older
// scan published last before it.
func TestShadowPublishedEvidenceIsDatedByItsScan(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	c := newSourceConnection(t, repo, ou, "repos")
	older := newPendingScan(t, repo, ou, c.ID)
	newer := newPendingScan(t, repo, ou, c.ID)

	agent := codeAgentInput("a:agent", "a", older.CreatedAt)
	agent.Signals = []models.ShadowFindingSignal{
		{Code: "github.d1_agent_dependency", Source: "requirements.txt", DetectedAt: older.CreatedAt},
		{Code: "github.d2_agent_import", Source: "agent.py", DetectedAt: older.CreatedAt},
	}
	_, err := repo.PublishScanResults(ctx, scanPublish(older, c, []ShadowFindingInput{agent}, []string{"a"}, nil))
	require.NoError(t, err, "the older scan publishes first")

	again := codeAgentInput("a:agent", "a", newer.CreatedAt)
	_, err = repo.PublishScanResults(ctx, scanPublish(newer, c, []ShadowFindingInput{again}, []string{"a"}, nil))
	require.NoError(t, err)

	rows, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	signals, err := repo.ListSignals(ctx, ou, rows[0].ID)
	require.NoError(t, err)
	require.Len(t, signals, 1, "evidence the newer scan did not see again is removed")
	assert.Equal(t, "github.d2_agent_import", signals[0].Code)
}

// An onboarded agent removed from the list stays out of it: on every
// connection, after a later scan, and when another connection finds it.
// Putting it back lists it again; an agent that is not onboarded cannot be
// removed.
func TestShadowOnboardedAgentRemovedFromTheList(t *testing.T) {
	ctx := context.Background()
	repo := NewShadowAgentRepository(db.GetDB())
	ou := "ou-" + uuid.NewString()
	a := newShadowConnection(t, repo, ou, "Alpha")
	b := newShadowConnection(t, repo, ou, "Beta")
	t0 := time.Now().UTC().Truncate(time.Microsecond)
	apply := func(c *models.ShadowConnection, at time.Time, ids ...string) {
		t.Helper()
		in := ShadowConnectionApply{OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: at, Complete: true}
		for _, id := range ids {
			in.Findings = append(in.Findings, agentInput(id, "Agent "+id, at))
		}
		_, err := repo.ApplyConnectionResult(ctx, in)
		require.NoError(t, err)
	}
	listed := func() []string {
		t.Helper()
		rows, total, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Limit: 10})
		require.NoError(t, err)
		out := []string{}
		for _, r := range rows {
			out = append(out, r.ExternalID)
		}
		assert.Equal(t, len(out), total)
		return out
	}
	apply(a, t0, "agent_kept", "agent_removed")
	rows, _, err := repo.ListFindings(ctx, ou, ShadowFindingFilter{Search: "agent_removed", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	removed := rows[0]

	err = repo.SetFindingHidden(ctx, ou, removed.ID, true, "user-1", t0)
	assert.ErrorIs(t, err, utils.ErrShadowNotOnboarded, "only an onboarded agent can be removed")
	assert.True(t, IsShadowNotFound(repo.SetFindingHidden(ctx, ou, uuid.New(), true, "user-1", t0)))
	assert.True(t, IsShadowNotFound(repo.SetFindingHidden(ctx, "ou-"+uuid.NewString(), removed.ID, true, "user-1", t0)),
		"another org cannot change it")

	require.NoError(t, repo.OnboardFinding(ctx, ou, removed.ID, "default", "removed-agent", "user-1", t0))
	require.NoError(t, repo.SetFindingHidden(ctx, ou, removed.ID, true, "user-1", t0))
	assert.ElementsMatch(t, []string{"agent_kept"}, listed())
	got, err := repo.GetFinding(ctx, ou, removed.ID)
	require.NoError(t, err)
	require.NotNil(t, got.HiddenAt, "it can still be opened")
	assert.Equal(t, "user-1", got.HiddenBy)
	sum, err := repo.Summary(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, 2, sum.Total, "the summary still counts it")
	assert.Equal(t, 1, sum.Onboarded)

	// A later scan, and another connection that finds it, keep it out.
	apply(a, t0.Add(time.Minute), "agent_kept", "agent_removed")
	apply(b, t0.Add(2*time.Minute), "agent_removed")
	assert.ElementsMatch(t, []string{"agent_kept"}, listed())
	var visible int64
	require.NoError(t, db.GetDB().Raw(`SELECT COUNT(*) FROM shadow_findings WHERE ou_id = ? AND external_id = 'agent_removed' AND hidden_at IS NULL`, ou).Scan(&visible).Error)
	assert.Zero(t, visible, "every connection's row carries it")

	// Put back: listed again, on every connection.
	require.NoError(t, repo.SetFindingHidden(ctx, ou, removed.ID, false, "user-2", t0))
	assert.ElementsMatch(t, []string{"agent_kept", "agent_removed"}, listed())
	got, err = repo.GetFinding(ctx, ou, removed.ID)
	require.NoError(t, err)
	assert.Nil(t, got.HiddenAt)
	assert.Empty(t, got.HiddenBy)
}
