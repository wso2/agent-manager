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

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
)

func Test_BuildEnvVars(t *testing.T) {
	tests := []struct {
		name      string
		urlEnv    string
		apiKeyEnv string
		want      *[]aigateway.EnvironmentVariableConfig
	}{
		{
			name:      "both",
			urlEnv:    "LLM_URL",
			apiKeyEnv: "LLM_KEY",
			want: &[]aigateway.EnvironmentVariableConfig{
				{Key: "url", Name: "LLM_URL"},
				{Key: "apikey", Name: "LLM_KEY"},
			},
		},
		{
			name:   "url only",
			urlEnv: "LLM_URL",
			want:   &[]aigateway.EnvironmentVariableConfig{{Key: "url", Name: "LLM_URL"}},
		},
		{
			name:      "apikey only",
			apiKeyEnv: "LLM_KEY",
			want:      &[]aigateway.EnvironmentVariableConfig{{Key: "apikey", Name: "LLM_KEY"}},
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
		{name: "non-empty", in: "primary model", want: func() *string { v := "primary model"; return &v }()},
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
