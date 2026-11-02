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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The render retry backoff: the A2A card reconciler's 5 s doubling, with a longer cap. AMS's own agent deploy or
// promote marks a waiting row due at once, which is what makes the long cap safe.
const (
	baseBackoff = 5 * time.Second
	maxBackoff  = 5 * time.Minute
)

// renderOutcome is what a failed render attempt leaves on its row. Waiting for an input is uncounted and never
// gives up: each wait doubles the last one. Any other error is counted and gives up when the budget is spent, or at
// once when it is Permanent.
func renderOutcome(err error, reason string, claim models.ClaimedRender, budget int) repositories.RenderOutcome {
	switch {
	case errors.Is(err, ErrInputNotReady):
		next := claim.ClaimedAt.Add(min(max(2*claim.LastDelay, baseBackoff), maxBackoff))
		return repositories.RenderOutcome{Outcome: repositories.RenderWaiting, Reason: reason, NextAttemptAt: &next}
	case IsPermanent(err) || claim.AttemptCount+1 >= budget:
		return repositories.RenderOutcome{Outcome: repositories.RenderGiveUp, Reason: reason}
	default:
		next := claim.ClaimedAt.Add(utils.ExponentialBackoff(claim.AttemptCount, baseBackoff, maxBackoff))
		return repositories.RenderOutcome{Outcome: repositories.RenderRetry, Reason: reason, NextAttemptAt: &next}
	}
}

// renderReason is the render_reason a user is shown for err: what an input-not-ready render waits for, else the
// kind's description of the error, else the error.
func renderReason(kind Kind, err error) string {
	var waiting *inputNotReady
	if errors.As(err, &waiting) {
		return waiting.reason
	}
	if describer, ok := kind.(ErrorDescriber); ok {
		return describer.DescribeRenderError(err)
	}
	return err.Error()
}
