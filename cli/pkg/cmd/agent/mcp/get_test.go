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
	"net/http"
	"strings"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

const getUUID = "11111111-1111-1111-1111-111111111111"

func getResponseFixture() aigateway.AgentMcpConfigDocument {
	desc := "primary github binding"
	uid := mustUUID(getUUID)
	url := "https://default.mcp"
	policies := []aigateway.LLMPolicy{{Name: "rate-limit", Version: "v1"}}
	vars := []aigateway.EnvironmentVariableConfig{
		{Key: "url", Name: "MCP_URL"},
		{Key: "apikey", Name: "MCP_KEY"},
	}
	return aigateway.AgentMcpConfigDocument{
		Metadata: aigateway.ProjectScopedDocumentMetadata{Name: "primary", Project: "triage", Uid: &uid},
		Spec: aigateway.AgentMcpConfigSpec{
			ProxyName:            "github",
			Description:          &desc,
			Configuration:        &aigateway.EnvProviderConfiguration{Policies: &policies},
			EnvironmentVariables: &vars,
		},
		Status: &aigateway.AgentConfigStatus{Url: &url},
	}
}

func newGetOpts(io *iostreams.IOStreams, client clientFn) *GetOptions {
	return &GetOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot", Name: "primary"}
}

func Test_runGet_humanOutput(t *testing.T) {
	io, out, _ := newTestIO(false)
	io.SetTerminal(true, true, true)
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("GET", getUUID): okJSON(getResponseFixture()),
	})
	defer cleanup()

	if err := runGet(context.Background(), newGetOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"primary", "primary github binding",
		"PROXY", "URL",
		"github", "https://default.mcp",
		"MCP_URL", "MCP_KEY",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
}

func Test_runGet_json(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("GET", getUUID): okJSON(getResponseFixture()),
	})
	defer cleanup()

	if err := runGet(context.Background(), newGetOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, out.String())["data"].(map[string]any)
	if meta := data["metadata"].(map[string]any); meta["name"] != "primary" {
		t.Errorf("metadata.name = %v, want primary", meta["name"])
	}
	if spec := data["spec"].(map[string]any); spec["proxyName"] != "github" {
		t.Errorf("spec.proxyName = %v, want github", spec["proxyName"])
	}
}

func Test_runGet_nameNotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	// Only list is stubbed; if get() were called the mock returns NOT_STUBBED.
	client, cleanup := newClient(t, map[string]route{
		listPath: okJSON(listResponse(mcpItem("other", getUUID))),
	})
	defer cleanup()

	opts := newGetOpts(io, client)
	opts.Name = "primary"
	if err := runGet(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	errBody := decodeEnvelope(t, out.String())["error"].(map[string]any)
	if errBody["code"] != clierr.NotFound {
		t.Fatalf("code = %v, want %s", errBody["code"], clierr.NotFound)
	}
}

func Test_runGet_detailServerError(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("GET", getUUID): {status: http.StatusNotFound, body: map[string]any{"code": "MODEL_CONFIG_NOT_FOUND", "message": "gone"}},
	})
	defer cleanup()

	if err := runGet(context.Background(), newGetOpts(io, client)); err == nil {
		t.Fatal("expected error")
	}
	if status := decodeEnvelope(t, out.String())["error"].(map[string]any)["status"]; status.(float64) != 404 {
		t.Fatalf("status = %v, want 404", status)
	}
}
