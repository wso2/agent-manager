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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func sampleProviderResponse() aigateway.LlmProviderDocument {
	return aigateway.LlmProviderDocument{
		ApiVersion: aigateway.LlmProviderDocumentApiVersion("ai-control-plane.choreolabs.com/v1"),
		Kind:       aigateway.LlmProviderDocumentKindLlmProvider,
		Metadata:   aigateway.DocumentMetadata{Name: "openai"},
		Spec: aigateway.LlmProviderSpec{
			DisplayName: "OpenAI", Version: "v1.0", Context: "/", Template: "openai",
		},
	}
}

// specOf returns the spec object of a captured create document.
func specOf(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal body: %v\nbody=%s", err, raw)
	}
	spec, ok := doc["spec"].(map[string]any)
	if !ok {
		t.Fatalf("body has no spec object: %s", raw)
	}
	return spec
}

// authValue digs out upstream.main.auth.value from a captured create body.
func authValue(t *testing.T, raw []byte) (string, bool) {
	t.Helper()
	up, ok := specOf(t, raw)["upstream"].(map[string]any)
	if !ok {
		return "", false
	}
	main, ok := up["main"].(map[string]any)
	if !ok {
		return "", false
	}
	auth, ok := main["auth"].(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := auth["value"].(string)
	return v, ok
}

func TestCreate_SuccessWithAPIKey(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: unreachableClient, ProviderClient: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.method != "POST" {
		t.Errorf("method = %q, want POST", captured.method)
	}
	if captured.path != "/llm-providers" {
		t.Errorf("path = %q, want /llm-providers", captured.path)
	}
	if v, ok := authValue(t, captured.body); !ok || v != "sk-test" {
		t.Errorf("upstream auth value = %q (ok=%v), want sk-test", v, ok)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if name := data["metadata"].(map[string]any)["name"]; name != "openai" {
		t.Errorf("metadata.name = %v, want openai", name)
	}
}

func TestCreate_MinimalDefersToTemplate(t *testing.T) {
	io, _, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	// No key, no upstream overrides, no explicit auth flags: the request must
	// not carry an upstream auth block — the template supplies it.
	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: unreachableClient, ProviderClient: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := authValue(t, captured.body); ok {
		t.Errorf("expected no upstream auth block, body=%s", captured.body)
	}
}

func TestCreate_APIKeyStdin(t *testing.T) {
	io, _, _ := newTestIOWithStdin(true, "sk-stdin\n")
	clientFn, captured, closeFn := newTestClient(t, http.StatusCreated, sampleProviderResponse())
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: unreachableClient, ProviderClient: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKeyStdin: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := authValue(t, captured.body); !ok || v != "sk-stdin" {
		t.Errorf("upstream auth value = %q (ok=%v), want sk-stdin", v, ok)
	}
}

func TestCreate_EmptyStdinKey(t *testing.T) {
	io, out, _ := newTestIOWithStdin(true, "   \n")
	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: unreachableClient, ProviderClient: unreachableClient, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKeyStdin: true,
	})
	if err == nil {
		t.Fatal("expected error for empty stdin key")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}

func TestCreate_Conflict(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusConflict, aigateway.ErrorResponse{
		Code:    "LLM_PROVIDER_ALREADY_EXISTS",
		Message: "provider 'openai' already exists",
	})
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: unreachableClient, ProviderClient: clientFn, Org: "acme", Scope: baseScope(),
		ID: "openai", DisplayName: "OpenAI", Version: "v1", Context: "/",
		Template: "openai", AuthType: "api-key", APIKey: "sk-test",
	})
	if err == nil {
		t.Fatal("expected error for 409")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "LLM_PROVIDER_ALREADY_EXISTS" {
		t.Errorf("code = %v, want LLM_PROVIDER_ALREADY_EXISTS", errBody["code"])
	}
}

// --- flag-parsing / validation via the cobra tree ---

