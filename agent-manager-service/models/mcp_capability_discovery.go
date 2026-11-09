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

// MCPCapabilityDiscovery is the capability discovery of one project MCP proxy binding, keyed by the binding's
// gateway artifact.
type MCPCapabilityDiscovery struct {
	ArtifactUUID  uuid.UUID  `gorm:"column:artifact_uuid;primaryKey"`
	OUID          string     `gorm:"column:ou_id"`
	Status        string     `gorm:"column:status"`
	AttemptCount  int        `gorm:"column:attempt_count"`
	NextAttemptAt *time.Time `gorm:"column:next_attempt_at"`
	LastError     string     `gorm:"column:last_error"`
	DiscoveredAt  *time.Time `gorm:"column:discovered_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at"`
}

// TableName returns the table name for the MCPCapabilityDiscovery model.
func (MCPCapabilityDiscovery) TableName() string {
	return "mcp_capability_discoveries"
}

// MCPCapabilityDiscoveryStatus is a binding's capability discovery as the API shows it.
type MCPCapabilityDiscoveryStatus struct {
	// Status is Pending, Discovered or Failed.
	Status       string     `json:"status"`
	LastError    string     `json:"lastError,omitempty"`
	DiscoveredAt *time.Time `json:"discoveredAt,omitempty"`
}

// MCPCapabilityDiscoveryStatusOf is the API view of a discovery row.
func MCPCapabilityDiscoveryStatusOf(d MCPCapabilityDiscovery) *MCPCapabilityDiscoveryStatus {
	status := map[string]string{
		MCPCapabilityDiscoveryPending:    "Pending",
		MCPCapabilityDiscoveryDiscovered: "Discovered",
		MCPCapabilityDiscoveryFailed:     "Failed",
	}[d.Status]
	return &MCPCapabilityDiscoveryStatus{Status: status, LastError: d.LastError, DiscoveredAt: d.DiscoveredAt}
}
