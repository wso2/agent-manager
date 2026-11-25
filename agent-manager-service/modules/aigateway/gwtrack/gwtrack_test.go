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

package gwtrack_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtrack"
)

type trackCall struct {
	feature string
	dims    map[string]any
}

func recordTrack(calls *[]trackCall) func(context.Context, string, map[string]any) {
	return func(_ context.Context, feature string, dims map[string]any) {
		*calls = append(*calls, trackCall{feature: feature, dims: dims})
	}
}

func serve(h http.HandlerFunc) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	return rec
}

// Wrap reports the route's feature and dimensions once the handler has run.
func TestWrapTracksFeatureWithRouteDimensionsAfterHandler(t *testing.T) {
	var calls []trackCall
	handled := false
	h := gwtrack.Wrap(recordTrack(&calls), "gateway.list", map[string]any{"op": "list"}, func(w http.ResponseWriter, _ *http.Request) {
		handled = true
		assert.Empty(t, calls, "track must run after the handler returns")
		w.WriteHeader(http.StatusTeapot)
	})

	rec := serve(h)

	assert.True(t, handled)
	assert.Equal(t, http.StatusTeapot, rec.Code)
	require.Len(t, calls, 1)
	assert.Equal(t, "gateway.list", calls[0].feature)
	assert.Equal(t, map[string]any{"op": "list"}, calls[0].dims)
}

// Set adds dimensions and overrides the route's own for that request only.
func TestSetAddsAndOverridesDimensionsPerRequest(t *testing.T) {
	routeDims := map[string]any{"op": "create", "kind": "route"}
	var calls []trackCall
	first := true
	h := gwtrack.Wrap(recordTrack(&calls), "gateway.create", routeDims, func(_ http.ResponseWriter, r *http.Request) {
		if first {
			gwtrack.Set(r.Context(), "kind", "handler")
			gwtrack.Set(r.Context(), "provider", "openai")
			first = false
		}
	})

	serve(h)
	serve(h)

	require.Len(t, calls, 2)
	assert.Equal(t, map[string]any{"op": "create", "kind": "handler", "provider": "openai"}, calls[0].dims)
	assert.Equal(t, map[string]any{"op": "create", "kind": "route"}, calls[1].dims, "a request's Set must not leak into the next")
	assert.Equal(t, map[string]any{"op": "create", "kind": "route"}, routeDims, "the route's map must not be mutated")
}

// A route without dimensions still gets a usable map for Set.
func TestWrapWithNilDimensionsAcceptsSet(t *testing.T) {
	var calls []trackCall
	h := gwtrack.Wrap(recordTrack(&calls), "f", nil, func(_ http.ResponseWriter, r *http.Request) {
		gwtrack.Set(r.Context(), "k", 1)
	})

	serve(h)

	require.Len(t, calls, 1)
	assert.Equal(t, map[string]any{"k": 1}, calls[0].dims)
}

// Set outside a Wrap request does nothing and does not panic.
func TestSetOutsideWrapIsNoOp(t *testing.T) {
	assert.NotPanics(t, func() { gwtrack.Set(context.Background(), "k", "v") })
}
