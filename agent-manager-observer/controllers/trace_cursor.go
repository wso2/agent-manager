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
	"time"
)

// TraceCursor marks where a trace-list page stopped in its window and sort order.
// The API carries it as opaque base64url JSON.
type TraceCursor struct {
	// Rank is the upstream trace slots up to and including the cursor trace.
	// It only sizes the next fetch.
	Rank int `json:"r"`
	// Time is the cursor trace's start time. The next page skips traces on
	// the near side of it and keeps traces at exactly this time.
	Time time.Time `json:"t"`
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
	if c.Time.IsZero() {
		return nil, errors.New("controllers.DecodeTraceCursor: missing time")
	}
	return &c, nil
}

// beforeCursor reports whether start is strictly on the near side of cur:
// newer for desc, older for asc.
func beforeCursor(start time.Time, cur *TraceCursor, asc bool) bool {
	switch {
	case cur == nil:
		return false
	case asc:
		return start.Before(cur.Time)
	default:
		return start.After(cur.Time)
	}
}

// atCursor reports whether start is exactly the cursor time.
func atCursor(start time.Time, cur *TraceCursor) bool {
	return cur != nil && start.Equal(cur.Time)
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
