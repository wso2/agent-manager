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

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestAPIURL(t *testing.T) {
	cases := map[string]struct{ env, want string }{
		"production by default": {"", DefaultGatewayURL + APIPath},
		"whitespace is ignored": {"   ", DefaultGatewayURL + APIPath},
		"override":              {"https://dev.gateway.example", "https://dev.gateway.example" + APIPath},
		"trailing slash":        {"https://dev.gateway.example///", "https://dev.gateway.example" + APIPath},
		"padded override":       {"  https://dev.gateway.example/ ", "https://dev.gateway.example" + APIPath},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := APIURL(env(map[string]string{GatewayURLEnv: tc.env})); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoginHint_TellsUsersExactlyWhatToRun(t *testing.T) {
	raw := "https://dev-wso2cloud.gateway.dev.cloud.wso2.com" + APIPath + "/api/v1"
	got := LoginHint(raw, "https://releases.example")
	for _, want := range []string{
		raw,
		"export " + GatewayURLEnv + "=https://dev-wso2cloud.gateway.dev.cloud.wso2.com",
		"amctl login --cloud",
		"https://releases.example",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hint missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "export "+GatewayURLEnv+"=https://dev-wso2cloud.gateway.dev.cloud.wso2.com"+APIPath) {
		t.Error("the gateway variable must hold the origin, not the API path")
	}
}

func TestLoginHint_SchemelessURL(t *testing.T) {
	got := LoginHint("production-wso2cloud.gateway.cloud.wso2.com"+APIPath, "r")
	if !strings.Contains(got, "="+"https://production-wso2cloud.gateway.cloud.wso2.com ") {
		t.Errorf("hint = %s", got)
	}
}
