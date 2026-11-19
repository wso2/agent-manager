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

package gwapi

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
)

// TestCredentialRoutesAreFailClosed: a route whose effect lands outside the
// database (a key or token a gateway receives, what a gateway trusts) must not
// run unrecorded.
func TestCredentialRoutesAreFailClosed(t *testing.T) {
	failClosed := map[string]bool{
		"api-keys.create": true, "api-keys.rotate": true, "api-keys.revoke": true,
		"gateway-tokens.rotate": true, "gateway-tokens.revoke": true, "gateways.delete": true,
		"gateways.set-identity-provider": true, "gateways.remove-identity-provider": true,
		"agents.set-a2a-proxy": true, "agents.remove-a2a-proxy": true, "agents.remove-card-source": true,
	}
	for pattern, opts := range routeOptions {
		o := modkit.Options(opts)
		if o.AuditFailClosed != failClosed[o.AuditAction] {
			t.Errorf("%s (%s): fail-closed is %v, want %v", pattern, o.AuditAction, o.AuditFailClosed, failClosed[o.AuditAction])
		}
	}
}

// TestRouteOptionsNameDeclaredActionsAndEvents: every option names one of
// the manifest's audit actions and webhook events.
func TestRouteOptionsNameDeclaredActionsAndEvents(t *testing.T) {
	actions := map[string]bool{}
	for _, a := range gwaudit.Actions {
		actions[a.Name] = true
	}
	events := map[string]bool{}
	for _, e := range gwaudit.Events {
		events[e.Name] = true
	}
	for _, table := range []map[string][]modkit.RouteOption{routeOptions, internalRouteOptions} {
		for pattern, opts := range table {
			o := modkit.Options(opts)
			if o.AuditAction != "" && !actions[o.AuditAction] {
				t.Errorf("%s records %q, which gwaudit.Actions does not declare", pattern, o.AuditAction)
			}
			if o.Event != "" && !events[o.Event] {
				t.Errorf("%s emits %q, which gwaudit.Events does not declare", pattern, o.Event)
			}
		}
	}
}
