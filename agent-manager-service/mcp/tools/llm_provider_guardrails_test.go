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
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// policiesMock serves a provider holding the given guardrails, as the console's
// Guardrails tab leaves them: per-route entries beside, or instead of, /*.
type policiesMock struct {
	*MockToolsetHandler
	policies []models.LLMPolicy
}

func (m policiesMock) GetLLMProvider(ctx context.Context, ouID string, providerID string) (*models.LLMProvider, []*models.Deployment, error) {
	p, d, err := m.MockToolsetHandler.GetLLMProvider(ctx, ouID, providerID)
	p.Configuration.Policies = m.policies
	return p, d, err
}

func (m policiesMock) UpdateLLMProviderGuardrails(
	_ context.Context, ouID string, providerID string, change func([]models.LLMPolicy) ([]models.LLMPolicy, error),
) (*models.LLMProvider, []services.DeploymentResult, error) {
	return m.saveGuardrails(ouID, providerID, m.policies, change)
}

func allRoutes(params map[string]any) models.LLMPolicyPath {
	return models.LLMPolicyPath{Path: guardrailPathAll, Methods: []string{"*"}, Params: params}
}

func chatRoute(params map[string]any) models.LLMPolicyPath {
	return models.LLMPolicyPath{Path: "/chat/completions", Methods: []string{"POST"}, Params: params}
}

// callGuardrailTool runs a tool against a provider holding policies and returns
// whether it failed, its text, and the guardrails it saved (nil when it saved none).
func callGuardrailTool(t *testing.T, policies []models.LLMPolicy, tool string, args map[string]any) (bool, string, []models.LLMPolicy) {
	t.Helper()
	mock := policiesMock{MockToolsetHandler: NewMockToolsetHandler(), policies: policies}
	session := setupTestServerWithToolsets(t, &Toolsets{LLMProviderToolset: mock})
	args["provider_id"] = testProviderID
	res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*gomcp.TextContent).Text
	calls := mock.calls["UpdateLLMProviderGuardrails"]
	if len(calls) == 0 {
		return res.IsError, text, nil
	}
	return res.IsError, text, calls[0].([]interface{})[2].([]models.LLMPolicy)
}

// A same-name add must not flatten a guardrail the console scoped to a route
// onto every route, nor drop its per-route params.
func TestAddGuardrailKeepsConsoleRouteScopes(t *testing.T) {
	t.Run("updates the all-routes entry and keeps the route", func(t *testing.T) {
		policies := []models.LLMPolicy{{Name: testGuardrailName, Version: "v1", Paths: []models.LLMPolicyPath{
			chatRoute(map[string]any{"mask": "route"}), allRoutes(map[string]any{"mask": "old"}),
		}}}
		failed, text, saved := callGuardrailTool(t, policies, "add_llm_provider_guardrails", map[string]any{
			"guardrails": []any{map[string]any{"name": testGuardrailName, "version": "v1", "params": map[string]any{"mask": "new"}}},
		})
		if failed {
			t.Fatalf("tool failed: %s", text)
		}
		if len(saved) != 1 || len(saved[0].Paths) != 2 {
			t.Fatalf("saved %+v, want one policy with both paths", saved)
		}
		if route := saved[0].Paths[0]; route.Path != "/chat/completions" || route.Params["mask"] != "route" {
			t.Errorf("route entry changed: %+v", route)
		}
		if all := saved[0].Paths[1]; all.Path != guardrailPathAll || all.Params["mask"] != "new" {
			t.Errorf("all-routes entry not updated: %+v", all)
		}
	})

	t.Run("adds an all-routes entry to a route-only guardrail", func(t *testing.T) {
		policies := []models.LLMPolicy{{Name: testGuardrailName, Version: "v1", Paths: []models.LLMPolicyPath{chatRoute(map[string]any{"mask": "route"})}}}
		_, text, saved := callGuardrailTool(t, policies, "add_llm_provider_guardrails", map[string]any{
			"guardrails": []any{map[string]any{"name": testGuardrailName, "version": "v1"}},
		})
		if len(saved) != 1 || len(saved[0].Paths) != 2 || saved[0].Paths[0].Path != "/chat/completions" || saved[0].Paths[1].Path != guardrailPathAll {
			t.Fatalf("saved %+v (%s), want the route kept and /* added", saved, text)
		}
	})

	t.Run("refuses a different version of a guardrail already there", func(t *testing.T) {
		policies := []models.LLMPolicy{{Name: testGuardrailName, Version: "v1", Paths: []models.LLMPolicyPath{allRoutes(nil)}}}
		failed, text, saved := callGuardrailTool(t, policies, "add_llm_provider_guardrails", map[string]any{
			"guardrails": []any{map[string]any{"name": testGuardrailName, "version": "v2"}},
		})
		if !failed || saved != nil {
			t.Fatalf("want an error and no save, got failed=%v saved=%+v", failed, saved)
		}
		if !strings.Contains(text, "version v1") {
			t.Errorf("error should name the version already there: %s", text)
		}
	})
}

