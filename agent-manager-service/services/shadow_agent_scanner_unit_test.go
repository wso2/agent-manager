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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// fastShadowScanner is a scanner that writes progress and checks for a
// cancel every few milliseconds.
func fastShadowScanner(h *shadowScannerHarness, connectors ShadowConnectors) ShadowAgentScanner {
	s := NewShadowAgentScanner(h.repo, connectors, shadowTestKey, discardLogger())
	s.(*shadowAgentScanner).progressEvery = 5 * time.Millisecond
	s.(*shadowAgentScanner).watchEvery = 5 * time.Millisecond
	return s
}

func shadowTestConnection(t *testing.T, name string) models.ShadowConnection {
	t.Helper()
	enc, err := (&shadowAgentService{encryptionKey: shadowTestKey}).encryptSecret(models.ShadowConnectionSecret{})
	require.NoError(t, err)
	return models.ShadowConnection{ID: uuid.New(), OUID: shadowTestOU, Name: name, Provider: models.ShadowProviderOpenAI, SecretEncrypted: enc}
}

// waitFor polls cond until it holds or a second passes.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// A running scan writes each connection's progress: queued, then running
// with the calls made so far (scrubbed), then its final result.
func TestShadowScannerWritesProgress(t *testing.T) {
	c := shadowTestConnection(t, "OpenAI prod")
	scan := models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU, Status: models.ShadowScanRunning, Attempts: 1, ConnectionIDs: []string{c.ID.String()}}
	h := newShadowScannerHarness(scan, c)
	var sawCalls bool
	connectors := ShadowConnectors{models.ShadowProviderOpenAI: &fakeShadowConnector{
		provider: models.ShadowProviderOpenAI,
		scanCtx: func(ctx context.Context, _ sa.Connection) *sa.Result {
			calls := sa.NewCallsFor(ctx)
			calls.Record("GET /v1/agents", 1, nil)
			calls.Note("GET /v1/agents", "listed with sk-SECRETSECRETSECRET")
			// The calls so far are written before the connection ends.
			sawCalls = waitFor(func() bool {
				h.mu.Lock()
				defer h.mu.Unlock()
				last := h.progress[len(h.progress)-1][0]
				return last.Outcome == models.ShadowConnectionRunning && len(last.Calls) == 1
			})
			return agentResult("agent_1", calls.Results()...)
		},
	}}
	fastShadowScanner(h, connectors).RunOnce(context.Background())

	assert.True(t, sawCalls, "the calls made so far are written while the connection runs")
	require.NotEmpty(t, h.progress)
	first := h.progress[0]
	require.Len(t, first, 1)
	assert.Equal(t, models.ShadowConnectionQueued, first[0].Outcome)
	assert.Equal(t, c.ID.String(), first[0].ConnectionID)
	assert.NotNil(t, first[0].Calls)

	var running *models.ShadowScanConnectionResult
	for _, p := range h.progress {
		if p[0].Outcome == models.ShadowConnectionRunning {
			running = &p[0]
			break
		}
	}
	require.NotNil(t, running, "the connection is written as running when it starts")
	assert.Equal(t, "OpenAI prod", running.ConnectionName)
	assert.Equal(t, models.ShadowProviderOpenAI, running.Provider)
	assert.NotNil(t, running.StartedAt)

	last := h.progress[len(h.progress)-1][0]
	assert.Equal(t, shadowConnectionOK, last.Outcome, "the final result is written when the connection ends")
	assert.NotNil(t, last.FinishedAt)
	b, err := json.Marshal(h.progress)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "SECRETSECRET", "progress is scrubbed like the final result")

	require.NotNil(t, h.finished)
	assert.Equal(t, models.ShadowScanSucceeded, h.finished.Status)
	require.Len(t, h.finished.Results, 1)
	assert.NotNil(t, h.finished.Results[0].StartedAt)
	assert.NotNil(t, h.finished.Results[0].FinishedAt)

	applied := h.applied[c.ID.String()]
	require.NotNil(t, applied.Result, "the final result is saved with the agents")
	assert.Equal(t, 0, applied.ResultIndex)
	assert.Equal(t, shadowConnectionOK, applied.Result.Outcome)
	assert.NotNil(t, applied.Result.FinishedAt)
}

// A connection that ends after the user's cancel committed, before the
// worker noticed it, saves nothing: the repository refuses its result, and
// the worker stops without recording an outcome.
func TestShadowScannerApplyAfterCancelSavesNothing(t *testing.T) {
	c := shadowTestConnection(t, "c")
	scan := models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU, Status: models.ShadowScanRunning, Attempts: 1, ConnectionIDs: []string{c.ID.String()}}
	h := newShadowScannerHarness(scan, c)
	connectors := ShadowConnectors{models.ShadowProviderOpenAI: &fakeShadowConnector{
		provider: models.ShadowProviderOpenAI,
		scan: func(sa.Connection) *sa.Result {
			h.cancel() // the cancel commits as the connector returns
			return agentResult("agent_1", models.ShadowScanCallResult{Call: "a", Outcome: models.ShadowCallOK})
		},
	}}
	// The default intervals: the watcher does not notice the cancel first.
	NewShadowAgentScanner(h.repo, connectors, shadowTestKey, discardLogger()).RunOnce(context.Background())
	require.Len(t, h.repo.ApplyConnectionResultCalls(), 1)
	assert.Empty(t, h.applied, "nothing is applied after the cancel")
	assert.Nil(t, h.finished, "the worker does not overwrite the cancelled scan")
}

