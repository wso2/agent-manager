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

package agent

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestList_TextOutput(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = false
	status := "active"
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.AgentListResponse{
		Agents: []amsvc.AgentResponse{
			{Name: "order-triage", DisplayName: "Order Triage", Status: &status, CreatedAt: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)},
		},
		Limit: 20, Offset: 0, Total: 1,
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "order-triage") {
		t.Errorf("output should contain agent name, got %q", got)
	}
	if !strings.Contains(got, "Order Triage") {
		t.Errorf("output should contain display name, got %q", got)
	}
	if !strings.Contains(got, "active") {
		t.Errorf("output should contain status, got %q", got)
	}
}

func TestList_JSONOutput(t *testing.T) {
	io, out, _ := newTestIO(true)
	status := "active"
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.AgentListResponse{
		Agents: []amsvc.AgentResponse{
			{Name: "order-triage", DisplayName: "Order Triage", Status: &status, CreatedAt: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)},
		},
		Limit: 20, Offset: 0, Total: 1,
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	if _, ok := env["data"]; !ok {
		t.Fatal("expected data key in JSON envelope")
	}
}