// Remove takes away the all-routes guardrail only; routes the console scoped
// the guardrail to stay, and a route-only guardrail is left to the console.
func TestRemoveGuardrailKeepsConsoleRouteScopes(t *testing.T) {
	t.Run("keeps route entries", func(t *testing.T) {
		policies := []models.LLMPolicy{{Name: testGuardrailName, Version: "v1", Paths: []models.LLMPolicyPath{chatRoute(nil), allRoutes(nil)}}}
		failed, text, saved := callGuardrailTool(t, policies, "remove_llm_provider_guardrail", map[string]any{"name": testGuardrailName})
		if failed {
			t.Fatalf("tool failed: %s", text)
		}
		if len(saved) != 1 || len(saved[0].Paths) != 1 || saved[0].Paths[0].Path != "/chat/completions" {
			t.Fatalf("saved %+v, want only the route entry left", saved)
		}
		if !strings.Contains(text, `"routes":[{"path":"/chat/completions","methods":["POST"]}]`) || !strings.Contains(text, `"all_routes":false`) {
			t.Errorf("output should show the route still guarded: %s", text)
		}
	})

	t.Run("refuses a route-only guardrail", func(t *testing.T) {
		policies := []models.LLMPolicy{{Name: testGuardrailName, Version: "v1", Paths: []models.LLMPolicyPath{chatRoute(nil)}}}
		failed, text, saved := callGuardrailTool(t, policies, "remove_llm_provider_guardrail", map[string]any{"name": testGuardrailName})
		if !failed || saved != nil {
			t.Fatalf("want an error and no save, got failed=%v saved=%+v", failed, saved)
		}
		if !strings.Contains(text, "POST /chat/completions") {
			t.Errorf("error should name the console routes: %s", text)
		}
	})

	twoVersions := []models.LLMPolicy{
		{Name: testGuardrailName, Version: "v1", Paths: []models.LLMPolicyPath{allRoutes(nil)}},
		{Name: testGuardrailName, Version: "v2", Paths: []models.LLMPolicyPath{allRoutes(nil)}},
	}
	t.Run("asks for a version when several are there", func(t *testing.T) {
		failed, text, saved := callGuardrailTool(t, twoVersions, "remove_llm_provider_guardrail", map[string]any{"name": testGuardrailName})
		if !failed || saved != nil || !strings.Contains(text, "v1, v2") {
			t.Fatalf("want an error naming both versions and no save, got failed=%v saved=%+v text=%s", failed, saved, text)
		}
	})

	t.Run("removes only the version asked for", func(t *testing.T) {
		_, text, saved := callGuardrailTool(t, twoVersions, "remove_llm_provider_guardrail", map[string]any{"name": testGuardrailName, "version": "v2"})
		if len(saved) != 1 || saved[0].Version != "v1" {
			t.Fatalf("saved %+v (%s), want only v1 left", saved, text)
		}
	})
}

// A guardrail the console scoped to single routes must not read as
// provider-wide, and every route's params must be visible, not just the first.
func TestGetLLMProviderShowsGuardrailRouteScope(t *testing.T) {
	policies := []models.LLMPolicy{{Name: testGuardrailName, Version: "v1", Paths: []models.LLMPolicyPath{
		chatRoute(map[string]any{"mask": "chat"}),
		{Path: "/embeddings", Methods: []string{"POST"}, Params: map[string]any{"mask": "embed"}},
	}}}
	failed, text, _ := callGuardrailTool(t, policies, "get_llm_provider", map[string]any{})
	if failed {
		t.Fatalf("tool failed: %s", text)
	}
	want := `"guardrails":[{"name":"` + testGuardrailName + `","version":"v1","all_routes":false,"routes":[` +
		`{"path":"/chat/completions","methods":["POST"],"params":{"mask":"chat"}},` +
		`{"path":"/embeddings","methods":["POST"],"params":{"mask":"embed"}}]}]`
	if !strings.Contains(text, want) {
		t.Errorf("output does not show the route scope\n got: %s\nwant: %s", text, want)
	}

	_, text, _ = callGuardrailTool(t, nil, "get_llm_provider", map[string]any{})
	if !strings.Contains(text, `"guardrails":[]`) {
		t.Errorf("a provider with no guardrails should list [], got: %s", text)
	}
}

