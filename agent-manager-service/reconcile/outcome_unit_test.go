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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

var claimedAt = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func claimAfter(attempts int, lastDelay time.Duration) models.ClaimedRender {
	return models.ClaimedRender{AttemptCount: attempts, ClaimedAt: claimedAt, LastDelay: lastDelay}
}

// Waiting for an input never spends the budget, and backs off by doubling from 5 s up to the 5 min cap.
func TestRenderOutcomeWaitingBacksOffToTheCap(t *testing.T) {
	notReady := fmt.Errorf("render: %w", InputNotReady("waiting for the agent's endpoint"))
	lastDelay := time.Duration(0)
	var delays []time.Duration
	for range 10 {
		outcome := renderOutcome(notReady, "waiting for the agent's endpoint", claimAfter(29, lastDelay), 30)

		assert.Equal(t, repositories.RenderWaiting, outcome.Outcome, "never given up, whatever the attempt count")
		assert.Equal(t, "waiting for the agent's endpoint", outcome.Reason)
		lastDelay = outcome.NextAttemptAt.Sub(claimedAt)
		delays = append(delays, lastDelay)
	}

	assert.Equal(t, []time.Duration{
		5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 160 * time.Second,
		5 * time.Minute, 5 * time.Minute, 5 * time.Minute, 5 * time.Minute,
	}, delays)
}

// A transient error spends the budget and gives up on the 30th failure.
func TestRenderOutcomeTransientErrorSpendsTheBudget(t *testing.T) {
	transient := errors.New("connection reset")

	first := renderOutcome(transient, "connection reset", claimAfter(0, 0), 30)
	assert.Equal(t, repositories.RenderRetry, first.Outcome)
	assert.Equal(t, claimedAt.Add(5*time.Second), *first.NextAttemptAt)

	assert.Equal(t, repositories.RenderRetry, renderOutcome(transient, "", claimAfter(28, 0), 30).Outcome)
	last := renderOutcome(transient, "connection reset", claimAfter(29, 0), 30)
	assert.Equal(t, repositories.RenderGiveUp, last.Outcome)
	assert.Equal(t, "connection reset", last.Reason)
}

func TestRenderOutcomePermanentGivesUpAtOnce(t *testing.T) {
	outcome := renderOutcome(Permanent(errors.New("invalid policy")), "invalid policy", claimAfter(0, 0), 30)
	assert.Equal(t, repositories.RenderGiveUp, outcome.Outcome)
}

type describingKind struct{ Kind }

func (describingKind) DescribeRenderError(error) string { return "the security config is invalid" }

func TestRenderReason(t *testing.T) {
	assert.Equal(t, "waiting for the agent's endpoint",
		renderReason(nil, fmt.Errorf("wrapped: %w", InputNotReady("waiting for the agent's endpoint"))))
	assert.Equal(t, "boom", renderReason(nil, errors.New("boom")))
	assert.Equal(t, "the security config is invalid", renderReason(describingKind{}, errors.New("boom")))
}

func TestErrors(t *testing.T) {
	assert.ErrorIs(t, InputNotReady("x"), ErrInputNotReady)
	assert.False(t, IsPermanent(InputNotReady("x")))
	assert.True(t, IsPermanent(fmt.Errorf("render: %w", ErrRecordGone)))
	assert.ErrorIs(t, fmt.Errorf("render: %w", ErrRecordGone), ErrRecordGone)
	assert.NotErrorIs(t, Permanent(errors.New("invalid")), ErrRecordGone)
}
