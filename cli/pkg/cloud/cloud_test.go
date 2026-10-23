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

package cloud

import "testing"

func TestIsAPIURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "production", raw: "https://production-wso2cloud.gateway.cloud.wso2.com/agent-manager-service-agent-manager-api", want: true},
		{name: "uppercase host", raw: "https://Development-WSO2Cloud.Gateway.Dev.Cloud.WSO2.com/agent-manager-service-agent-manager-api", want: true},
		{name: "no scheme", raw: "production-wso2cloud.gateway.cloud.wso2.com/agent-manager-service-agent-manager-api", want: true},
		{name: "trailing slash", raw: "https://development-wso2cloud.gateway.dev.cloud.wso2.com/agent-manager-service-agent-manager-api/", want: true},
		{name: "subpath", raw: "https://development-wso2cloud.gateway.dev.cloud.wso2.com/agent-manager-service-agent-manager-api/api/v1/orgs", want: true},
		{name: "non-cloud host", raw: "http://localhost:9000/agent-manager-service-agent-manager-api", want: false},
		{name: "lookalike suffix", raw: "https://gateway.cloud.wso2.com.example.com/agent-manager-service-agent-manager-api", want: false},
		{name: "suffix without a dot", raw: "https://evilcloud.wso2.com/agent-manager-service-agent-manager-api", want: false},
		{name: "right host, wrong path", raw: "https://production-wso2cloud.gateway.cloud.wso2.com/api/v1", want: false},
		{name: "path only starts with the API path", raw: "https://production-wso2cloud.gateway.cloud.wso2.com/agent-manager-service-agent-manager-apix", want: false},
		{name: "empty", raw: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAPIURL(tt.raw); got != tt.want {
				t.Errorf("IsAPIURL(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
