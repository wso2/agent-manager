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

package shadowagents

import (
	"context"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// outcomeRank orders outcomes from best to worst, so a call made in several
// scopes (projects, regions, workspaces) reports its worst outcome.
var outcomeRank = map[string]int{
	models.ShadowCallOK:               0,
	models.ShadowCallSkipped:          1,
	models.ShadowCallNotFound:         2,
	models.ShadowCallUnsupported:      3,
	models.ShadowCallRateLimited:      4,
	models.ShadowCallPermissionDenied: 5,
	models.ShadowCallAuthFailed:       6,
	models.ShadowCallFailed:           7,
}

// Calls records the outcome of each provider call during a scan.
type Calls struct {
	list     []models.ShadowScanCallResult
	index    map[string]int
	fails    map[string]int
	total    map[string]int
	observer CallObserver
}

// CallObserver gets a copy of a recorder's calls each time they change, so a
// scan can show its progress while it runs.
type CallObserver func([]models.ShadowScanCallResult)

type callObserverKey struct{}

// WithCallObserver returns ctx carrying observer, which a recorder created
// with NewCallsFor(ctx) calls after each change.
func WithCallObserver(ctx context.Context, observer CallObserver) context.Context {
	return context.WithValue(ctx, callObserverKey{}, observer)
}

// NewCalls creates an empty recorder.
func NewCalls() *Calls {
	return &Calls{index: map[string]int{}, fails: map[string]int{}, total: map[string]int{}}
}

// NewCallsFor creates an empty recorder that reports its changes to the
// observer ctx carries, if any.
func NewCallsFor(ctx context.Context) *Calls {
	c := NewCalls()
	c.observer, _ = ctx.Value(callObserverKey{}).(CallObserver)
	return c
}

// changed tells the observer the calls so far.
func (c *Calls) changed() {
	if c.observer != nil {
		c.observer(c.Results())
	}
}

// Record records one attempt of call (in one scope) that returned items
// items and err. Several records of the same call are merged: items add up,
// the worst outcome wins, and the message says how many scopes failed.
func (c *Calls) Record(call string, items int, err error) {
	outcome := OutcomeOf(err)
	msg := MessageOf(err)
	i, ok := c.index[call]
	if !ok {
		c.index[call] = len(c.list)
		c.list = append(c.list, models.ShadowScanCallResult{Call: call, Outcome: outcome, Message: msg, Items: items})
		c.total[call] = 1
		if err != nil {
			c.fails[call] = 1
		}
		c.changed()
		return
	}
	c.total[call]++
	r := &c.list[i]
	r.Items += items
	if err != nil {
		c.fails[call]++
		if outcomeRank[outcome] > outcomeRank[r.Outcome] || r.Message == "" {
			r.Outcome, r.Message = outcome, msg
		}
	}
	if c.fails[call] > 0 && c.total[call] > 1 {
		r.Message = fmt.Sprintf("%d of %d scopes failed; %s", c.fails[call], c.total[call], baseMessage(r.Message))
	}
	c.changed()
}

// Skip records a call that was not made, and why (e.g. "not selected in
// this connection", "no project key").
func (c *Calls) Skip(call, reason string) {
	if _, ok := c.index[call]; ok {
		return
	}
	c.index[call] = len(c.list)
	c.list = append(c.list, models.ShadowScanCallResult{Call: call, Outcome: models.ShadowCallSkipped, Message: reason})
	c.changed()
}

// Note adds a short explanation to a recorded call, after any message it
// has (for example what a listing left out). A call not recorded yet is
// left as it is.
func (c *Calls) Note(call, note string) {
	i, ok := c.index[call]
	if !ok || note == "" {
		return
	}
	r := &c.list[i]
	if r.Message == "" {
		r.Message = note
	} else {
		r.Message += "; " + note
	}
	c.changed()
}

// OK reports whether every record of call succeeded.
func (c *Calls) OK(call string) bool {
	i, ok := c.index[call]
	return ok && c.list[i].Outcome == models.ShadowCallOK
}

// Results returns the recorded outcomes in call order.
func (c *Calls) Results() []models.ShadowScanCallResult {
	out := make([]models.ShadowScanCallResult, len(c.list))
	copy(out, c.list)
	return out
}

// baseMessage strips a previous "n of m scopes failed; " prefix.
func baseMessage(m string) string {
	var a, b int
	var rest string
	if n, _ := fmt.Sscanf(m, "%d of %d scopes failed; ", &a, &b); n == 2 {
		prefix := fmt.Sprintf("%d of %d scopes failed; ", a, b)
		if len(m) >= len(prefix) {
			rest = m[len(prefix):]
			return rest
		}
	}
	return m
}
