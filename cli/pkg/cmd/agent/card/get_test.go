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

package card

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func a2aAgent(provisioning amsvc.ProvisioningType) amsvc.AgentDocument {
	sub := "a2a-agent"
	return agentDoc(amsvc.AgentType{Type: "agent-api", SubType: &sub}, provisioning)
}

func agentDoc(agentType amsvc.AgentType, provisioning amsvc.ProvisioningType) amsvc.AgentDocument {
	return amsvc.AgentDocument{
		Metadata: amsvc.ProjectScopedDocumentMetadata{Name: "order-bot", Project: "triage"},
		Spec: amsvc.AgentSpec{
			AgentType:    agentType,
			Provisioning: amsvc.AgentProvisioning{Type: amsvc.AgentProvisioningKind(provisioning)},
		},
	}
}

// a2aExternal carries no subtype: an external agent is A2A wherever it registers a card URL.
func a2aExternal() amsvc.AgentDocument {
	return agentDoc(amsvc.AgentType{Type: "external-agent-api"}, amsvc.ProvisioningTypeExternal)
}
func a2aInternal() amsvc.AgentDocument { return a2aAgent(amsvc.ProvisioningTypeInternal) }

func getOpts(client func(context.Context) (*aigateway.ClientWithResponses, error), jsonMode bool) (*GetOptions, func() string) {
	ios, out, _ := newTestIO(jsonMode)
	return &GetOptions{
			IO: ios, Client: client, AgentManager: coreClientOf(client), Scope: baseScope(),
			Org: "acme", Proj: "triage", AgentName: "order-bot",
		},
		out.String
}

var (
	agentKey = "GET /agents/order-bot"
	proxyKey = "GET " + agentGWPath("a2a-proxy")
	cardKey  = "GET " + agentGWPath("agent-card")

	notFound = route{status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "nope"}}
)

func Test_runGet_direct(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: notFound,
		cardKey: okJSON(aigateway.AgentCardResponse{
			Status: "fetched", SourceUrl: "https://agent.example/card",
			Card: &map[string]interface{}{"name": "Echo"},
		}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	for _, want := range []string{"direct", "Card source: https://agent.example/card", "Echo"} {
		if !strings.Contains(got(), want) {
			t.Errorf("out missing %q:\n%s", want, got())
		}
	}
}

func Test_runGet_directJSON(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: notFound,
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "fetched"}),
	}), true)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	data := decodeEnvelope(t, got())["data"].(map[string]any)
	if v, ok := data["proxy"]; !ok || v != nil {
		t.Errorf("proxy = %v, want null", v)
	}
}

func Test_runGet_proxied(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: okJSON(aigateway.A2AProxyResponse{Status: aigateway.A2AProxyResponseStatusDEPLOYED, GatewayUrl: "https://gw.example/order-bot"}),
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "fetched"}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	for _, want := range []string{"proxied (DEPLOYED)", "https://gw.example/order-bot"} {
		if !strings.Contains(got(), want) {
			t.Errorf("out missing %q:\n%s", want, got())
		}
	}
}

func Test_runGet_proxiedWithoutGateway(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: okJSON(aigateway.A2AProxyResponse{Status: aigateway.A2AProxyResponseStatusDEPLOYED}),
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "fetched"}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(got(), "no ingress gateway mapped") {
		t.Errorf("out = %s", got())
	}
}

func Test_runGet_proxyFailedShowsStatusReason(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: okJSON(aigateway.A2AProxyResponse{Status: aigateway.A2AProxyResponseStatusFAILED, StatusReason: "DEPLOYMENT_TIMEOUT"}),
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "pending"}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(got(), "Routing reason: DEPLOYMENT_TIMEOUT") {
		t.Errorf("out = %s", got())
	}
}

func Test_runGet_pendingWithoutCard(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: okJSON(aigateway.A2AProxyResponse{Status: aigateway.A2AProxyResponseStatusDEPLOYED}),
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "pending", SourceUrl: "https://agent.example/card"}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(got(), "(not fetched yet)") {
		t.Errorf("out = %s", got())
	}
}

func Test_runGet_pendingShowsLastError(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: notFound,
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "pending", LastError: "connection refused"}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(got(), "Card error: connection refused") {
		t.Errorf("out = %s", got())
	}
}

