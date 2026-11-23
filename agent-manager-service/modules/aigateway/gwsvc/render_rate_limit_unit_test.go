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

package gwsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// rateLimited is a renderable provider with rateLimiting.
func rateLimited(rl *gwmodels.LLMRateLimitingConfig) *gwmodels.LLMProvider {
	p := renderableProvider()
	p.Configuration.RateLimiting = rl
	return p
}

// everyLimit enables a request, token and cost limit, each reset by window.
func everyLimit(window gwmodels.RateLimitResetWindow) gwmodels.RateLimitingLimitConfig {
	return gwmodels.RateLimitingLimitConfig{
		Request: &gwmodels.RequestRateLimit{Enabled: true, Count: 10, Reset: window},
		Token:   &gwmodels.TokenRateLimit{Enabled: true, Count: 100, Reset: window},
		Cost:    &gwmodels.CostRateLimit{Enabled: true, Amount: 5, Reset: window},
	}
}

// renderedPaths is each rendered policy's paths as "METHODS path", in order.
func renderedPaths(t *testing.T, p *gwmodels.LLMProvider) map[string][]string {
	t.Helper()
	out, err := renderLLMProvider(p)
	require.NoError(t, err)
	var doc struct {
		Spec struct {
			Policies []struct {
				Name  string `yaml:"name"`
				Paths []struct {
					Path    string   `yaml:"path"`
					Methods []string `yaml:"methods"`
				} `yaml:"paths"`
			} `yaml:"policies"`
		} `yaml:"spec"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(out), &doc))
	got := map[string][]string{}
	for _, pol := range doc.Spec.Policies {
		for _, path := range pol.Paths {
			got[pol.Name] = append(got[pol.Name], path.Methods[0]+" "+path.Path)
		}
	}
	return got
}

// Resource-wise limits put each resource's own path ahead of the catch-all
// default, for every kind of limit and for the cost accounting.
func TestRenderResourceWiseRateLimitsOrderSpecificPathsFirst(t *testing.T) {
	hour := gwmodels.RateLimitResetWindow{Duration: 1, Unit: "hour"}
	p := rateLimited(&gwmodels.LLMRateLimitingConfig{ProviderLevel: &gwmodels.RateLimitingScopeConfig{
		ResourceWise: &gwmodels.ResourceWiseRateLimitingConfig{
			Default:   everyLimit(hour),
			Resources: []gwmodels.RateLimitingResourceLimit{{Resource: "POST-/chat/completions", Limit: everyLimit(hour)}},
		},
	}})

	want := []string{"POST /chat/completions", "* /*"}
	assert.Equal(t, map[string][]string{
		tokenBasedRateLimitPolicyName: want,
		advancedRateLimitPolicyName:   want,
		costBasedRateLimitPolicyName:  want,
		"llm-cost":                    want,
	}, renderedPaths(t, p))
}

// A resource key without a method applies to every method.
func TestRenderResourceKeyWithoutMethodMatchesAnyMethod(t *testing.T) {
	hour := gwmodels.RateLimitResetWindow{Duration: 1, Unit: "hour"}
	p := rateLimited(&gwmodels.LLMRateLimitingConfig{ProviderLevel: &gwmodels.RateLimitingScopeConfig{
		ResourceWise: &gwmodels.ResourceWiseRateLimitingConfig{
			Resources: []gwmodels.RateLimitingResourceLimit{{Resource: "/embeddings", Limit: gwmodels.RateLimitingLimitConfig{
				Request: &gwmodels.RequestRateLimit{Enabled: true, Count: 1, Reset: hour},
			}}},
		},
	}})
	assert.Equal(t, map[string][]string{advancedRateLimitPolicyName: {"* /embeddings"}}, renderedPaths(t, p))
}

// Consumer-level limits render alongside the provider's own.
func TestRenderConsumerLevelRateLimits(t *testing.T) {
	day := gwmodels.RateLimitResetWindow{Duration: 1, Unit: "day"}
	global := everyLimit(day)
	p := rateLimited(&gwmodels.LLMRateLimitingConfig{ConsumerLevel: &gwmodels.RateLimitingScopeConfig{Global: &global}})

	got := renderedPaths(t, p)
	for _, name := range []string{tokenBasedRateLimitPolicyName, advancedRateLimitPolicyName, costBasedRateLimitPolicyName} {
		assert.NotEmpty(t, got[name], "consumer limits render %s", name)
	}
	out, err := renderLLMProvider(p)
	require.NoError(t, err)
	assert.Contains(t, out, "duration: 24h", "a day resets after 24 hours")
}

// A limit with a reset window the gateway cannot express is refused, naming
// the window, wherever the limit is configured.
func TestRenderRefusesAnUnsupportedResetWindow(t *testing.T) {
	bad := gwmodels.RateLimitResetWindow{Duration: 1, Unit: "fortnight"}
	zero := gwmodels.RateLimitResetWindow{Duration: 0, Unit: "hour"}
	only := func(kind string, w gwmodels.RateLimitResetWindow) gwmodels.RateLimitingLimitConfig {
		var l gwmodels.RateLimitingLimitConfig
		switch kind {
		case "request":
			l.Request = &gwmodels.RequestRateLimit{Enabled: true, Count: 1, Reset: w}
		case "token":
			l.Token = &gwmodels.TokenRateLimit{Enabled: true, Count: 1, Reset: w}
		case "cost":
			l.Cost = &gwmodels.CostRateLimit{Enabled: true, Amount: 1, Reset: w}
		}
		return l
	}
	for _, kind := range []string{"request", "token", "cost"} {
		for _, w := range []gwmodels.RateLimitResetWindow{bad, zero} {
			limit := only(kind, w)
			cases := map[string]*gwmodels.LLMRateLimitingConfig{
				"provider global": {ProviderLevel: &gwmodels.RateLimitingScopeConfig{Global: &limit}},
				"resource": {ProviderLevel: &gwmodels.RateLimitingScopeConfig{ResourceWise: &gwmodels.ResourceWiseRateLimitingConfig{
					Resources: []gwmodels.RateLimitingResourceLimit{{Resource: "GET-/models", Limit: limit}},
				}}},
				"resource default": {ProviderLevel: &gwmodels.RateLimitingScopeConfig{ResourceWise: &gwmodels.ResourceWiseRateLimitingConfig{
					Default: limit,
				}}},
				"consumer global": {ConsumerLevel: &gwmodels.RateLimitingScopeConfig{Global: &limit}},
			}
			for where, rl := range cases {
				t.Run(where+"/"+kind+"/"+w.Unit, func(t *testing.T) {
					_, err := renderLLMProvider(rateLimited(rl))
					require.Error(t, err)
					assert.Contains(t, err.Error(), kind+" reset window")
				})
			}
		}
	}
}
