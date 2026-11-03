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

package reconcile

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// failingKind is an inline kind whose every render fails with err.
type failingKind struct {
	stubKind
	err error
}

func (k failingKind) Render(context.Context, models.ArtifactRef) (Rendered, error) {
	return Rendered{}, k.err
}

func testClaim() models.ClaimedRender {
	return models.ClaimedRender{
		Placement: models.Placement{
			ArtifactRef: models.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: models.KindMCPMapping},
			GatewayUUID: uuid.New(),
		},
		ClaimedAt: time.Now(),
	}
}

// renderRepo hands out claim as the artifact's one due row and records the outcome written back for it.
func renderRepo(claim models.ClaimedRender, recordErr error) (*repomocks.DeploymentRepositoryMock, *[]repositories.RenderOutcome) {
	var recorded []repositories.RenderOutcome
	repo := &repomocks.DeploymentRepositoryMock{
		QueueRenderFunc: func(context.Context, *gorm.DB, string, uuid.UUID, []uuid.UUID, string, repositories.QueueScope) (int64, error) {
			return 1, nil
		},
		ClaimDueRendersOfFunc: func(context.Context, string, uuid.UUID, []uuid.UUID, time.Time, time.Duration) ([]models.ClaimedRender, error) {
			return []models.ClaimedRender{claim}, nil
		},
		ClaimDueRendersFunc: func(context.Context, time.Time, time.Duration, int) ([]models.ClaimedRender, error) {
			return []models.ClaimedRender{claim}, nil
		},
		RecordRenderOutcomeFunc: func(_ context.Context, _ models.ClaimedRender, outcome repositories.RenderOutcome) error {
			recorded = append(recorded, outcome)
			return recordErr
		},
	}
	return repo, &recorded
}

func newRenderCore(repo repositories.DeploymentRepository, logger *slog.Logger, kind Kind) *Core {
	return NewCore(nil, repo, nil, Kinds{kind}, Config{Lease: time.Minute, AttemptBudget: 3}, logger)
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRenderClaimReturnsAFailedRenderAfterRecordingIt(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		outcome repositories.RenderOutcomeKind
	}{
		{name: "retry", err: errors.New("connection reset"), outcome: repositories.RenderRetry},
		{name: "give up", err: Permanent(errors.New("invalid config")), outcome: repositories.RenderGiveUp},
		{name: "waiting", err: InputNotReady("waiting for the endpoint"), outcome: repositories.RenderWaiting},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claim := testClaim()
			repo, recorded := renderRepo(claim, nil)
			core := newRenderCore(repo, discardLogger(), failingKind{err: tt.err})

			err := core.renderClaim(context.Background(), claim)

			var failed *RenderFailedError
			require.ErrorAs(t, err, &failed)
			assert.Equal(t, tt.outcome, failed.Outcome)
			assert.ErrorIs(t, err, tt.err)
			require.Len(t, *recorded, 1)
			assert.Equal(t, tt.outcome, (*recorded)[0].Outcome)
		})
	}
}

func TestRenderClaimDropsASupersededOutcome(t *testing.T) {
	claim := testClaim()
	repo, _ := renderRepo(claim, repositories.ErrRenderSuperseded)
	core := newRenderCore(repo, discardLogger(), failingKind{err: errors.New("connection reset")})

	assert.NoError(t, core.renderClaim(context.Background(), claim))
}

func TestRenderClaimReturnsAFailedWriteBackUnchanged(t *testing.T) {
	claim := testClaim()
	dbErr := errors.New("connection refused")
	repo, _ := renderRepo(claim, dbErr)
	core := newRenderCore(repo, discardLogger(), failingKind{err: errors.New("connection reset")})

	err := core.renderClaim(context.Background(), claim)

	assert.ErrorIs(t, err, dbErr)
	var failed *RenderFailedError
	assert.NotErrorAs(t, err, &failed)
}

func TestMarkStalePassesOnAFailedInlineRender(t *testing.T) {
	claim := testClaim()
	repo, _ := renderRepo(claim, nil)
	renderErr := Permanent(errors.New("invalid config"))
	core := newRenderCore(repo, discardLogger(), failingKind{err: renderErr})

	err := core.MarkStale(context.Background(), models.KindMCPMapping, claim.OUID, claim.ArtifactUUID, nil, "scopes changed")

	var failed *RenderFailedError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, repositories.RenderGiveUp, failed.Outcome)
	assert.ErrorIs(t, err, renderErr)
}

func TestMarkStaleLeavesARenderWaitingForAnInputToTheWorker(t *testing.T) {
	claim := testClaim()
	repo, recorded := renderRepo(claim, nil)
	core := newRenderCore(repo, discardLogger(), failingKind{err: InputNotReady("waiting for the endpoint")})

	err := core.MarkStale(context.Background(), models.KindMCPMapping, claim.OUID, claim.ArtifactUUID, nil, "scopes changed")

	assert.NoError(t, err)
	require.Len(t, *recorded, 1)
	assert.Equal(t, repositories.RenderWaiting, (*recorded)[0].Outcome)
}

func TestWorkerDoesNotReportAFailedRenderAsAnError(t *testing.T) {
	claim := testClaim()
	repo, recorded := renderRepo(claim, nil)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	worker := NewWorker(newRenderCore(repo, logger, failingKind{err: errors.New("connection reset")}), logger)

	worker.RunOnce(context.Background())
	err := worker.RunOnceFor(context.Background(), claim.OUID, claim.ArtifactUUID)

	assert.NoError(t, err)
	assert.Empty(t, logs.String(), "nothing is logged at error level")
	assert.Len(t, *recorded, 2, "both passes recorded the outcome")
}
