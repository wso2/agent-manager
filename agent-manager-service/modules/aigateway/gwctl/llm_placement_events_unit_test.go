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

package gwctl

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
)

type raisedEvent struct {
	eventType string
	data      map[string]any
}

// eventRecorder stands in for the core's event scope and keeps the extra events raised.
type eventRecorder struct{ raised []raisedEvent }

func (r *eventRecorder) EventEnvironment(string) {}
func (r *eventRecorder) EventData(string, any)   {}
func (r *eventRecorder) EventAlso(eventType string, data map[string]any) {
	r.raised = append(r.raised, raisedEvent{eventType, data})
}

// A deployed event is raised per newly placed gateway, an undeployed event per removed one, and
// failures and redeploys raise nothing.
func TestEmitPlacementEvents(t *testing.T) {
	rec := &eventRecorder{}
	ctx := modkit.WithEventScope(context.Background(), rec)
	id := uuid.New()
	provider := &gwmodels.LLMProvider{UUID: id, Artifact: &gwmodels.Artifact{Handle: "openai-prod"}}

	emitPlacementEvents(ctx, provider,
		[]gwsvc.DeploymentResult{
			{GatewayID: "gw-new", Success: true, Placed: true},
			{GatewayID: "gw-current", Success: true},
			{GatewayID: "gw-failed", Success: false, Placed: true},
		},
		[]gwsvc.DeploymentResult{
			{GatewayID: "gw-removed", Success: true},
			{GatewayID: "gw-stuck", Success: false},
		})

	assert.Equal(t, []raisedEvent{
		{gwaudit.LLMProviderDeployed, map[string]any{"providerId": id.String(), "provider": "openai-prod", "gatewayId": "gw-new"}},
		{gwaudit.LLMProviderUndeployed, map[string]any{"providerId": id.String(), "provider": "openai-prod", "gatewayId": "gw-removed"}},
	}, rec.raised)

	// A provider without its artifact still names its UUID.
	rec.raised = nil
	emitPlacementEvents(ctx, &gwmodels.LLMProvider{UUID: id}, nil, []gwsvc.DeploymentResult{{GatewayID: "gw", Success: true}})
	assert.Equal(t, []raisedEvent{{gwaudit.LLMProviderUndeployed, map[string]any{"providerId": id.String(), "gatewayId": "gw"}}}, rec.raised)
}
