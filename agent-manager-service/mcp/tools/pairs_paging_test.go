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

package tools

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// pagedMock reports more projects and agents than it returns, as a large
// organization does once it outgrows one page.
type pagedMock struct {
	*MockToolsetHandler
}

func (m pagedMock) ListProjects(ctx context.Context, ouID string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	return []*models.ProjectResponse{{Name: testProjectName}}, 3, nil
}

func (m pagedMock) ListAgents(ctx context.Context, ouID string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	return []*models.AgentResponse{{Name: testAgentName}}, 7, nil
}

// One page must never look like the whole organization: the pairs tool has to
// say when projects or agents were left out.
func TestListProjectAgentPairsReportsPartialPage(t *testing.T) {
	cases := []struct {
		name     string
		toolsets func(*MockToolsetHandler) *Toolsets
		complete bool
	}{
		{"single page", func(m *MockToolsetHandler) *Toolsets {
			return &Toolsets{ProjectToolset: m, AgentToolset: m, EnvironmentToolset: m}
		}, true},
		{"more on later pages", func(m *MockToolsetHandler) *Toolsets {
			p := pagedMock{m}
			return &Toolsets{ProjectToolset: p, AgentToolset: p, EnvironmentToolset: m}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session := setupTestServerWithToolsets(t, tc.toolsets(NewMockToolsetHandler()))
			res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: "list_project_agent_pairs", Arguments: map[string]any{}})
			if err != nil || res.IsError {
				t.Fatalf("CallTool: err=%v result=%+v", err, res)
			}
			var out listProjectAgentPairsOutput
			if err := json.Unmarshal([]byte(res.Content[0].(*gomcp.TextContent).Text), &out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out.Complete != tc.complete {
				t.Fatalf("complete: got %v, want %v (%+v)", out.Complete, tc.complete, out)
			}
			if !tc.complete && (!out.MoreProjects || len(out.ProjectsWithMoreAgents) != 1 || out.Note == "") {
				t.Errorf("partial page not described: %+v", out)
			}
		})
	}
}

// Offsets are converted to int32 for ListAgents, so one above math.MaxInt32 is refused rather than wrapped.
func TestOffsetsAboveInt32AreRefused(t *testing.T) {
	session := setupTestServerWithToolsets(t, &Toolsets{
		ProjectToolset: NewMockToolsetHandler(), AgentToolset: NewMockToolsetHandler(), EnvironmentToolset: NewMockToolsetHandler(),
	})
	for tool, args := range map[string]map[string]any{
		"list_project_agent_pairs": {"agent_offset": math.MaxInt32 + 1},
		"list_agents":              {"project_name": testProjectName, "offset": math.MaxInt32 + 1},
	} {
		res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
		if err == nil && (res == nil || !res.IsError) {
			t.Errorf("%s: an offset above %d was accepted", tool, math.MaxInt32)
		}
	}
}

// Both of list_project_agent_pairs' pages share the bounds, and the error names
// the argument. project_offset had no upper bound, and one near MaxInt64
// overflowed offset+limit in the project service and panicked on the slice.
func TestListProjectAgentPairsBoundsBothPages(t *testing.T) {
	session, mock := setupTestServer(t)
	for arg, value := range map[string]int{
		"project_offset": math.MaxInt32 + 1,
		"agent_offset":   math.MaxInt32 + 1,
		"project_limit":  utils.MaxLimit + 1,
		"agent_limit":    0,
	} {
		res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
			Name: "list_project_agent_pairs", Arguments: map[string]any{arg: value},
		})
		if err != nil {
			t.Fatalf("%s: CallTool: %v", arg, err)
		}
		if !res.IsError {
			t.Errorf("%s=%d was accepted", arg, value)
			continue
		}
		if text := res.Content[0].(*gomcp.TextContent).Text; !strings.HasPrefix(text, arg+" must be between") {
			t.Errorf("%s: error does not name the argument: %s", arg, text)
		}
	}
	if _, listed := mock.calls["ListProjects"]; listed {
		t.Error("an out-of-range argument reached ListProjects")
	}
}

func TestResolvePaginationOffsetRange(t *testing.T) {
	for offset, wantErr := range map[int]bool{-1: true, 0: false, math.MaxInt32: false, math.MaxInt32 + 1: true} {
		o := offset
		if _, got, err := resolvePagination(nil, &o); (err != nil) != wantErr || (!wantErr && got != offset) {
			t.Errorf("offset %d: got (%d, %v), wantErr %v", offset, got, err, wantErr)
		}
	}
}

// A page reached with an offset left earlier projects or agents out, so even the last page is not the whole
// organization.
func TestListProjectAgentPairsOffsetPageIsNotComplete(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"project offset": {"project_offset": 1},
		"agent offset":   {"agent_offset": 1},
	} {
		t.Run(name, func(t *testing.T) {
			m := NewMockToolsetHandler()
			session := setupTestServerWithToolsets(t, &Toolsets{ProjectToolset: m, AgentToolset: m, EnvironmentToolset: m})
			res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: "list_project_agent_pairs", Arguments: args})
			if err != nil || res.IsError {
				t.Fatalf("CallTool: err=%v result=%+v", err, res)
			}
			var out listProjectAgentPairsOutput
			if err := json.Unmarshal([]byte(res.Content[0].(*gomcp.TextContent).Text), &out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out.Complete || out.MoreProjects || len(out.ProjectsWithMoreAgents) != 0 || out.Note == "" {
				t.Errorf("an offset page with nothing after it: %+v", out)
			}
		})
	}
}