// A failing progress write is logged and never fails the scan.
func TestShadowScannerProgressFailureIsNotFatal(t *testing.T) {
	c := shadowTestConnection(t, "c")
	scan := models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU, Attempts: 1, ConnectionIDs: []string{c.ID.String()}}
	h := newShadowScannerHarness(scan, c)
	h.repo.UpdateScanProgressFunc = func(context.Context, uuid.UUID, []models.ShadowScanConnectionResult) (bool, error) {
		return false, errors.New("db down")
	}
	connectors := ShadowConnectors{models.ShadowProviderOpenAI: &fakeShadowConnector{
		provider: models.ShadowProviderOpenAI,
		scan: func(sa.Connection) *sa.Result {
			return agentResult("agent_1", models.ShadowScanCallResult{Call: "a", Outcome: models.ShadowCallOK})
		},
	}}
	fastShadowScanner(h, connectors).RunOnce(context.Background())
	require.NotNil(t, h.finished)
	assert.Equal(t, models.ShadowScanSucceeded, h.finished.Status)
	assert.Len(t, h.applied, 1)
}

// When the user cancels a running scan, the connection still running has its
// context cancelled and saves nothing; the connection that finished keeps its
// agents; the worker does not record an outcome over the cancel.
func TestShadowScannerStopsOnCancel(t *testing.T) {
	done := shadowTestConnection(t, "done")
	slow := shadowTestConnection(t, "slow")
	slow.Provider = models.ShadowProviderAnthropic
	scan := models.ShadowScan{
		ID: uuid.New(), OUID: shadowTestOU, Status: models.ShadowScanRunning, Attempts: 1,
		ConnectionIDs: []string{done.ID.String(), slow.ID.String()},
	}
	h := newShadowScannerHarness(scan, done, slow)
	var cause error
	connectors := ShadowConnectors{
		models.ShadowProviderOpenAI: &fakeShadowConnector{provider: models.ShadowProviderOpenAI, scan: func(sa.Connection) *sa.Result {
			return agentResult("agent_done", models.ShadowScanCallResult{Call: "a", Outcome: models.ShadowCallOK})
		}},
		models.ShadowProviderAnthropic: &fakeShadowConnector{
			provider: models.ShadowProviderAnthropic,
			scanCtx: func(ctx context.Context, _ sa.Connection) *sa.Result {
				assert.True(t, waitFor(func() bool {
					h.mu.Lock()
					defer h.mu.Unlock()
					_, ok := h.applied[done.ID.String()]
					return ok
				}))
				h.cancel()
				select {
				case <-ctx.Done():
					cause = context.Cause(ctx)
				case <-time.After(5 * time.Second):
				}
				return agentResult("agent_slow", models.ShadowScanCallResult{Call: "b", Outcome: models.ShadowCallFailed})
			},
		},
	}
	fastShadowScanner(h, connectors).RunOnce(context.Background())

	require.ErrorIs(t, cause, errShadowScanCancelled, "the cancel stops the connection's calls")
	assert.Contains(t, h.applied, done.ID.String(), "a connection that finished keeps its agents")
	assert.NotContains(t, h.applied, slow.ID.String(), "nothing the cancelled connection found is saved")
	assert.Nil(t, h.finished, "the worker does not overwrite the cancelled scan")
}

// A scan cancelled after its claim stops at its first progress write.
func TestShadowScannerStopsOnCancelBeforeStart(t *testing.T) {
	c := shadowTestConnection(t, "c")
	scan := models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU, Status: models.ShadowScanRunning, Attempts: 1, ConnectionIDs: []string{c.ID.String()}}
	h := newShadowScannerHarness(scan, c)
	h.cancel()
	connectors := ShadowConnectors{models.ShadowProviderOpenAI: &fakeShadowConnector{
		provider: models.ShadowProviderOpenAI,
		scan: func(sa.Connection) *sa.Result {
			t.Error("a cancelled scan runs no connector")
			return &sa.Result{}
		},
	}}
	fastShadowScanner(h, connectors).RunOnce(context.Background())
	assert.Empty(t, h.applied)
	assert.Nil(t, h.finished)
}

