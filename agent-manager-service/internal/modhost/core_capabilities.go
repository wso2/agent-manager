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
package modhost

import (
	"fmt"
	"slices"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
)

// CoreCapabilities are the capabilities the core provides to every module. A module lists one in Requires like
// any other; it is always satisfied, and no module may provide it. The server supplies the implementations in
// Deps.Capabilities; a host without them (a CLI command) resolves them as missing at call time.
var CoreCapabilities = []string{identityv1.ID, notifyv1.ID}

// coreProvider is the provider name a core capability resolves to.
const coreProvider = "core"

func isCoreCapability(capability string) bool {
	return slices.Contains(CoreCapabilities, capability)
}

func checkCoreCapabilities(caps map[string]any) {
	for id := range caps {
		if !isCoreCapability(id) {
			panic(fmt.Sprintf("modhost: %s is not a core capability", id))
		}
	}
}

// seedCore puts the core's capabilities into a snapshot being built.
func (h *Host) seedCore(v *view) *view {
	for id, impl := range h.deps().Capabilities {
		v.caps[id] = impl
		v.capBy[id] = coreProvider
	}
	return v
}
