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

package llm

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
					ProviderName: "openai",
					Url:          "https://server-managed.example.com",
					Policies:     &policies,
				},
			},
			"prod": {
				EnvironmentName: "prod",
				Configuration:   &amsvc.ProviderConfig{ProviderName: "anthropic"},
			},
		},
	}

	got := modelconfig.MergeExistingEnvMappings(resp, toEnvModelConfigRequest)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got["dev"].ProviderName == nil || *got["dev"].ProviderName != "openai" {
		t.Errorf("dev provider = %v, want openai", got["dev"].ProviderName)
	}
	if got["prod"].ProviderName == nil || *got["prod"].ProviderName != "anthropic" {
		t.Errorf("prod provider = %v, want anthropic", got["prod"].ProviderName)
	}
	// Policies must survive the round-trip translation.
	dp := got["dev"].Configuration.Policies
	if dp == nil || len(*dp) != 1 || (*dp)[0].Name != "rate-limit" {
		t.Errorf("dev policies not carried over: %#v", got["dev"].Configuration.Policies)
	}
}

func Test_toEnvModelConfigRequest_nilConfiguration(t *testing.T) {
	got := toEnvModelConfigRequest(amsvc.EnvProviderConfigMappings{EnvironmentName: "dev"})
	if got.ProviderName != nil {
		t.Errorf("ProviderName = %q, want nil", *got.ProviderName)
	}
	if got.Configuration.Policies != nil {
		t.Errorf("Policies = %#v, want nil", got.Configuration.Policies)
	}
}

func listResponse(items ...amsvc.AgentModelConfigListItem) amsvc.AgentModelConfigListResponse {
	return amsvc.AgentModelConfigListResponse{Configs: items}
}

func llmItem(name, uuid string) amsvc.AgentModelConfigListItem {
	return amsvc.AgentModelConfigListItem{Name: name, Type: "llm", Uuid: mustUUID(uuid)}
}

func Test_findLLMConfigByName(t *testing.T) {
	const uuidA = "11111111-1111-1111-1111-111111111111"
	t.Run("found", func(t *testing.T) {
		client, cleanup := newClient(t, map[string]route{
			listPath: okJSON(listResponse(
				amsvc.AgentModelConfigListItem{Name: "shared", Type: "mcp", Uuid: mustUUID("22222222-2222-2222-2222-222222222222")},
				llmItem("primary", uuidA),
			)),
		})
		defer cleanup()
		c, _ := client(context.Background())
		id, found, err := findLLMConfigByName(context.Background(), c, "acme", "triage", "order-bot", "primary")
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

	t.Run("ignores same name with non-llm type", func(t *testing.T) {
		client, cleanup := newClient(t, map[string]route{
			listPath: okJSON(listResponse(
				amsvc.AgentModelConfigListItem{Name: "primary", Type: "mcp", Uuid: mustUUID(uuidA)},
			)),
		})
		defer cleanup()
		c, _ := client(context.Background())
		_, found, err := findLLMConfigByName(context.Background(), c, "acme", "triage", "order-bot", "primary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false (mcp type must be ignored)")
		}
	})

	t.Run("not found returns no error", func(t *testing.T) {
		client, cleanup := newClient(t, map[string]route{
			listPath: okJSON(listResponse(llmItem("other", uuidA))),
		})
		defer cleanup()
		c, _ := client(context.Background())
		_, found, err := findLLMConfigByName(context.Background(), c, "acme", "triage", "order-bot", "primary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false")
		}
	})
}
