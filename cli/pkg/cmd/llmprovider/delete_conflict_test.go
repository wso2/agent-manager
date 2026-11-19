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

// TestDelete_ConflictSurfacesServerMessage is the regression test for "the provider
// cannot be deleted". The server has always answered a blocked delete with a 409 and
// an actionable message, but deleteLLMProvider declared no 409 in the spec, so the
// generated client had no field to decode it into and the CLI printed the flatly
// untrue "server returned 409 with no JSON body".
func TestDelete_ConflictSurfacesServerMessage(t *testing.T) {
	ios, out, _ := newTestIO(true)
	const serverMessage = "cannot delete LLM provider: it has associated LLM proxies. " +
		"Please delete all proxies before deleting the provider"
	clientFn, _, closeFn := newTestClient(t, http.StatusConflict, aigateway.ErrorResponse{
		Code:    "CONFLICT",
		Message: serverMessage,
	})
	defer closeFn()

	err := runDelete(context.Background(), &DeleteOptions{
		IO: ios, Prompter: &fakePrompter{}, Client: clientFn, Scope: baseScope(),
		Org: "acme", Provider: "openai", Yes: true,
	})
	if err == nil {
		t.Fatal("expected an error for 409")
	}

	rendered := out.String()
	if strings.Contains(rendered, "no JSON body") {
		t.Errorf("the 409 body was discarded; envelope=%s", rendered)
	}
	env := decodeEnvelope(t, rendered)
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "CONFLICT" {
		t.Errorf("code = %v, want CONFLICT", errBody["code"])
	}
	if msg, _ := errBody["message"].(string); !strings.Contains(msg, "delete all proxies") {
		t.Errorf("message = %q, want the server's actionable text", msg)
	}
}

// A 403 masked identically before the spec declared it.
func TestDelete_ForbiddenSurfacesServerMessage(t *testing.T) {
	ios, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusForbidden, aigateway.ErrorResponse{
		Code:    "FORBIDDEN",
		Message: "missing llm-provider:delete permission",
	})
	defer closeFn()

	err := runDelete(context.Background(), &DeleteOptions{
		IO: ios, Prompter: &fakePrompter{}, Client: clientFn, Scope: baseScope(),
		Org: "acme", Provider: "openai", Yes: true,
	})
	if err == nil {
		t.Fatal("expected an error for 403")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "FORBIDDEN" {
		t.Errorf("code = %v, want FORBIDDEN; envelope=%s", errBody["code"], out.String())
	}
}
