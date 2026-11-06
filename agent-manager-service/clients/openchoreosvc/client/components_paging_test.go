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

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

// componentPages serves one ComponentList per cursor value: "" is the first page, and each
// page names the cursor of the next one, as OpenChoreo's continuation tokens do.
func componentPages(t *testing.T, pages map[string]gen.ComponentList) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := pages[r.URL.Query().Get("cursor")]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"unknown cursor"}`))
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(page))
	})
}

func cursorTo(next string) gen.Pagination {
	return gen.Pagination{NextCursor: &next}
}

// A project with more components than one page holds must list the agents on every page,
// with the terminating and other-product filters applied to each page, not just the first.
func TestListComponents_FollowsCursorAcrossPages(t *testing.T) {
	c := newTestClient(t, componentPages(t, map[string]gen.ComponentList{
		"": {
			Items:      []gen.Component{agentComponent(t, liveMeta("agent-1")), agentComponent(t, terminatingMeta("deleted-1"))},
			Pagination: cursorTo("page-2"),
		},
		"page-2": {
			Items:      []gen.Component{{Metadata: liveMeta("other-product-service")}, agentComponent(t, liveMeta("agent-2"))},
			Pagination: cursorTo("page-3"),
		},
		"page-3": {
			Items: []gen.Component{agentComponent(t, liveMeta("agent-3"))},
		},
	}))

	agents, err := c.ListComponents(context.Background(), "ou-acme", "proj")

	require.NoError(t, err)
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		names = append(names, a.Name)
	}
	assert.Equal(t, []string{"agent-1", "agent-2", "agent-3"}, names)
}

// A cursor that does not advance would repeat the same page until the page cap.
func TestListComponents_FailsOnStuckCursor(t *testing.T) {
	c := newTestClient(t, componentPages(t, map[string]gen.ComponentList{
		"":       {Items: []gen.Component{agentComponent(t, liveMeta("agent-1"))}, Pagination: cursorTo("page-2")},
		"page-2": {Items: []gen.Component{agentComponent(t, liveMeta("agent-2"))}, Pagination: cursorTo("page-2")},
	}))

	_, err := c.ListComponents(context.Background(), "ou-acme", "proj")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "repeated")
}

// A cursor that leads back to an earlier page, not just the last one, is a loop too.
func TestListComponents_FailsOnCursorCycle(t *testing.T) {
	c := newTestClient(t, componentPages(t, map[string]gen.ComponentList{
		"":       {Items: []gen.Component{agentComponent(t, liveMeta("agent-1"))}, Pagination: cursorTo("page-2")},
		"page-2": {Items: []gen.Component{agentComponent(t, liveMeta("agent-2"))}, Pagination: cursorTo("page-3")},
		"page-3": {Items: []gen.Component{agentComponent(t, liveMeta("agent-3"))}, Pagination: cursorTo("page-2")},
	}))

	agents, err := c.ListComponents(context.Background(), "ou-acme", "proj")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "repeated")
	assert.Nil(t, agents)
}

// Past the page cap the list is incomplete, and ListAgents would total and paginate the part it got,
// so it is an error rather than a truncated list.
func TestListComponents_FailsAtPageCap(t *testing.T) {
	pages := 0
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		require.NoError(t, json.NewEncoder(w).Encode(gen.ComponentList{
			Items:      []gen.Component{agentComponent(t, liveMeta(fmt.Sprintf("agent-%d", pages)))},
			Pagination: cursorTo(fmt.Sprintf("page-%d", pages+1)),
		}))
	}))

	agents, err := c.ListComponents(context.Background(), "ou-acme", "proj")

	require.Error(t, err)
	assert.Nil(t, agents)
	assert.Equal(t, maxListPages, pages, "stops at the cap")
}

// A failed later page must fail the list rather than return the earlier pages as if complete.
func TestListComponents_FailsWhenLaterPageFails(t *testing.T) {
	c := newTestClient(t, componentPages(t, map[string]gen.ComponentList{
		"": {Items: []gen.Component{agentComponent(t, liveMeta("agent-1"))}, Pagination: cursorTo("missing-page")},
	}))

	agents, err := c.ListComponents(context.Background(), "ou-acme", "proj")

	require.Error(t, err)
	assert.Nil(t, agents)
}
