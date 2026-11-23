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

package gwsvc

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

// DeploymentAckHandler processes deployment acknowledgment messages from gateways
type DeploymentAckHandler struct {
	deploymentRepo gwrepo.DeploymentRepository
	afterLive      gwrepo.AfterLiveFunc
	usage          GatewayUsageHandler
}

// GatewayUsageHandler takes a gateway's usage message: budget allowance totals.
type GatewayUsageHandler interface {
	HandleUsage(ctx context.Context, gatewayID string, payload json.RawMessage)
}

// NewDeploymentAckHandler creates a new deployment ack handler. afterLive runs in the ack's transaction when a row
// goes DEPLOYED (the reconcile core's kind hooks). usage, when not nil, takes usage messages.
func NewDeploymentAckHandler(
	deploymentRepo gwrepo.DeploymentRepository, afterLive gwrepo.AfterLiveFunc, usage GatewayUsageHandler,
) *DeploymentAckHandler {
	return &DeploymentAckHandler{
		deploymentRepo: deploymentRepo,
		afterLive:      afterLive,
		usage:          usage,
	}
}

// HandleMessage parses a raw WebSocket message and processes it if it's a known event type.
// Returns true if the message was handled, false if it was unrecognized.
func (h *DeploymentAckHandler) HandleMessage(gatewayID string, data []byte) bool {
	var msg gwmodels.GatewayMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("DeploymentAckHandler: failed to parse gateway message", "gatewayID", gatewayID, "error", err)
		return false
	}

	switch msg.Type {
	case "deployment.ack":
		h.handleDeploymentAck(gatewayID, msg.Payload)
		return true
	case GatewayMessageBudgetUsage:
		if h.usage == nil {
			return false
		}
		h.usage.HandleUsage(context.Background(), gatewayID, msg.Payload)
		return true
	default:
		slog.Debug("DeploymentAckHandler: unhandled message type", "gatewayID", gatewayID, "type", msg.Type)
		return false
	}
}

func (h *DeploymentAckHandler) handleDeploymentAck(gatewayID string, payload json.RawMessage) {
	var ack gwmodels.DeploymentAck
	if err := json.Unmarshal(payload, &ack); err != nil {
		slog.Error("DeploymentAckHandler: failed to parse deployment ack payload", "gatewayID", gatewayID, "error", err)
		return
	}

	log := slog.With(
		"gatewayID", gatewayID,
		"deploymentID", ack.DeploymentID,
		"artifactID", ack.ArtifactID,
		"resourceType", ack.ResourceType,
		"action", ack.Action,
		"status", ack.Status,
	)

	if ack.Status == "failed" {
		log.Warn("Gateway reported deployment failure", "errorCode", ack.ErrorCode)
	} else {
		log.Info("Gateway deployment ack received")
	}

	// Only process acks for deployable gateway artifacts with current deployment state.
	// The gateway sends "agentproxy" for Agent acks up to v2026.09.24 and "Agent" after.
	switch ack.ResourceType {
	case "llmprovider", "llmproxy", "mcpproxy", "agentproxy", "Agent", "api":
		applied, err := h.deploymentRepo.ApplyAck(context.Background(), gatewayID, ack, h.afterLive)
		switch {
		case err != nil:
			log.Error("DeploymentAckHandler: failed to apply deployment ack", "error", err)
		case applied:
			log.Info("DeploymentAckHandler: deployment status updated")
		default:
			log.Info("DeploymentAckHandler: ack matched no transition, ignored")
		}
	default:
		log.Debug("DeploymentAckHandler: skipping ack for unsupported resource type")
	}
}