// Add and remove read the provider once, inside the update, so the guardrails
// they compute are the ones on the record that is saved.
func TestGuardrailUpdatesReadTheProviderOnce(t *testing.T) {
	session, mock := setupTestServer(t)
	for tool, args := range map[string]map[string]any{
		"add_llm_provider_guardrails":   {"guardrails": []any{map[string]any{"name": "word-count", "version": "v1"}}},
		"remove_llm_provider_guardrail": {"name": testGuardrailName},
	} {
		args["provider_id"] = testProviderID
		res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("%s: err=%v content=%+v", tool, err, res.Content)
		}
	}
	if _, read := mock.calls["GetLLMProvider"]; read {
		t.Error("the tool read the provider itself as well as inside the update")
	}
	if got := len(mock.calls["UpdateLLMProviderGuardrails"]); got != 2 {
		t.Errorf("UpdateLLMProviderGuardrails saved %d times, want 2", got)
	}
}

// failingUpdateMock fails the update itself, as the provider service does for
// a provider that does not exist.
type failingUpdateMock struct{ *MockToolsetHandler }

func (failingUpdateMock) UpdateLLMProviderGuardrails(
	context.Context, string, string, func([]models.LLMPolicy) ([]models.LLMPolicy, error),
) (*models.LLMProvider, []services.DeploymentResult, error) {
	return nil, nil, utils.ErrLLMProviderNotFound
}

// change's errors already say what to do and reach the model unchanged; the
// service's errors get wrapToolError's hint.
func TestGuardrailUpdateErrors(t *testing.T) {
	failed, text, saved := callGuardrailTool(t, nil, "remove_llm_provider_guardrail", map[string]any{"name": "not-there"})
	if !failed || saved != nil {
		t.Fatalf("want an error and no save, got failed=%v saved=%+v", failed, saved)
	}
	if want := `remove_llm_provider_guardrail: provider "` + testProviderID + `" has no guardrail named "not-there"`; !strings.HasPrefix(text, want) {
		t.Errorf("change error was rewritten:\n got: %s\nwant prefix: %s", text, want)
	}

	session := setupTestServerWithToolsets(t, &Toolsets{LLMProviderToolset: failingUpdateMock{NewMockToolsetHandler()}})
	res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name: "remove_llm_provider_guardrail", Arguments: map[string]any{"provider_id": testProviderID, "name": testGuardrailName},
	})
	if err != nil || !res.IsError {
		t.Fatalf("want a tool error, got err=%v res=%+v", err, res)
	}
	if text := res.Content[0].(*gomcp.TextContent).Text; !strings.Contains(text, "list_llm_providers") {
		t.Errorf("service error should carry wrapToolError's hint: %s", text)
	}
}

// catalogMock lists one provider in the catalog and one taken out of it.
type catalogMock struct{ *MockToolsetHandler }

func (catalogMock) ListLLMProviders(context.Context, string, int, int) ([]*models.LLMProvider, int, error) {
	return []*models.LLMProvider{
		{InCatalog: true, Artifact: &models.Artifact{Handle: "in-catalog"}},
		{InCatalog: false, Artifact: &models.Artifact{Handle: "out-of-catalog"}},
	}, 2, nil
}

// Attach takes a provider's id (handle) only and refuses one out of the
// catalog, so the provider list offers no UUID to copy there by mistake and
// says which providers can be attached.
func TestLLMProviderOutputOffersOnlyAttachableIdentifiers(t *testing.T) {
	session := setupTestServerWithToolsets(t, &Toolsets{LLMProviderToolset: catalogMock{NewMockToolsetHandler()}})
	for tool, args := range map[string]map[string]any{
		"list_llm_providers": {},
		"get_llm_provider":   {"provider_id": testProviderID},
	} {
		res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("%s: err=%v content=%+v", tool, err, res.Content)
		}
		text := res.Content[0].(*gomcp.TextContent).Text
		if strings.Contains(text, `"uuid"`) {
			t.Errorf("%s returns a uuid beside the id: %s", tool, text)
		}
		if !strings.Contains(text, `"in_catalog":true`) {
			t.Errorf("%s does not say the provider is in the catalog: %s", tool, text)
		}
		if tool == "list_llm_providers" && !strings.Contains(text, `"id":"out-of-catalog","name":"","template":"","status":"","in_catalog":false`) {
			t.Errorf("list does not flag the provider out of the catalog: %s", text)
		}
	}
}