func runTreeExpectViolation(t *testing.T, args []string, wants ...string) {
	t.Helper()
	ios, out, _ := newTestIO(true)
	cmd := testLLMProviderCmd(t, ios, nil)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected validation error")
	}
	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != clierr.InvalidFlag {
		t.Fatalf("code = %v, want %s", errMap["code"], clierr.InvalidFlag)
	}
	additional := errMap["additionalData"].(map[string]any)
	details, ok := additional["details"].([]any)
	if !ok {
		t.Fatalf("details type = %T", additional["details"])
	}
	for _, want := range wants {
		found := false
		for _, d := range details {
			if strings.Contains(d.(string), want) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q in details, got %v", want, details)
		}
	}
}

func TestCreate_MissingRequired(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create"},
		"id argument is required", "--display-name is required", "--template is required")
}

func TestCreate_IDWithSlash(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "bad/id", "--display-name", "X", "--template", "openai"},
		"id must contain only letters, digits, '-' or '_'")
}

func TestCreate_BadID(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "DX Test Bad!", "--display-name", "X", "--template", "openai"},
		"id must contain only letters, digits, '-' or '_'")
}

func TestCreate_ContextMissingLeadingSlash(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--context", "v1/api"},
		"--context must start with '/'")
}

func TestCreate_ContextTrailingSlash(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--context", "/v1/api/"},
		"--context must not end with '/'")
}

// TestValidateCreate_RootContextOK guards that the default root context "/" is
// not caught by the trailing-slash rule.
func TestValidateCreate_RootContextOK(t *testing.T) {
	err := validateCreate(&CreateOptions{
		ID: "openai", DisplayName: "OpenAI", Template: "openai",
		Context: "/", AuthType: "api-key", Version: defaultVersion,
	})
	if err != nil {
		t.Fatalf("expected valid options, got %v", err)
	}
}

func TestCreate_BadAuthType(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--auth-type", "weird"},
		"--auth-type must be one of")
}

func TestCreate_MutuallyExclusiveKeys(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--api-key", "k", "--api-key-stdin"},
		"--api-key and --api-key-stdin are mutually exclusive")
}

func TestCreate_KeyWithAuthNone(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--auth-type", "none", "--api-key", "k"},
		"an API key cannot be used with --auth-type none")
}

func TestCreate_BlankAPIKey(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--api-key", "   "},
		"--api-key must not be blank")
}

func TestCreate_BlankGateway(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--gateways", "  "},
		"--gateways must not contain a blank value")
}

// A value that can be neither a UUID nor a gateway name is rejected here rather
// than after resolveGatewayNames has paged through every gateway in the org. The
// command tree is built with a nil client factory, so reaching the server at all
// would fail differently than a flag violation.
func TestCreate_MalformedGatewayFailsBeforeTheLookup(t *testing.T) {
	base := []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai"}
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"slash", "acme/edge", "must not contain '/'"},
		{"whitespace", "ai gateway", "must not contain whitespace"},
		{"too long", strings.Repeat("g", gatewayNameMaxLen+1), "longer than a gateway name can be"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runTreeExpectViolation(t, append(append([]string{}, base...), "--gateways", tc.value), tc.want)
		})
	}
}

// The shape gate must not take over resolution: a value that looks like a gateway
// name is still the server's to accept or reject, because only the server knows
// which names exist.
func TestValidateCreate_PlausibleGatewayNameIsNotRejectedLocally(t *testing.T) {
	err := validateCreate(&CreateOptions{
		ID: "openai", DisplayName: "OpenAI", Template: "openai",
		Context: "/", AuthType: "api-key", Version: defaultVersion,
		Gateways: []string{"not-a-uuid-or-a-name", "22222222-2222-2222-2222-222222222222"},
	})
	if err != nil {
		t.Fatalf("expected valid options, got %v", err)
	}
}

func TestCreate_BadVersion(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--version", "v1"},
		"--version must match")
}

func TestCreate_BadAccessMode(t *testing.T) {
	runTreeExpectViolation(t, []string{"llm-provider", "create", "p", "--display-name", "X", "--template", "openai", "--access-mode", "everyone"},
		"--access-mode must be one of")
}
