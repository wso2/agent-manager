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
	"errors"
	"net/http"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func TestDelete_SuccessByHandle(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientFn, Scope: baseScope(),
		Org: "acme", Provider: "openai", Yes: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "DELETE" {
		t.Errorf("method = %q, want DELETE", captured.method)
	}
	if captured.path != "/orgs/acme/llm-providers/openai" {
		t.Errorf("path = %q, want /orgs/acme/llm-providers/openai", captured.path)
	}
	if prompter.calls != 1 {
		t.Errorf("prompter calls = %d, want 1", prompter.calls)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["name"] != "openai" || data["deleted"] != true {
		t.Errorf("data = %v, want {name=openai, deleted=true}", data)
	}
}

func TestDelete_ByUUID(t *testing.T) {
	io, _, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()

	uuid := "11111111-1111-1111-1111-111111111111"
	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: clientFn, Scope: baseScope(),
		Org: "acme", Provider: uuid, Yes: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.path != "/orgs/acme/llm-providers/"+uuid {
		t.Errorf("path = %q, want /orgs/acme/llm-providers/%s", captured.path, uuid)
	}
}

func TestDelete_YesSkipsPrompt(t *testing.T) {
	io, _, _ := newTestIO(false)
	clientFn, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientFn, Scope: baseScope(),
		Org: "acme", Provider: "openai", Yes: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.calls != 0 {
		t.Errorf("prompter calls = %d, want 0 with --yes", prompter.calls)
	}
	if !captured.called {
		t.Fatal("server should have been called")
	}
}

func TestDelete_NonTTYWithoutYes(t *testing.T) {
	io, out, _ := newTestIO(false)

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Provider: "openai", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error for non-TTY without --yes")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}

func TestDelete_NotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusNotFound, aigateway.ErrorResponse{
		Code:    "LLM_PROVIDER_NOT_FOUND",
		Message: "LLM provider not found",
	})
	defer closeFn()

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: clientFn, Scope: baseScope(),
		Org: "acme", Provider: "openai", Yes: true,
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "LLM_PROVIDER_NOT_FOUND" {
		t.Errorf("code = %v, want LLM_PROVIDER_NOT_FOUND", errBody["code"])
	}
}

func TestDelete_ConfirmationMismatch(t *testing.T) {
	io, out, _ := newTestIO(true)
	prompter := &fakePrompter{confirmDeletionErr: errors.New("confirmation mismatch")}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Provider: "openai", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error from confirmation mismatch")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}
