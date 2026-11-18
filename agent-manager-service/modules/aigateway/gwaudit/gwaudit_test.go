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

package gwaudit

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

// TestIdentityProviderTrustInputsAreRecorded pins the fields that decide which
// tokens a gateway will accept. The issuer alone does not answer that: jwksUri
// says where the signing keys are fetched from, and skipTlsVerify says whether
// that fetch validates the certificate.
func TestIdentityProviderTrustInputsAreRecorded(t *testing.T) {
	for _, action := range []audit.Action{GatewaySetIdentityProvider, GatewayRemoveIdentityProvider} {
		schema := audit.DetailSchema(action)
		for _, key := range []string{"identityProviderName", "issuer", "jwksUri", "skipTlsVerify"} {
			if _, ok := schema[key]; !ok {
				t.Errorf("%s: trust input %q is not a detail key", action, key)
			}
		}
		if schema["jwksUri"] != audit.KindURL {
			t.Errorf("%s: jwksUri must be a URL, so its userinfo and query are stripped", action)
		}
	}
}

// TestCredentialActionsAreCritical pins the severities that drive alerting.
func TestCredentialActionsAreCritical(t *testing.T) {
	for _, action := range []audit.Action{
		APIKeyCreate, APIKeyRotate, APIKeyRevoke, GatewayTokenRotate, GatewayTokenRevoke,
		GatewaySetIdentityProvider, GatewayRemoveIdentityProvider, AgentSetA2AProxy, AgentRemoveA2AProxy,
	} {
		if action.Severity() != audit.SeverityCritical || action.Class() != audit.ClassCredential {
			t.Errorf("%s is %s/%d, want a critical credential action", action, action.Class(), action.Severity())
		}
	}
}

// TestActionsAreTheModules: every action is ai-gateway:<resource>.<verb>.
func TestActionsAreTheModules(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Actions {
		if seen[a.Name] {
			t.Errorf("%s is declared twice", a.Name)
		}
		seen[a.Name] = true
		if got := audit.Action("ai-gateway:" + a.Name).Verb(); got == "" || got == a.Name {
			t.Errorf("%s has no verb of its own (%q)", a.Name, got)
		}
	}
}
