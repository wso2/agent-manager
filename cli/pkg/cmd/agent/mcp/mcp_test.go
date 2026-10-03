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

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/cmd/agent/modelconfig"
)

func Test_toEnvModelConfigRequest_mergesEveryEnv(t *testing.T) {
	policies := []amsvc.LLMPolicy{{Name: "rate-limit", Version: "v1"}}
	resp := &amsvc.AgentModelConfigResponse{
		EnvMappings: map[string]amsvc.EnvProviderConfigMappings{
			"dev": {
				EnvironmentName: "dev",
				Configuration: &amsvc.ProviderConfig{
					ProviderName: "github",
					ProxyName:    modelconfig.StringPtr("github"),
					Url:          "https://server-managed.example.com",
					Policies:     &policies,
				},
			},
			"prod": {
				EnvironmentName: "prod",
				Configuration:   &amsvc.ProviderConfig{ProviderName: "gitlab", ProxyName: modelconfig.StringPtr("gitlab")},
			},
		},
	}

	got := modelconfig.MergeExistingEnvMappings(resp, toEnvModelConfigRequest)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got["dev"].ProxyName == nil || *got["dev"].ProxyName != "github" {
		t.Errorf("dev proxy = %v, want github", got["dev"].ProxyName)
	}
	if got["prod"].ProxyName == nil || *got["prod"].ProxyName != "gitlab" {
		t.Errorf("prod proxy = %v, want gitlab", got["prod"].ProxyName)
	}
	// Policies must survive the round-trip translation.
	dp := got["dev"].Configuration.Policies
	if dp == nil || len(*dp) != 1 || (*dp)[0].Name != "rate-limit" {
		t.Errorf("dev policies not carried over: %#v", got["dev"].Configuration.Policies)
	}
}

func Test_toEnvModelConfigRequest_nilConfiguration(t *testing.T) {
	got := toEnvModelConfigRequest(amsvc.EnvProviderConfigMappings{EnvironmentName: "dev"})
	if got.ProxyName != nil {
		t.Errorf("ProxyName = %q, want nil", *got.ProxyName)
	}
	if got.Configuration.Policies != nil {
		t.Errorf("Policies = %#v, want nil", got.Configuration.Policies)
	}
}

// proxyNameFromConfig must prefer the proxy-specific fields, then fall back to
// providerName (which the server mirrors the proxy handle into).
func Test_proxyNameFromConfig(t *testing.T) {
	t.Run("prefers proxyName", func(t *testing.T) {
		c := &amsvc.ProviderConfig{ProviderName: "github", ProxyName: modelconfig.StringPtr("github-proxy")}
		if got := proxyNameFromConfig(c); got != "github-proxy" {
			t.Errorf("got %q, want github-proxy", got)
		}
	})
	t.Run("falls back to providerName", func(t *testing.T) {
		c := &amsvc.ProviderConfig{ProviderName: "github"}
		if got := proxyNameFromConfig(c); got != "github" {
			t.Errorf("got %q, want github", got)
		}
	})
	t.Run("nil configuration", func(t *testing.T) {
		if got := proxyNameFromConfig(nil); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func listResponse(items ...amsvc.AgentModelConfigListItem) amsvc.AgentModelConfigListResponse {
	return amsvc.AgentModelConfigListResponse{Configs: items}
}

func mcpItem(name, uuid string) amsvc.AgentModelConfigListItem {
	return amsvc.AgentModelConfigListItem{Name: name, Type: "mcp", Uuid: mustUUID(uuid)}
}

func Test_findMCPConfigByName(t *testing.T) {
	const uuidA = "11111111-1111-1111-1111-111111111111"
	t.Run("found", func(t *testing.T) {
		client, cleanup := newClient(t, map[string]route{
			listPath: okJSON(listResponse(
				amsvc.AgentModelConfigListItem{Name: "shared", Type: "llm", Uuid: mustUUID("22222222-2222-2222-2222-222222222222")},
				mcpItem("primary", uuidA),
			)),
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

	t.Run("ignores same name with non-mcp type", func(t *testing.T) {
		client, cleanup := newClient(t, map[string]route{
			listPath: okJSON(listResponse(
				amsvc.AgentModelConfigListItem{Name: "primary", Type: "llm", Uuid: mustUUID(uuidA)},
			)),
		})
		defer cleanup()
		c, _ := client(context.Background())
		_, found, err := findMCPConfigByName(context.Background(), c, "acme", "triage", "order-bot", "primary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false (llm type must be ignored)")
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
