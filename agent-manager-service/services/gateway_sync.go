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
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ListGatewayDeployments is the gateway's startup sync list: what it should hold, as it stored it. It only reads.
func (s *GatewayInternalAPIService) ListGatewayDeployments(ctx context.Context, ouID, gatewayID string) ([]models.ControlPlaneDeployment, error) {
	rows, err := s.deploymentRepo.ListForGatewaySync(ctx, ouID, gatewayID)
	if err != nil {
		return nil, err
	}
	deployments := make([]models.ControlPlaneDeployment, 0, len(rows))
	for _, row := range rows {
		kind, artifactID, ok := s.artifacts.GatewayArtifact(row.ArtifactRef)
		if !ok {
			slog.Warn("GatewayInternalAPIService: skipping a gateway deployment of a kind the gateway doesn't sync",
				"artifactUUID", row.ArtifactUUID, "kind", row.Kind, "gatewayID", gatewayID)
			continue
		}
		deployedAt := row.DeployedAt.UTC()
		deployments = append(deployments, models.ControlPlaneDeployment{
			ArtifactID:   artifactID,
			DeploymentID: row.DeploymentID,
			Kind:         kind,
			State:        row.State,
			DeployedAt:   deployedAt,
			Etag:         syncEtag(artifactID, row.DeploymentID, deployedAt),
		})
	}
	return deployments, nil
}

// syncEtag is a deterministic UUIDv7: deployedAt's milliseconds, then bits hashed from the artifact and deployment
// ids. The gateway uses it as the correlation id of what it applies, which is also its own event id, deduplicated
// per gateway. Hashing the artifact id too keeps undeployed entries, which all list an empty deployment id, apart.
func syncEtag(artifactID, deploymentID string, deployedAt time.Time) string {
	hash := sha256.Sum256([]byte(artifactID + "/" + deploymentID))
	var b [16]byte
	ms := uint64(deployedAt.UnixMilli())
	binary.BigEndian.PutUint16(b[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(b[2:6], uint32(ms))
	copy(b[6:], hash[:10])
	b[6] = (b[6] & 0x0F) | 0x70 // version 7
	b[8] = (b[8] & 0x3F) | 0x80 // RFC 9562 variant
	return uuid.UUID(b).String()
}

// FetchDeploymentBatch returns the gateway's deployments among deploymentIDs, with secrets resolved as the single pull
// resolves them. An id that isn't one of this gateway's deployments is skipped, and so is a deployment whose secrets
// can't be resolved, so one bad deployment doesn't fail the gateway's whole batch.
func (s *GatewayInternalAPIService) FetchDeploymentBatch(
	ctx context.Context, ouID, gatewayID string, deploymentIDs []string,
) ([]utils.BatchDeployment, error) {
	ids := make([]uuid.UUID, 0, len(deploymentIDs))
	for _, id := range deploymentIDs {
		if parsed, err := uuid.Parse(id); err == nil {
			ids = append(ids, parsed)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	contents, err := s.deploymentRepo.ListContentForGateway(ctx, ouID, gatewayID, ids)
	if err != nil {
		return nil, err
	}
	batch := make([]utils.BatchDeployment, 0, len(contents))
	for _, c := range contents {
		kind, artifactID, ok := s.artifacts.GatewayArtifact(c.ArtifactRef)
		if !ok {
			continue
		}
		resolved, err := s.resolveSecretsFor(ctx, kind, string(c.Content))
		if err != nil {
			slog.Error("GatewayInternalAPIService: leaving a deployment out of the gateway's batch: its secrets can't be resolved",
				"deploymentID", c.DeploymentID, "artifactUUID", c.ArtifactUUID, "gatewayID", gatewayID, "error", err)
			continue
		}
		batch = append(batch, utils.BatchDeployment{
			DeploymentID: c.DeploymentID.String(), Kind: kind, ArtifactID: artifactID, Content: []byte(resolved),
		})
	}
	return batch, nil
}

// resolveSecretsFor decrypts the secrets in a deployment's YAML, by the kind the gateway knows the artifact as.
func (s *GatewayInternalAPIService) resolveSecretsFor(ctx context.Context, gatewayKind, content string) (string, error) {
	switch gatewayKind {
	case models.KindLLMProvider:
		return s.resolveSecretsInYAML(ctx, content, "upstream.auth")
	case models.KindLLMProxy:
		return s.resolveSecretsInYAML(ctx, content, "provider.auth")
	default:
		return s.resolveAllSecretsInYAML(ctx, content)
	}
}

// CheckArtifactsExist answers, for each id the gateway asks about, whether the artifact still exists in the org,
// whatever it is deployed to. An id may be an artifact UUID or an LLM proxy handle. The gateway keeps an artifact
// that exists, so an artifact with no deployment keeps its keys there.
func (s *GatewayInternalAPIService) CheckArtifactsExist(ctx context.Context, ouID string, artifactIDs []string) ([]models.ArtifactExistence, error) {
	var uuids []uuid.UUID
	for _, id := range artifactIDs {
		if parsed, err := uuid.Parse(id); err == nil {
			uuids = append(uuids, parsed)
		}
	}
	existing, err := s.artifactRepo.ListExisting(ctx, ouID, uuids, artifactIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to check artifacts: %w", err)
	}
	byUUID := make(map[uuid.UUID]bool, len(existing))
	byProxyHandle := make(map[string]bool, len(existing))
	for _, ref := range existing {
		byUUID[ref.ArtifactUUID] = true
		if ref.Kind == models.KindLLMProxy {
			byProxyHandle[ref.Handle] = true
		}
	}
	answers := make([]models.ArtifactExistence, 0, len(artifactIDs))
	for _, id := range artifactIDs {
		parsed, err := uuid.Parse(id)
		exists := byProxyHandle[id] || (err == nil && byUUID[parsed])
		answers = append(answers, models.ArtifactExistence{ArtifactID: id, Exists: exists})
	}
	return answers, nil
}
