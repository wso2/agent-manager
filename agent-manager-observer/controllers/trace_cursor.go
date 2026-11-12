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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

// TraceCursor names the trace a trace-list page stopped on. Traces are in
// page order: start time in the sort order, then trace ID ascending. The next
// page starts strictly after the cursor trace, so pages never overlap.
// The API carries it as opaque base64url JSON.
type TraceCursor struct {
	// Rank is the upstream trace slots up to and including the cursor trace.
	// It only sizes the next fetch.
	Rank int `json:"r"`
	// Time is the cursor trace's start time.
	Time time.Time `json:"t"`
	// ID is the cursor trace's ID. An empty ID sorts before every ID, so a
	// cursor without one keeps every trace at Time.
	ID string `json:"i,omitempty"`
}

// Encode returns the cursor's wire form.
func (c TraceCursor) Encode() string {
	// An int and a time.Time always marshal.
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeTraceCursor parses a cursor from Encode.
func DecodeTraceCursor(s string) (*TraceCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("controllers.DecodeTraceCursor: %w", err)
	}
	var c TraceCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("controllers.DecodeTraceCursor: %w", err)
	}
	if c.Rank < 0 {
		return nil, errors.New("controllers.DecodeTraceCursor: negative rank")
	}
	if c.Rank > maxCursorDepth {
		return nil, errors.New("controllers.DecodeTraceCursor: rank past depth cap")
	}
	if c.Time.IsZero() {
		return nil, errors.New("controllers.DecodeTraceCursor: missing time")
	}
	return &c, nil
}

// compareTraces orders traces by page order: start time in the sort order,
// then trace ID ascending.
func compareTraces(a, b store.Trace, asc bool) int {
	if c := a.StartTime.Compare(b.StartTime); c != 0 {
		if asc {
			return c
		}
		return -c
	}
	return strings.Compare(a.TraceID, b.TraceID)
}

// pastCursor reports whether t comes strictly after cur in page order. Every
// trace is past a nil cursor.
func pastCursor(t store.Trace, cur *TraceCursor, asc bool) bool {
	return cur == nil || compareTraces(t, store.Trace{StartTime: cur.Time, TraceID: cur.ID}, asc) > 0
}

// sortedTraces returns a copy of traces in page order. The upstream sorts by
// start time only, so traces with the same start time come back in any order.
func sortedTraces(traces []store.Trace, asc bool) []store.Trace {
	sorted := slices.Clone(traces)
	slices.SortFunc(sorted, func(a, b store.Trace) int { return compareTraces(a, b, asc) })
	return sorted
}

// settledLen is how many leading traces of a sorted response are settled:
// the response holds every trace at their start time. A response cut by its
// limit can hold only part of the traces at its last start time, so a page
// may stop only on a settled trace. complete means the response holds the
// whole window.
func settledLen(sorted []store.Trace, complete bool) int {
	n := len(sorted)
	if complete || n == 0 {
		return n
	}
	last := sorted[n-1].StartTime
	for n > 0 && sorted[n-1].StartTime.Equal(last) {
		n--
	}
	return n
}

// fetchSize is the upstream limit for a walk that needs n traces: one more,
// which only shows whether the n-th is settled, capped at maxCursorDepth.
func fetchSize(n int) int {
	return min(n+1, maxCursorDepth)
}

// rootlessSlots estimates the limit slots a response spent on traces whose
// root span is outside the window. A response short of its limit while the
// window holds more traces spent the rest on them.
func rootlessSlots(fetchLimit, returned, total int) int {
	if returned >= total {
		return 0
	}
	return max(fetchLimit-returned, 0)
}