func Test_runGet_internalSkipsProxy(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aInternal()),
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "fetched", Card: &map[string]interface{}{"name": "Echo"}}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if strings.Contains(got(), "Routing:") {
		t.Errorf("internal agent should not show Routing:\n%s", got())
	}
}

func Test_runGet_noCardExternal(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: notFound,
		cardKey:  notFound,
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(got(), "amctl agent card set") {
		t.Errorf("out = %s", got())
	}
}

func Test_runGet_noCardInternal(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aInternal()),
		cardKey:  notFound,
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(got(), "no card fetched yet") {
		t.Errorf("out = %s", got())
	}
}

func Test_runGet_noCardJSON(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: notFound,
		cardKey:  notFound,
	}), true)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	data := decodeEnvelope(t, got())["data"].(map[string]any)
	if v, ok := data["card"]; !ok || v != nil {
		t.Errorf("card = %v, want null", v)
	}
}

func Test_runGet_notA2A(t *testing.T) {
	sub := "chat-api"
	agent := agentDoc(amsvc.AgentType{Type: "agent-api", SubType: &sub}, amsvc.ProvisioningTypeInternal)
	o, _ := getOpts(newClient(t, map[string]route{agentKey: okJSON(agent)}), true)
	err := runGet(context.Background(), o)
	if err == nil {
		t.Fatal("expected error")
	}
	var ce clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != clierr.Validation {
		t.Errorf("err = %v, want Validation", err)
	}
}

func Test_runGet_untypedCardFields(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aInternal()),
		cardKey: okJSON(aigateway.AgentCardResponse{
			Status: "fetched",
			Card:   &map[string]interface{}{"name": 42, "version": "v1.2"},
		}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if !strings.Contains(got(), "v1.2") || strings.Contains(got(), "vv1.2") || strings.Contains(got(), "42") {
		t.Errorf("out = %s", got())
	}
}

func Test_runGet_stripsControlCharsFromCardText(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aInternal()),
		cardKey: okJSON(aigateway.AgentCardResponse{
			Status: "fetched",
			Card:   &map[string]interface{}{"name": "Echo", "description": "\x1b[31mred\u009b"},
		}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	if strings.ContainsAny(got(), "\x1b\u009b") || !strings.Contains(got(), "[31mred") {
		t.Errorf("out = %q", got())
	}
}

func Test_runGet_proxyForbiddenShowsCard(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: {status: http.StatusForbidden, body: map[string]any{"code": "FORBIDDEN", "message": "missing scope"}},
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "fetched", Card: &map[string]interface{}{"name": "Echo"}}),
	}), false)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	for _, want := range []string{"Routing: unknown", "Echo"} {
		if !strings.Contains(got(), want) {
			t.Errorf("out missing %q:\n%s", want, got())
		}
	}
}

func Test_runGet_proxyForbiddenJSON(t *testing.T) {
	o, got := getOpts(newClient(t, map[string]route{
		agentKey: okJSON(a2aExternal()),
		proxyKey: {status: http.StatusForbidden, body: map[string]any{"code": "FORBIDDEN", "message": "missing scope"}},
		cardKey:  okJSON(aigateway.AgentCardResponse{Status: "fetched"}),
	}), true)
	if err := runGet(context.Background(), o); err != nil {
		t.Fatalf("runGet: %v", err)
	}
	data := decodeEnvelope(t, got())["data"].(map[string]any)
	if data["routingUnknown"] != true || data["proxy"] != nil {
		t.Errorf("data = %v, want routingUnknown true and proxy null", data)
	}
}

// coreClientOf is a core API client on the same test server as client: the
// agent is read from the core, its card and proxy from the gateway.
func coreClientOf(client func(context.Context) (*aigateway.ClientWithResponses, error)) func(context.Context) (*amsvc.ClientWithResponses, error) {
	return func(ctx context.Context) (*amsvc.ClientWithResponses, error) {
		gw, err := client(ctx)
		if err != nil {
			return nil, err
		}
		return amsvc.NewClientWithResponses(gw.ClientInterface.(*aigateway.Client).Server)
	}
}
