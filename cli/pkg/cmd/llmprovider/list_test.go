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

package llmprovider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
)

func sampleListResponse() aigateway.LLMProviderListResponse {
	return aigateway.LLMProviderListResponse{
		Providers: []aigateway.LLMProviderListItem{
			{
				Uuid:     "11111111-1111-1111-1111-111111111111",
				Id:       "openai",
				Name:     "OpenAI",
				Template: "openai",
				Status:   aigateway.LLMProviderListItemStatus("deployed"),
			},
		},
		Total:  1,
		Limit:  20,
		Offset: 0,
	}
}

func TestList_SuccessJSON(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "GET" {
		t.Errorf("method = %q, want GET", captured.method)
	}
	if captured.path != "/orgs/acme/llm-providers" {
		t.Errorf("path = %q, want /orgs/acme/llm-providers", captured.path)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	providers := data["providers"].([]any)
	if len(providers) != 1 {
		t.Fatalf("providers len = %d, want 1", len(providers))
	}
	first := providers[0].(map[string]any)
	if first["id"] != "openai" {
		t.Errorf("id = %v, want openai", first["id"])
	}
}

func TestList_RequestsPageSize50(t *testing.T) {
	io, _, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := captured.rawQuery; !strings.Contains(got, "limit=50") {
		t.Errorf("query = %q, want it to contain limit=50", got)
	}
}

func TestList_Table(t *testing.T) {
	io, out, _ := newTestIO(false)
	io.JSON = false
	clientFn, _, closeFn := newTestClient(t, http.StatusOK, sampleListResponse())
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"openai", "OpenAI", "deployed"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q; got:\n%s", want, got)
		}
	}
}

func TestList_ServerError(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusInternalServerError, aigateway.ErrorResponse{
		Code:    "INTERNAL",
		Message: "boom",
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	env := decodeEnvelope(t, out.String())
	if _, ok := env["error"]; !ok {
		t.Fatalf("expected error envelope, got %v", env)
	}
}
