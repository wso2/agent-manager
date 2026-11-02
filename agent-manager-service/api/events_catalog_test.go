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

package api

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/events"
)

// Every route the event catalog maps must be a real, registered route, or
// its event would silently never fire after a route is renamed.
func TestEventCatalogRoutesAreRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, meta := range registerAllRoutesForAudit(t).Routes() {
		registered[meta.Pattern] = true
	}
	for _, pattern := range events.RoutePatterns() {
		if !registered[pattern] {
			t.Errorf("event catalog maps %q, which is not a registered route", pattern)
		}
		name, _, _ := events.RouteEventType(pattern)
		if _, ok := events.Lookup(name); !ok {
			t.Errorf("route %q emits %q, which is not in the event catalog", pattern, name)
		}
	}
}
