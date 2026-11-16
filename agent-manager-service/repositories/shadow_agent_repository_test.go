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
	assert.Equal(t, "Bot", rows[0].DisplayName, "ordered by name")
	bot := rows[0]
	assert.Equal(t, "conn", bot.ConnectionName)
	assert.True(t, bot.Present)
	assert.Equal(t, models.ShadowStatusNew, bot.Status)
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

// The same agent (provider, kind, external ID) found through two connections
// is one agent: listed once with both connections, counted once, one status.
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

	// One status for the agent, whichever row is reviewed.
	require.NoError(t, repo.UpdateFindingStatus(ctx, ou, shared.FoundBy[0].findingUUID(t), models.ShadowStatusReviewed))
	for _, src := range shared.FoundBy {
		got, err := repo.GetFinding(ctx, ou, src.findingUUID(t))
		require.NoError(t, err)
		assert.Equal(t, models.ShadowStatusReviewed, got.Status, src.ConnectionName)
		assert.Len(t, got.FoundBy, 2)
	}
	_, total, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Status: models.ShadowStatusReviewed, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	sum, err = repo.Summary(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, 1, sum.ByStatus[models.ShadowStatusReviewed])
	assert.Equal(t, 1, sum.ByStatus[models.ShadowStatusNew])

	// A third connection that finds the agent later inherits its status.
	c := newShadowConnection(t, repo, ou, "Gamma")
	_, err = repo.ApplyConnectionResult(ctx, ShadowConnectionApply{
		OUID: ou, ConnectionID: c.ID, ScanID: newShadowScan(t, repo, ou), ScanStartedAt: t0.Add(2 * time.Minute), Complete: true,
		Findings: []ShadowFindingInput{agentInput("agent_shared", "Shared", t0.Add(2*time.Minute))},
	})
	require.NoError(t, err)
	rows, _, err = repo.ListFindings(ctx, ou, ShadowFindingFilter{Search: "shared", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, models.ShadowStatusReviewed, rows[0].Status)
	assert.Len(t, rows[0].FoundBy, 3)

	assert.True(t, IsShadowNotFound(repo.UpdateFindingStatus(ctx, ou, uuid.New(), models.ShadowStatusReviewed)))
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
	require.NoError(t, repo.FinishScan(ctx, s1))
	s2 := &models.ShadowScan{ID: uuid.New(), OUID: ou, Status: models.ShadowScanQueued, ConnectionIDs: []string{"c"}}
	require.NoError(t, repo.CreateScan(ctx, s2), "a new scan can be queued once the last one finished")
	latest, err := repo.LatestScan(ctx, ou)
	require.NoError(t, err)
	assert.Equal(t, s2.ID, latest.ID)
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
	done.Status = models.ShadowScanSucceeded
	require.NoError(t, repo.FinishScan(ctx, done))
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
