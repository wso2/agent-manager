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
	"testing"

	"github.com/wso2/agent-manager/internal/amctl/clierr"
)

func TestGet_RejectsEmptyName(t *testing.T) {
	io, out, _ := newTestIO(true)

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "",
	})
	if err == nil {
		t.Fatal("expected error for empty agent name")
	}
	env := decodeEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error key, got %v", env)
	}
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}

func TestGet_RejectsSlashInName(t *testing.T) {
	io, out, _ := newTestIO(true)

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "foo/bar",
	})
	if err == nil {
		t.Fatal("expected error for slash in agent name")
	}
	env := decodeEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error key, got %v", env)
	}
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}
