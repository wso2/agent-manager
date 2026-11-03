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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The gateway stores times at millisecond precision and GET /deployments must echo exactly what was sent.
func TestNewPerformedAt(t *testing.T) {
	colombo := time.FixedZone("+0530", 5*3600+30*60)
	in := time.Date(2026, 10, 29, 16, 4, 5, 123_456_789, colombo)

	got := NewPerformedAt(in)

	assert.Equal(t, time.UTC, got.Location())
	assert.Equal(t, time.Date(2026, 10, 29, 10, 34, 5, 123_000_000, time.UTC), got)
}

func TestNewPerformedAtIsNeverZero(t *testing.T) {
	before := time.Now().Add(-time.Second)
	got := NewPerformedAt(time.Time{})
	assert.False(t, got.IsZero())
	assert.True(t, got.After(before), "a zero time is replaced by now")
}

func TestDeploymentStatusValues(t *testing.T) {
	for want, status := range map[string]DeploymentStatus{
		"PENDING":     DeploymentStatusPending,
		"DEPLOYING":   DeploymentStatusDeploying,
		"DEPLOYED":    DeploymentStatusDeployed,
		"UNDEPLOYING": DeploymentStatusUndeploying,
		"UNDEPLOYED":  DeploymentStatusUndeployed,
		"FAILED":      DeploymentStatusFailed,
	} {
		assert.Equal(t, want, string(status))
	}
}

func TestPlatformAgentExposureTable(t *testing.T) {
	assert.Equal(t, "platform_agent_exposures", PlatformAgentExposure{}.TableName())
}

// UserStatusOf is the design's table of what a user is shown for each row shape.
func TestUserStatusOf(t *testing.T) {
	due := time.Now()
	row := func(status DeploymentStatus, nextAttemptAt *time.Time, renderReason, statusReason string) DeploymentStatusRecord {
		return DeploymentStatusRecord{
			Status: status, NextAttemptAt: nextAttemptAt, RenderReason: renderReason, StatusReason: statusReason,
		}
	}
	for name, tc := range map[string]struct {
		row  DeploymentStatusRecord
		want UserStatus
	}{
		"pending, queue due": {
			row(DeploymentStatusPending, &due, "waiting for the agent's endpoint", ""),
			UserStatus{DeploymentStatusDeploying, "waiting for the agent's endpoint"},
		},
		"pending, queue given up": {
			row(DeploymentStatusPending, nil, "render failed", ""),
			UserStatus{DeploymentStatusFailed, "render failed"},
		},
		"deployed, a redeploy is owed": {
			row(DeploymentStatusDeployed, &due, "waiting for the agent's endpoint", ""),
			UserStatus{DeploymentStatusDeploying, "waiting for the agent's endpoint"},
		},
		"failed, a redeploy is owed": {
			row(DeploymentStatusFailed, &due, "", "GATEWAY_PROCESSING_ERROR"),
			UserStatus{DeploymentStatusDeploying, ""},
		},
		"deployed, the redeploy was given up": {
			row(DeploymentStatusDeployed, nil, "render failed", ""),
			UserStatus{DeploymentStatusDeployed, "render failed"},
		},
		"failed by the gateway": {
			row(DeploymentStatusFailed, nil, "", "GATEWAY_PROCESSING_ERROR"),
			UserStatus{DeploymentStatusFailed, "GATEWAY_PROCESSING_ERROR"},
		},
		"timed out": {
			row(DeploymentStatusFailed, nil, "", "DEPLOYMENT_TIMEOUT"),
			UserStatus{DeploymentStatusFailed, "DEPLOYMENT_TIMEOUT"},
		},
		"deploying": {row(DeploymentStatusDeploying, nil, "", ""), UserStatus{DeploymentStatusDeploying, ""}},
		"deployed":  {row(DeploymentStatusDeployed, nil, "", ""), UserStatus{DeploymentStatusDeployed, ""}},
		"undeploying": {
			row(DeploymentStatusUndeploying, nil, "", ""), UserStatus{DeploymentStatusUndeploying, ""},
		},
		"undeployed": {row(DeploymentStatusUndeployed, nil, "", ""), UserStatus{DeploymentStatusUndeployed, ""}},
		"pending, never shown as pending": {
			row(DeploymentStatusPending, nil, "", ""), UserStatus{DeploymentStatusDeploying, ""},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, UserStatusOf(tc.row))
		})
	}
}
