//
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

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
)

func conds(c ...gen.Condition) *[]gen.Condition { return &c }

func cond(condType, status, reason, message string) gen.Condition {
	return gen.Condition{
		Type:    condType,
		Status:  gen.ConditionStatus(status),
		Reason:  reason,
		Message: &message,
	}
}

func TestReconcileBlockFromConditions(t *testing.T) {
	t.Run("nil conditions are not blocked", func(t *testing.T) {
		assert.Nil(t, reconcileBlockFromConditions(nil))
	})

	t.Run("no Ready condition is not blocked", func(t *testing.T) {
		// A component that has never been reconciled publishes no Ready condition. That is
		// absence of evidence, not evidence of a problem, so deploys must not be refused.
		assert.Nil(t, reconcileBlockFromConditions(conds(
			cond("Progressing", "True", "Reconciling", ""),
		)))
	})

	t.Run("Ready=True is not blocked", func(t *testing.T) {
		assert.Nil(t, reconcileBlockFromConditions(conds(
			cond("Ready", "True", "Reconciled", "all good"),
		)))
	})

	t.Run("workload type mismatch is blocked", func(t *testing.T) {
		// The case this guard exists for: the ComponentType was replaced with a different
		// workloadType, so the controller stops before cutting a new ComponentRelease.
		got := reconcileBlockFromConditions(conds(
			cond("Ready", "False", "InvalidConfiguration",
				"WorkloadType mismatch: component specifies deployment but ComponentType has proxy"),
		))
		require.NotNil(t, got)
		assert.Equal(t, "InvalidConfiguration", got.Reason)
		assert.Contains(t, got.Message, "WorkloadType mismatch")
	})

	t.Run("missing component type is blocked", func(t *testing.T) {
		got := reconcileBlockFromConditions(conds(
			cond("Ready", "False", "ComponentTypeNotFound", `ComponentType "agent-api" not found`),
		))
		require.NotNil(t, got)
		assert.Equal(t, "ComponentTypeNotFound", got.Reason)
	})

	t.Run("awaiting first build is not blocked", func(t *testing.T) {
		// WorkloadNotFound is the normal state before a build completes; refusing deploys here
		// would break the ordinary create-then-deploy flow.
		assert.Nil(t, reconcileBlockFromConditions(conds(
			cond("Ready", "False", "WorkloadNotFound", "no workload yet"),
		)))
	})

	t.Run("unknown reasons fail open", func(t *testing.T) {
		// Reasons added upstream must not silently start refusing deploys.
		assert.Nil(t, reconcileBlockFromConditions(conds(
			cond("Ready", "False", "SomeFutureReason", "unrecognised"),
		)))
	})

	t.Run("blocking reason is found among other conditions", func(t *testing.T) {
		got := reconcileBlockFromConditions(conds(
			cond("Progressing", "False", "Stalled", ""),
			cond("Ready", "False", "TraitNotFound", `Trait "api-configuration" not found`),
		))
		require.NotNil(t, got)
		assert.Equal(t, "TraitNotFound", got.Reason)
	})

	t.Run("blocking condition without a message still reports the reason", func(t *testing.T) {
		got := reconcileBlockFromConditions(&[]gen.Condition{
			{Type: "Ready", Status: "False", Reason: "ProjectNotFound"},
		})
		require.NotNil(t, got)
		assert.Equal(t, "ProjectNotFound", got.Reason)
		assert.Empty(t, got.Message)
	})
}
