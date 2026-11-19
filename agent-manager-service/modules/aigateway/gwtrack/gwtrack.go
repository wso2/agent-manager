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

// Package gwtrack reports the gateway's API requests as feature usage through
// the Runtime's Track.
package gwtrack

import (
	"context"
	"maps"
	"net/http"
	"sync"
)

type dimsKey struct{}

type dims struct {
	mu   sync.Mutex
	vals map[string]any
}

// Wrap reports each request h serves as feature, with dimensions plus what h
// adds with Set, by calling track once h has returned.
func Wrap(track func(context.Context, string, map[string]any), feature string, dimensions map[string]any, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := &dims{vals: maps.Clone(dimensions)}
		if d.vals == nil {
			d.vals = map[string]any{}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), dimsKey{}, d)))
		d.mu.Lock()
		defer d.mu.Unlock()
		track(r.Context(), feature, d.vals)
	}
}

// Set adds a dimension a handler learns while it runs; it wins over the
// route's own. It does nothing outside a Wrap request.
func Set(ctx context.Context, key string, value any) {
	if d, ok := ctx.Value(dimsKey{}).(*dims); ok {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.vals[key] = value
	}
}
