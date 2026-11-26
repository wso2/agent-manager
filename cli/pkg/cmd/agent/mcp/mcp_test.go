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

package mcp

import (
	"context"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
)

func listResponse(items ...aigateway.AgentMcpConfigDocument) aigateway.AgentMcpConfigListResponse {
	return aigateway.AgentMcpConfigListResponse{Count: int32(len(items)), List: items}
}

func mcpItem(name, id string) aigateway.AgentMcpConfigDocument {
	uid := mustUUID(id)
	return aigateway.AgentMcpConfigDocument{
		Metadata: aigateway.ProjectScopedDocumentMetadata{Name: name, Project: "triage", Uid: &uid},
		Spec:     aigateway.AgentMcpConfigSpec{ProxyName: "bound"},
	}
}

func Test_findMCPConfigByName(t *testing.T) {
	const uuidA = "11111111-1111-1111-1111-111111111111"
	t.Run("found", func(t *testing.T) {
		client, cleanup := newClient(t, map[string]route{
			listPath: okJSON(listResponse(mcpItem("shared", "22222222-2222-2222-2222-222222222222"), mcpItem("primary", uuidA))),
		})
		defer cleanup()
		c, _ := client(context.Background())
		id, found, err := findMCPConfigByName(context.Background(), c, "acme", "triage", "order-bot", "primary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("found = false, want true")
		}
		if id != mustUUID(uuidA) {
			t.Errorf("uuid = %v, want %s", id, uuidA)
		}
	})

	t.Run("not found returns no error", func(t *testing.T) {
		client, cleanup := newClient(t, map[string]route{
			listPath: okJSON(listResponse(mcpItem("other", uuidA))),
		})
		defer cleanup()
		c, _ := client(context.Background())
		_, found, err := findMCPConfigByName(context.Background(), c, "acme", "triage", "order-bot", "primary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false")
		}
	})
}