// A scan given up after its workers stopped fails the connections its last
// attempt left queued or running, and keeps the ones that finished.
func TestShadowScannerGiveUpFailsUnfinishedConnections(t *testing.T) {
	a, b, q := shadowTestConnection(t, "a"), shadowTestConnection(t, "b"), shadowTestConnection(t, "queued")
	scan := models.ShadowScan{
		ID: uuid.New(), OUID: shadowTestOU, Attempts: shadowScanMaxAttempts + 1,
		ConnectionIDs: []string{a.ID.String(), b.ID.String(), q.ID.String()},
		Results: []models.ShadowScanConnectionResult{
			{ConnectionID: a.ID.String(), ConnectionName: "a", Outcome: shadowConnectionOK, FindingsCount: 3},
			{ConnectionID: b.ID.String(), ConnectionName: "b", Outcome: models.ShadowConnectionRunning, FindingsCount: 0},
			{ConnectionID: q.ID.String(), Outcome: models.ShadowConnectionQueued},
		},
	}
	h := newShadowScannerHarness(scan, a, b, q)
	NewShadowAgentScanner(h.repo, ShadowConnectors{}, shadowTestKey, discardLogger()).RunOnce(context.Background())
	require.NotNil(t, h.finished)
	require.Len(t, h.finished.Results, 3)
	assert.Equal(t, shadowConnectionOK, h.finished.Results[0].Outcome)
	assert.Equal(t, shadowConnectionFailed, h.finished.Results[1].Outcome)
	assert.Contains(t, h.finished.Results[1].Message, "gave up")
	assert.Equal(t, shadowConnectionFailed, h.finished.Results[2].Outcome)
	assert.Equal(t, "queued", h.finished.Results[2].ConnectionName, "a queued connection gets its name")
	assert.Equal(t, models.ShadowProviderOpenAI, h.finished.Results[2].Provider)
	assert.Equal(t, 3, h.finished.FindingsCount, "agents of the connections that finished are counted")
}

// ---- cancel ----

func cancelFixture(status string) (*repomocks.ShadowAgentRepositoryMock, *models.ShadowScan) {
	scan := &models.ShadowScan{ID: uuid.New(), OUID: shadowTestOU, Status: status, ConnectionIDs: []string{uuid.NewString()}}
	repo := &repomocks.ShadowAgentRepositoryMock{
		GetScanFunc: func(_ context.Context, ouID string, id uuid.UUID) (*models.ShadowScan, error) {
			if ouID != shadowTestOU || id != scan.ID {
				return nil, gorm.ErrRecordNotFound
			}
			s := *scan
			return &s, nil
		},
	}
	return repo, scan
}

func TestShadowCancelQueuedAndRunningScans(t *testing.T) {
	for _, status := range []string{models.ShadowScanQueued, models.ShadowScanRunning} {
		t.Run(status, func(t *testing.T) {
			repo, scan := cancelFixture(status)
			repo.CancelActiveScanFunc = func(_ context.Context, ouID string, id uuid.UUID, _ time.Time) (bool, error) {
				assert.Equal(t, shadowTestOU, ouID)
				assert.Equal(t, scan.ID, id)
				scan.Status = models.ShadowScanFailed
				scan.Results = []models.ShadowScanConnectionResult{{
					ConnectionID: scan.ConnectionIDs[0], Outcome: shadowConnectionFailed, Message: models.ShadowConnectionCancelledMessage,
				}}
				return true, nil
			}
			// CancelPendingScan is nil: reaching it panics.
			got, err := newShadowSvc(repo).CancelRunnerScan(context.Background(), shadowTestOU, scan.ID)
			require.NoError(t, err)
			assert.Equal(t, models.ShadowScanFailed, got.Status)
			assert.Equal(t, models.ShadowConnectionCancelledMessage, got.Results[0].Message)
			require.Len(t, repo.CancelActiveScanCalls(), 1)
		})
	}
}

func TestShadowCancelFinishedScanIsRefused(t *testing.T) {
	for _, status := range []string{models.ShadowScanSucceeded, models.ShadowScanPartial, models.ShadowScanFailed} {
		t.Run(status, func(t *testing.T) {
			repo, scan := cancelFixture(status)
			// Every cancel func is nil: reaching one panics.
			_, err := newShadowSvc(repo).CancelRunnerScan(context.Background(), shadowTestOU, scan.ID)
			require.ErrorIs(t, err, utils.ErrShadowScanFinished)
		})
	}

	// It finished between the read and the cancel.
	repo, scan := cancelFixture(models.ShadowScanRunning)
	repo.CancelActiveScanFunc = func(context.Context, string, uuid.UUID, time.Time) (bool, error) {
		scan.Status = models.ShadowScanSucceeded
		return false, nil
	}
	_, err := newShadowSvc(repo).CancelRunnerScan(context.Background(), shadowTestOU, scan.ID)
	require.ErrorIs(t, err, utils.ErrShadowScanFinished)

	_, err = newShadowSvc(repo).CancelRunnerScan(context.Background(), "ou-other", scan.ID)
	require.ErrorIs(t, err, utils.ErrShadowScanNotFound, "another org's scan is not found")

	// Every connection finished; the worker is recording the scan.
	repo, scan = cancelFixture(models.ShadowScanRunning)
	repo.CancelActiveScanFunc = func(context.Context, string, uuid.UUID, time.Time) (bool, error) { return false, nil }
	_, err = newShadowSvc(repo).CancelRunnerScan(context.Background(), shadowTestOU, scan.ID)
	require.ErrorIs(t, err, utils.ErrShadowScanFinished)
}
