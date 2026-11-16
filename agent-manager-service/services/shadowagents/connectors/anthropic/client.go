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

package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// platform is the Anthropic organization the connection reaches: the
// first-party Claude API with a workspace key for each workspace's routes.
type platform struct {
	// base is the expanded base URL.
	base string
	// wsAPI returns the client for a workspace's routes, or false when the
	// connection has no credential for it.
	wsAPI func(ws string) (*sa.API, bool)
}

// call returns the call name a result records, e.g. "GET /v1/agents".
func (p *platform) call(name string) string {
	return name
}

// wsURL is the URL of a workspace route; the workspace is named in the
// anthropic-workspace-id header (wsHeaders).
func (p *platform) wsURL(_, path string) string {
	return p.base + path
}

// wsHeaders returns the headers of a workspace route: the beta header (when
// given) and anthropic-workspace-id.
func (p *platform) wsHeaders(ws, beta string) http.Header {
	h := http.Header{}
	if beta != "" {
		h.Set(headerBeta, beta)
	}
	h.Set(headerWorkspace, ws)
	return h
}

// orgScope is the Finding.Scope of an organization-level finding.
func (p *platform) orgScope() string {
	return "organization"
}

// wsScope is the Finding.Scope of a workspace's finding.
func (p *platform) wsScope(ws string) string {
	return "workspace:" + ws
}

// pageFunc receives one page of items; returning false stops the listing.
type pageFunc[T any] func(items []T) bool

// errPageLimit reports a listing cut at the page bound: what was read is
// used, and the call says the result is incomplete.
func errPageLimit(n int) error {
	return &sa.CallError{Outcome: models.ShadowCallFailed, Message: fmt.Sprintf("stopped after %d pages; the result is incomplete", n)}
}

func errNoData() error {
	return sa.Unsupported("the response has no data list")
}

// listByCursor follows the page/next_page pagination of the Managed Agents
// list to the end, or maxPages. It returns the number of items read.
func listByCursor[T any](ctx context.Context, api *sa.API, u string, q url.Values, h http.Header, maxPages int,
	fn pageFunc[T],
) (int, error) {
	n := 0
	q = cloneQuery(q)
	for page := 1; ; page++ {
		var env cursorPage[T]
		if err := api.DoJSON(ctx, sa.Request{Method: http.MethodGet, URL: u, Query: q, Header: h}, &env); err != nil {
			return n, err
		}
		if env.Data == nil {
			return n, errNoData()
		}
		n += len(*env.Data)
		if !fn(*env.Data) {
			return n, nil
		}
		if env.NextPage == nil || *env.NextPage == "" || (env.HasMore != nil && !*env.HasMore) {
			return n, nil
		}
		if page >= maxPages {
			return n, errPageLimit(maxPages)
		}
		if q.Get("page") == *env.NextPage {
			return n, sa.Unsupported("the API returned the same page cursor twice")
		}
		q.Set("page", *env.NextPage)
	}
}

func cloneQuery(q url.Values) url.Values {
	out := url.Values{}
	for k, vs := range q {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
