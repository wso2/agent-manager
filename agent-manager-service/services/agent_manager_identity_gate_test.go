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

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// promotionPlacementStub is the promote fixture's provisioning stub for a
// deployment whose staging environment receives its identity by promotion. It has
// no promotion method at all: a workload promotion must not promote identity.
type promotionPlacementStub struct {
	*provisionForEnvIfMissingStub
	byPromotion map[string]bool
	kickedOff   int
}

func (s *promotionPlacementStub) IdentityCreatedByPromotion(_ context.Context, _, env string) (bool, error) {
	return s.byPromotion[env], nil
}

func (s *promotionPlacementStub) ProvisionForEnvironmentIfMissing(context.Context, string, string, string, string, models.AgentProvisioningType, string) (bool, error) {
	s.kickedOff++
	return false, nil
}

func promotionGateFixture(t *testing.T, state *AgentThunderBindingState, byPromotion map[string]bool) (*agentManagerService, *promotionPlacementStub, *bool) {
	t.Helper()
	s, promoteCalled := promoteAgentTestFixture(t, []client.EnvVar{{Key: "AMP_AGENTID_CLIENT_ID", Value: "staging-client-id"}}, nil)
	base := s.agentThunderProvisioning.(*provisionForEnvIfMissingStub)
	base.GetBindingStateFunc = func(context.Context, string, string, string, string) (*AgentThunderBindingState, error) {
		return state, nil
	}
	stub := &promotionPlacementStub{provisionForEnvIfMissingStub: base, byPromotion: byPromotion}
	s.agentThunderProvisioning = stub
	return s, stub, promoteCalled
}

func promoteToStaging(s *agentManagerService, t *testing.T) error {
	t.Helper()
	return s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev", TargetEnvironment: "staging",
	})
}

func TestPromoteAgentRequiresACompletedIdentityInAPromotionCreatedTarget(t *testing.T) {
	cases := map[string]struct {
		state  *AgentThunderBindingState
		failed bool
		reason string
	}{
		"no binding yet": {state: nil},
		"pending":        {state: &AgentThunderBindingState{Status: models.AgentThunderStatusPending}},
		"in progress":    {state: &AgentThunderBindingState{Status: models.AgentThunderStatusInProgress}},
		"failed": {
			state:  &AgentThunderBindingState{Status: models.AgentThunderStatusFailed, LastError: "Identity provisioning failed. Please retry."},
			failed: true, reason: "Identity provisioning",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s, stub, promoteCalled := promotionGateFixture(t, tc.state, map[string]bool{"staging": true})

			err := promoteToStaging(s, t)

			var notReady *utils.AgentIdentityNotReadyError
			require.ErrorAs(t, err, &notReady, "the refusal is a typed error the controller maps to a 409")
			require.Equal(t, "staging", notReady.Environment)
			require.Equal(t, tc.failed, notReady.Failed)
			require.Contains(t, notReady.Message, `"staging"`, "the message names the environment")
			if tc.reason != "" {
				require.Contains(t, notReady.Reason, tc.reason, "the short reason is carried")
			}
			require.False(t, *promoteCalled, "nothing may reach the workload without the target's identity")
			require.Zero(t, stub.kickedOff, "a promotion created identity is never provisioned from the workload promotion")
		})
	}
}

func TestPromoteAgentPromotesTheWorkloadWhenThePromotionCreatedTargetIdentityIsCompleted(t *testing.T) {
	s, stub, promoteCalled := promotionGateFixture(t,
		&AgentThunderBindingState{Status: models.AgentThunderStatusCompleted, HasSecret: true}, map[string]bool{"staging": true})

	require.NoError(t, promoteToStaging(s, t))

	require.True(t, *promoteCalled)
	require.Zero(t, stub.kickedOff, "the workload promotion does not start an identity either")
}

func TestPromoteAgentKeepsTheDirectIdentityFlowForAnEnvironmentThatIsNotPromotionCreated(t *testing.T) {
	// Legacy placement: the stub reports no promotion created environment, so the
	// binding kick-off and the readiness poll behave as they always did.
	s, stub, promoteCalled := promotionGateFixture(t,
		&AgentThunderBindingState{Status: models.AgentThunderStatusCompleted, HasSecret: true}, map[string]bool{})

	require.NoError(t, promoteToStaging(s, t))

	require.True(t, *promoteCalled)
	require.Equal(t, 1, stub.kickedOff, "a direct environment still gets its binding kicked off")
}

func TestPlatformHostedPromotionIsRefusedWhenTheFirstEnvironmentIsNotWhereIdentityIsManaged(t *testing.T) {
	s, _, promoteCalled := promotionGateFixture(t, nil, map[string]bool{"dev": true, "staging": true})

	err := promoteToStaging(s, t)

	require.ErrorIs(t, err, utils.ErrIdentityHomeMismatch)
	require.False(t, *promoteCalled)
}
