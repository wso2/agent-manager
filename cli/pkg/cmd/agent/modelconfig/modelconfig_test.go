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

package modelconfig

import (
	"reflect"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

func Test_BuildEnvVars(t *testing.T) {
	tests := []struct {
		name      string
		urlEnv    string
		apiKeyEnv string
		want      *[]amsvc.EnvironmentVariableConfig
	}{
		{
			name:      "both",
			urlEnv:    "LLM_URL",
			apiKeyEnv: "LLM_KEY",
			want: &[]amsvc.EnvironmentVariableConfig{
				{Key: "url", Name: "LLM_URL"},
				{Key: "apikey", Name: "LLM_KEY"},
			},
		},
		{
			name:   "url only",
			urlEnv: "LLM_URL",
			want:   &[]amsvc.EnvironmentVariableConfig{{Key: "url", Name: "LLM_URL"}},
		},
		{
			name:      "apikey only",
			apiKeyEnv: "LLM_KEY",
			want:      &[]amsvc.EnvironmentVariableConfig{{Key: "apikey", Name: "LLM_KEY"}},
		},
		{
			name: "none",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildEnvVars(tt.urlEnv, tt.apiKeyEnv)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildEnvVars(%q, %q) = %#v, want %#v", tt.urlEnv, tt.apiKeyEnv, got, tt.want)
			}
		})
	}
}

func Test_OptionalString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want *string
	}{
		{name: "empty is nil", in: "", want: nil},
		{name: "non-empty", in: "primary model", want: StringPtr("primary model")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OptionalString(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OptionalString(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func Test_MergeExistingEnvMappings(t *testing.T) {
	resp := &amsvc.AgentModelConfigResponse{
		EnvMappings: map[string]amsvc.EnvProviderConfigMappings{
			"dev":  {EnvironmentName: "dev", Configuration: &amsvc.ProviderConfig{ProviderName: "openai"}},
			"prod": {EnvironmentName: "prod"},
		},
	}
	// Stub transform echoes the environment name so routing per env is observable.
	stub := func(m amsvc.EnvProviderConfigMappings) amsvc.EnvModelConfigRequest {
		return amsvc.EnvModelConfigRequest{ProviderName: StringPtr(m.EnvironmentName)}
	}

	got := MergeExistingEnvMappings(resp, stub)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for _, env := range []string{"dev", "prod"} {
		if p := got[env].ProviderName; p == nil || *p != env {
			t.Errorf("%s provider = %v, want %s", env, p, env)
		}
	}
}

func Test_MergeExistingEnvMappings_empty(t *testing.T) {
	got := MergeExistingEnvMappings(&amsvc.AgentModelConfigResponse{}, func(amsvc.EnvProviderConfigMappings) amsvc.EnvModelConfigRequest {
		t.Fatal("transform called for empty response")
		return amsvc.EnvModelConfigRequest{}
	})
	if got == nil || len(got) != 0 {
		t.Errorf("got %#v, want empty non-nil map", got)
	}
}
