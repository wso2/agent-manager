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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// scoreWindowPad widens the score lookup's window on each side, as the console does.
const scoreWindowPad = time.Second

// ErrScoresNotConfigured is returned for a score filter when no score client is set.
var ErrScoresNotConfigured = errors.New("score filters need AGENT_MANAGER_SERVICE_URL")

// ErrScoreLookup wraps a failed score lookup.
var ErrScoreLookup = errors.New("score lookup failed")

// ScoreClient reads trace scores from agent-manager-service.
type ScoreClient interface {
	// TraceScores returns the mean non-skipped score of each listed trace,
	// only evaluator's rows when it is set; a trace without one is absent or nil.
	TraceScores(ctx context.Context, org, project, agent string, start, end time.Time, traceIDs []string, evaluator string) (map[string]*float64, error)
}

// WithScoreClient sets the client score filters read from, and returns c.
func (c *TracingController) WithScoreClient(scores ScoreClient) *TracingController {
	c.scores = scores
	return c
}

// filterByScore looks up the scores of traces in one call and keeps those
// within the filter's bounds, in order. outOfRange holds the IDs it dropped.
func (c *TracingController) filterByScore(
	ctx context.Context,
	params TraceQueryParams,
	traces []observer.TraceInfo,
) (kept []observer.TraceInfo, outOfRange map[string]bool, err error) {
	if len(traces) == 0 {
		return traces, nil, nil
	}
	ids := make([]string, len(traces))
	for i, t := range traces {
		ids[i] = t.TraceID
	}
	scores, err := c.scores.TraceScores(ctx, params.Organization, derefString(params.Project), derefString(params.Agent),
		params.StartTime.Add(-scoreWindowPad), params.EndTime.Add(scoreWindowPad), ids, params.Filters.Evaluator)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrScoreLookup, err)
	}
	kept = make([]observer.TraceInfo, 0, len(traces))
	outOfRange = make(map[string]bool)
	for _, t := range traces {
		if matchesScore(scores[t.TraceID], params.Filters) {
			kept = append(kept, t)
		} else {
			outOfRange[t.TraceID] = true
		}
	}
	return kept, outOfRange, nil
}

// derefString is *s, or "" when s is nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
