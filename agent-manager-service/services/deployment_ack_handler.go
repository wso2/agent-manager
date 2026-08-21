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
	"encoding/json"
	"log/slog"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// DeploymentAckHandler processes deployment acknowledgment messages from gateways
type DeploymentAckHandler struct {
	deploymentRepo repositories.DeploymentRepository
}

// NewDeploymentAckHandler creates a new deployment ack handler
func NewDeploymentAckHandler(deploymentRepo repositories.DeploymentRepository) *DeploymentAckHandler {
	return &DeploymentAckHandler{
		deploymentRepo: deploymentRepo,
	}
}

// HandleMessage parses a raw WebSocket message and processes it if it's a known event type.
// Returns true if the message was handled, false if it was unrecognized.
func (h *DeploymentAckHandler) HandleMessage(gatewayID string, data []byte) bool {
	var msg models.GatewayMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		//nolint:forbidigo // gateway WebSocket message handler; the message is not tied to an API request
		slog.Warn("DeploymentAckHandler: failed to parse gateway message", "gateway_id", gatewayID, "error", err)
		return false
	}

	switch msg.Type {
	case "deployment.ack":
		h.handleDeploymentAck(gatewayID, msg.Payload)
		return true
	default:
		//nolint:forbidigo // gateway WebSocket message handler; the message is not tied to an API request
		slog.Debug("DeploymentAckHandler: unhandled message type", "gateway_id", gatewayID, "type", msg.Type)
		return false
	}
}

func (h *DeploymentAckHandler) handleDeploymentAck(gatewayID string, payload json.RawMessage) {
	var ack models.DeploymentAckPayload
	if err := json.Unmarshal(payload, &ack); err != nil {
		//nolint:forbidigo // gateway WebSocket message handler; the message is not tied to an API request
		slog.Error("DeploymentAckHandler: failed to parse deployment ack payload", "gateway_id", gatewayID, "error", err)
		return
	}

	log := slog.With(
		"gateway_id", gatewayID,
		"deployment_id", ack.DeploymentID,
		"artifact_id", ack.ArtifactID,
		"resource_type", ack.ResourceType,
		"action", ack.Action,
		"status", ack.Status,
	)

	if ack.Status == "failed" {
		log.Warn("Gateway reported deployment failure", "error_code", ack.ErrorCode)
	} else {
		log.Info("Gateway deployment ack received")
	}

	// Only process acks for deployable gateway artifacts with current deployment state.
	switch ack.ResourceType {
	case "llmprovider", "llmproxy", "mcpproxy":
		// Update deployment status based on ack
		if ack.DeploymentID == "" {
			log.Warn("DeploymentAckHandler: missing deploymentID in ack, skipping status update")
			return
		}

		var status models.DeploymentStatus
		switch {
		case ack.Action == "deploy" && ack.Status == "success":
			status = models.DeploymentStatusDeployed
		case ack.Action == "undeploy" && ack.Status == "success":
			status = models.DeploymentStatusUndeployed
		case ack.Action == "deploy" && ack.Status == "failed":
			// Deploy failed — mark as undeployed so the user knows the deployment didn't succeed
			status = models.DeploymentStatusUndeployed
		case ack.Action == "undeploy" && ack.Status == "failed":
			// Undeploy failed — keep as deployed since the artifact is still running on the gateway
			status = models.DeploymentStatusDeployed
		default:
			log.Debug("DeploymentAckHandler: no status update needed for ack")
			return
		}

		if _, err := h.deploymentRepo.UpdateStatusByDeploymentID(ack.DeploymentID, gatewayID, status); err != nil {
			log.Error("DeploymentAckHandler: failed to update deployment status", "target_status", status, "error", err)
		} else {
			log.Info("DeploymentAckHandler: deployment status updated", "new_status", status)
		}
	default:
		log.Debug("DeploymentAckHandler: skipping ack for unsupported resource type")
	}
}
