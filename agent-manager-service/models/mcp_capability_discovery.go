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
package models

import (
	"time"

	"github.com/google/uuid"
)

// Capability discovery states of a project MCP proxy binding.
const (
	// MCPCapabilityDiscoveryPending: due at NextAttemptAt.
	MCPCapabilityDiscoveryPending = "PENDING"
	// MCPCapabilityDiscoveryDiscovered: the binding's capabilities are the ones last discovered, at DiscoveredAt.
	MCPCapabilityDiscoveryDiscovered = "DISCOVERED"
	// MCPCapabilityDiscoveryFailed: given up on; LastError says why. A redeploy or a refresh queues it again.
	MCPCapabilityDiscoveryFailed = "FAILED"
)

// MCPCapabilityDiscoveryState is a project MCP proxy binding's capability discovery, stored in its endpoint
// configuration next to the capabilities it discovers, as an org proxy's capabilities are stored. It is bookkeeping,
// not configuration: a binding PUT carries it over and never compares it.
type MCPCapabilityDiscoveryState struct {
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attemptCount,omitempty"`
	NextAttemptAt *time.Time `json:"nextAttemptAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
	DiscoveredAt  *time.Time `json:"discoveredAt,omitempty"`
	// Token changes each time the binding is queued, so an attempt from before a later queueing records nothing.
	Token string `json:"token"`
}

// MCPCapabilityDiscovery is a binding's discovery as a claim returns it: the state plus the binding it belongs to.
type MCPCapabilityDiscovery struct {
	MCPCapabilityDiscoveryState
	ArtifactUUID uuid.UUID
	OUID         string
}

// MCPCapabilityDiscoveryStatus is a binding's capability discovery as the API shows it.
type MCPCapabilityDiscoveryStatus struct {
	// Status is Pending, Discovered or Failed.
	Status       string     `json:"status"`
	LastError    string     `json:"lastError,omitempty"`
	DiscoveredAt *time.Time `json:"discoveredAt,omitempty"`
}

// MCPCapabilityDiscoveryStatusOf is the API view of a binding's discovery state.
func MCPCapabilityDiscoveryStatusOf(d MCPCapabilityDiscoveryState) *MCPCapabilityDiscoveryStatus {
	status := map[string]string{
		MCPCapabilityDiscoveryPending:    "Pending",
		MCPCapabilityDiscoveryDiscovered: "Discovered",
		MCPCapabilityDiscoveryFailed:     "Failed",
	}[d.Status]
	return &MCPCapabilityDiscoveryStatus{Status: status, LastError: d.LastError, DiscoveredAt: d.DiscoveredAt}
}
