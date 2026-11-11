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
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// waitingForEndpoint is what a platform agent's row waits on until its release binding publishes a service URL.
const waitingForEndpoint = "waiting for the agent's endpoint"

// PlatformAgentExposures is what an agent kind reads of a platform agent's exposure: the row, the endpoint its
// release binding serves, and the ingress gateway it is placed on.
type PlatformAgentExposures struct {
	exposureRepo repositories.PlatformAgentExposureRepository
	gatewayRepo  repositories.GatewayRepository
	ocClient     repositories.PlatformRepository
}

// NewPlatformAgentExposures creates PlatformAgentExposures.
func NewPlatformAgentExposures(
	exposureRepo repositories.PlatformAgentExposureRepository,
	gatewayRepo repositories.GatewayRepository,
	ocClient repositories.PlatformRepository,
) *PlatformAgentExposures {
	return &PlatformAgentExposures{exposureRepo: exposureRepo, gatewayRepo: gatewayRepo, ocClient: ocClient}
}

// exposure is the artifact's exposure, read in tx when non-nil. ErrPlatformAgentExposureNotFound passes through
// unwrapped: each kind decides what an artifact without one means.
func (e *PlatformAgentExposures) exposure(
	ctx context.Context, tx *gorm.DB, artifact models.ArtifactRef,
) (*models.PlatformAgentExposure, error) {
	repo := e.exposureRepo
	if tx != nil {
		repo = repo.WithTx(tx)
	}
	exposure, err := repo.GetByArtifact(ctx, artifact.OUID, artifact.ArtifactUUID)
	if errors.Is(err, repositories.ErrPlatformAgentExposureNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the agent's exposure: %w", err)
	}
	return exposure, nil
}

// serviceURL is the endpoint the agent's release binding serves; the render waits while there is none.
func (e *PlatformAgentExposures) serviceURL(ctx context.Context, exposure *models.PlatformAgentExposure) (string, error) {
	// Platform-hosted agents need an agent runtime, which is not available.
	return "", fmt.Errorf("%w: agent %s has no hosted endpoint", utils.ErrNotImplemented, exposure.AgentName)
}

// target is the record's environment's ingress gateway, read in the transaction ctx carries, if any.
func (e *PlatformAgentExposures) target(ctx context.Context, record reconcile.Record) (uuid.UUID, bool, error) {
	gateway, err := findIngressGateway(gatewaysIn(ctx, e.gatewayRepo), record.OUID, record.EnvironmentUUID)
	if err != nil || gateway == nil {
		return uuid.Nil, false, err
	}
	return gateway.UUID, true, nil
}

// unplacedExposureArm selects the protocol's exposures whose artifact has no row yet, in environments with an
// ingress gateway, as one UNION arm of pageUnplacedRecords.
func unplacedExposureArm(protocol models.AgentExposureProtocol, ouID string, env *uuid.UUID) (string, []any) {
	condition, args := reconcile.UnplacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: "e.environment_uuid",
	}, models.IngressGatewayRoles, ouID, env)
	arm := `SELECT a.ou_id, a.uuid AS artifact_uuid, a.handle, a.kind, e.environment_uuid
			FROM platform_agent_exposures e JOIN artifacts a ON a.uuid = e.artifact_uuid
			WHERE e.protocol = ? AND ` + condition
	return arm, append([]any{protocol}, args...)
}

// placedExposureArm selects the protocol's exposures in env whose artifact is placed on gateway, as one UNION arm
// of a kind's PlacedOn query.
func placedExposureArm(protocol models.AgentExposureProtocol, env, gateway uuid.UUID) (string, []any) {
	condition, args := reconcile.PlacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: "e.environment_uuid",
	}, env, gateway)
	arm := `SELECT 1 FROM platform_agent_exposures e JOIN artifacts a ON a.uuid = e.artifact_uuid
			WHERE e.protocol = ? AND ` + condition
	return arm, append([]any{protocol}, args...)
}
