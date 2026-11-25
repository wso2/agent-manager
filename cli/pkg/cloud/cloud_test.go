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
		{name: "production US", raw: "https://prod-v2-wso2cloud.gateway.cloud.choreolabs.com/agent-manager-service-agent-manager-api", want: true},
		{name: "production US api alias", raw: "https://api.cloud.choreolabs.com/agent-manager-service-agent-manager-api", want: true},
		{name: "production EU", raw: "https://prod-v2-eu-wso2cloud.gateway.eu.cloud.choreolabs.com/agent-manager-service-agent-manager-api", want: true},
		{name: "stage", raw: "https://stage-v2-wso2cloud.gateway.stg.cloud.choreolabs.com/agent-manager-service-agent-manager-api", want: true},
		{name: "dev", raw: "https://development-wso2cloud.gateway.dev.cloud.wso2.com/agent-manager-service-agent-manager-api", want: true},
		{name: "old production", raw: "https://production-wso2cloud.gateway.cloud.wso2.com/agent-manager-service-agent-manager-api", want: true},
		{name: "uppercase host", raw: "https://Development-WSO2Cloud.Gateway.Dev.Cloud.WSO2.com/agent-manager-service-agent-manager-api", want: true},
		{name: "no scheme", raw: "production-wso2cloud.gateway.cloud.wso2.com/agent-manager-service-agent-manager-api", want: true},
		{name: "trailing slash", raw: "https://development-wso2cloud.gateway.dev.cloud.wso2.com/agent-manager-service-agent-manager-api/", want: true},
		{name: "subpath", raw: "https://development-wso2cloud.gateway.dev.cloud.wso2.com/agent-manager-service-agent-manager-api/api/v1/orgs", want: true},
		{name: "non-cloud host", raw: "http://localhost:9000/agent-manager-service-agent-manager-api", want: false},
		{name: "lookalike suffix", raw: "https://gateway.cloud.wso2.com.example.com/agent-manager-service-agent-manager-api", want: false},
		{name: "suffix without a dot", raw: "https://evilcloud.wso2.com/agent-manager-service-agent-manager-api", want: false},
		{name: "choreolabs lookalike suffix", raw: "https://api.cloud.choreolabs.com.example.com/agent-manager-service-agent-manager-api", want: false},
		{name: "choreolabs suffix without a dot", raw: "https://evilcloud.choreolabs.com/agent-manager-service-agent-manager-api", want: false},
		{name: "choreolabs data-plane domain", raw: "https://x.us.cloud.choreolabs.io/agent-manager-service-agent-manager-api", want: false},
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

func TestAPIURL(t *testing.T) {
	tests := []struct {
		name    string
		gateway string
		want    string
	}{
		{name: "US production by default", want: "https://prod-v2-wso2cloud.gateway.cloud.choreolabs.com/agent-manager-service-agent-manager-api"},
		{name: "blank env is the default", gateway: "  ", want: DefaultGatewayURL + APIPath},
		{name: "EU from env, trailing slash", gateway: "https://prod-v2-eu-wso2cloud.gateway.eu.cloud.choreolabs.com/", want: "https://prod-v2-eu-wso2cloud.gateway.eu.cloud.choreolabs.com" + APIPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := APIURL(func(k string) string {
				if k == GatewayURLEnv {
					return tt.gateway
				}
				return ""
			})
			if got != tt.want {
				t.Errorf("APIURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpdateResource(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: "urn:wso2:amp", want: Resource},
		{in: Resource, want: Resource},
		{in: "", want: ""},
		{in: "urn:example", want: "urn:example"},
	}
	for _, tt := range tests {
		if got := UpdateResource(tt.in); got != tt.want {
			t.Errorf("UpdateResource(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRegions(t *testing.T) {
	if r, ok := RegionByName(" EU "); !ok || r.GatewayURL != EUGatewayURL {
		t.Errorf("RegionByName(EU) = %+v, %v", r, ok)
	}
	if _, ok := RegionByName("apac"); ok {
		t.Errorf("RegionByName(apac) found a region")
	}
	if Regions[0].GatewayURL != DefaultGatewayURL {
		t.Errorf("first region = %q, want the default gateway", Regions[0].GatewayURL)
	}
	if r, ok := RegionOfAPIURL(EUGatewayURL + APIPath + "/"); !ok || r.Name != "eu" {
		t.Errorf("RegionOfAPIURL(EU) = %+v, %v", r, ok)
	}
	if _, ok := RegionOfAPIURL("https://development-wso2cloud.gateway.dev.cloud.wso2.com" + APIPath); ok {
		t.Errorf("RegionOfAPIURL(dev) found a region")
	}
	if got := RegionNames(); got != "us, eu" {
		t.Errorf("RegionNames() = %q", got)
	}
}
