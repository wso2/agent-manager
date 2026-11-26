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

package store

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// CursorKey is the last trace of a page sorted by start time, then trace ID.
type CursorKey struct {
	Order   string `json:"o"`
	StartNs int64  `json:"s"`
	TraceID string `json:"t"`
}

// EncodeCursor is k as an opaque cursor that starts with tag, the backend and its cursor version.
func EncodeCursor(tag string, k CursorKey) string {
	b, _ := json.Marshal(k)
	return tag + "." + base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor reads a cursor EncodeCursor made with tag for a list in order.
func DecodeCursor(tag, cursor, order string) (CursorKey, error) {
	payload, ok := strings.CutPrefix(cursor, tag+".")
	if !ok {
		return CursorKey{}, ErrInvalidCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return CursorKey{}, ErrInvalidCursor
	}
	var k CursorKey
	if json.Unmarshal(b, &k) != nil || k.TraceID == "" || k.Order != order {
		return CursorKey{}, ErrInvalidCursor
	}
	return k, nil
}

// After reports whether a trace starting at startNs with traceID comes after k in k's order.
func (k CursorKey) After(startNs int64, traceID string) bool {
	if k.Order == "asc" {
		return startNs > k.StartNs || startNs == k.StartNs && traceID > k.TraceID
	}
	return startNs < k.StartNs || startNs == k.StartNs && traceID < k.TraceID
}
