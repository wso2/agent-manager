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
	"strings"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"gorm.io/gorm"
)

// Gateway placement errors.
//
// Only errNoGatewayForEnvironment is tolerated by callers — it is a timing condition
// (the window between environment creation and the bootstrap job registering the
// gateway), not a misconfiguration. Every other error is a 400. Because roles are policy
// rather than capability, messages are worded as policy: "no gateway in this environment
// is designated for egress", not "no gateway can do egress".
var (
	// errNoGatewayForEnvironment means no gateway is mapped to this environment.
	// Renamed from errNoActiveGatewayForEnvironment: the condition is now membership,
	// not liveness. Tolerated.
	errNoGatewayForEnvironment = errors.New("no gateway is mapped to this environment")

	// errNoEgressGatewayForEnvironment means the environment has gateways but none is
	// designated for egress.
	errNoEgressGatewayForEnvironment = errors.New("no gateway in this environment is designated for egress")

	// errAmbiguousEgressGateway means the environment has 2+ egress gateways and the
	// caller named none.
	errAmbiguousEgressGateway = errors.New("this environment has more than one egress gateway; specify which one to use")

	// errInvalidEgressGateway means the named gateway is not in the environment or is
	// not designated for egress.
	errInvalidEgressGateway = errors.New("the specified gateway is not an egress gateway in this environment")

	// errPlacementFixed means the named gateway differs from where the artifact is
	// already deployed in this environment. There is no undeploy path, so a move is
	// refused rather than silently ignored.
	errPlacementFixed = errors.New("placement is fixed for this environment; delete and recreate the binding to change it")

	// errSelfHostedAnchor means an agent's or monitor's LLM proxy, which AMS owns and serves to workloads in
	// OpenChoreo, is placed on a self-hosted gateway. No path places one there, so this is a backstop only.
	errSelfHostedAnchor = errors.New("the agent's or monitor's LLM proxy is placed on a self-hosted gateway")
)

// resolveEgressGatewayForEnvironment selects the environment's egress gateway, either
// validating the caller's choice or inferring it when the environment has exactly one
// managed one.
//
// Deliberately unfiltered by is_active and unlimited by Limit. is_active is WebSocket
// liveness (false at registration, true on connect, false on disconnect) and flaps;
// filtering on it would make the same request succeed or fail depending on connectivity,
// and — worse — when one of two candidates is offline the survivor would silently win and
// anchoring would then pin the artifact there permanently. Delivery does not depend on
// it: the event hub inserts durably and gateways bulk-sync on reconnect. A Limit would
// make ambiguity undetectable.
//
// Self-hosted gateways are never inferred: only a request that names one selects it.
func resolveEgressGatewayForEnvironment(
	repo repositories.GatewayRepository, ouID string, envUUID uuid.UUID, requested *string,
) (*models.Gateway, error) {
	envIDStr := envUUID.String()
	gateways, err := repo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: ouID,
		EnvironmentID:  &envIDStr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways for environment %s: %w", envIDStr, err)
	}
	if len(gateways) == 0 {
		return nil, errNoGatewayForEnvironment
	}

	candidates := make([]*models.Gateway, 0, len(gateways))
	for _, gw := range gateways {
		if gw.IsEgressCapable() {
			candidates = append(candidates, gw)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w (environment %s)", errNoEgressGatewayForEnvironment, envIDStr)
	}

	if requested != nil && *requested != "" {
		for _, gw := range candidates {
			if gw.UUID.String() == *requested {
				return gw, nil
			}
		}
		return nil, fmt.Errorf("%w: gateway %s (candidates: %s)",
			errInvalidEgressGateway, *requested, describeGateways(candidates))
	}

	managed := make([]*models.Gateway, 0, len(candidates))
	for _, gw := range candidates {
		if gw.IsManaged() {
			managed = append(managed, gw)
		}
	}
	if len(managed) == 0 {
		return nil, fmt.Errorf("%w (environment %s)", errNoEgressGatewayForEnvironment, envIDStr)
	}
	if len(managed) > 1 {
		return nil, fmt.Errorf("%w (environment %s, candidates: %s)",
			errAmbiguousEgressGateway, envIDStr, describeGateways(managed))
	}
	return managed[0], nil
}

// resolveEgressGatewayForArtifact anchors selection to the gateway the artifact is
// already placed on in this environment, falling through to environment-only selection
// only when the artifact isn't placed there. That fallthrough is the sole place
// ambiguity can arise.
//
// deployedGatewayIDs is the artifact's placed gateway_uuid set (rows heading for DEPLOYED,
// in any status), supplied by the caller from PlacedGateways.
//
// The anchor's role is deliberately NOT re-validated: the role is immutable after
// registration, so an artifact's existing gateway is always still egress-capable. Role is
// checked only when selecting a new placement.
//
// Lookup failures are returned rather than skipped. The anchor is load-bearing: swallowing
// them would fall through to environment-only selection as if the artifact had no
// deployment, and a caller naming a different gateway would then dual-home the binding that
// errPlacementFixed exists to prevent. Only a genuinely absent anchor — no mapping to this
// environment, or a deleted gateway — continues to the next candidate.
func resolveEgressGatewayForArtifact(
	repo repositories.GatewayRepository, ouID string, envUUID uuid.UUID,
	deployedGatewayIDs []string, requested *string,
) (*models.Gateway, error) {
	envIDStr := envUUID.String()
	for _, gwID := range deployedGatewayIDs {
		exists, err := repo.EnvironmentMappingExists(gwID, envIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to check gateway %s mapping to environment %s: %w", gwID, envIDStr, err)
		}
		if !exists {
			continue
		}
		gw, err := repo.GetByUUID(gwID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // gateway deleted; its deployment row is stale
			}
			return nil, fmt.Errorf("failed to load anchor gateway %s: %w", gwID, err)
		}
		if gw == nil {
			continue
		}
		if requested != nil && *requested != "" && *requested != gwID {
			return nil, fmt.Errorf("%w: artifact is deployed to gateway %s in environment %s",
				errPlacementFixed, gwID, envIDStr)
		}
		return gw, nil
	}
	return resolveEgressGatewayForEnvironment(repo, ouID, envUUID, requested)
}

// managedAnchorsOnly is gatewayIDs without the self-hosted gateways, original order kept. Unknown or deleted
// ids stay: they are stale rows the placement lookup skips. Used for agent and monitor LLM proxies, which
// must never be placed next to a self-hosted anchor.
func managedAnchorsOnly(repo repositories.GatewayRepository, gatewayIDs []string) ([]string, error) {
	kept := make([]string, 0, len(gatewayIDs))
	for _, id := range gatewayIDs {
		gw, err := repo.GetByUUID(id)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to load anchor gateway %s: %w", id, err)
		}
		if err == nil && gw != nil && !gw.IsManaged() {
			continue
		}
		kept = append(kept, id)
	}
	return kept, nil
}

// resolveManagedPlacement is resolveEgressGatewayForArtifact for an AMS-owned proxy, refusing a self-hosted
// result rather than resolving again: another gateway would be one the proxy isn't on.
func resolveManagedPlacement(
	repo repositories.GatewayRepository, ouID string, envUUID uuid.UUID, placed []string,
) (*models.Gateway, error) {
	gateway, err := resolveEgressGatewayForArtifact(repo, ouID, envUUID, placed, nil)
	if err != nil {
		return nil, err
	}
	if !gateway.IsManaged() {
		return nil, fmt.Errorf("%w: gateway %s (%s) in environment %s",
			errSelfHostedAnchor, gateway.Name, gateway.UUID, envUUID)
	}
	return gateway, nil
}

// placedOn runs a kind's PlacedOn query, records whose artifacts PlacedCondition finds placed, through tx.
func placedOn(ctx context.Context, tx *gorm.DB, records string, args []any) (bool, error) {
	var placed bool
	err := tx.WithContext(ctx).Raw(`SELECT EXISTS (`+records+`)`, args...).Scan(&placed).Error
	return placed, err
}

// pageUnplacedRecords runs a kind's UnplacedRecords query: arms are UNION ALL arms selecting ou_id, artifact_uuid,
// handle, kind and environment_uuid, returned in artifact UUID order after afterKey.
func pageUnplacedRecords(
	ctx context.Context, tx *gorm.DB, arms string, args []any, afterKey string, limit int,
) ([]reconcile.Record, error) {
	var rows []struct {
		OUID            string    `gorm:"column:ou_id"`
		ArtifactUUID    uuid.UUID `gorm:"column:artifact_uuid"`
		Handle          string    `gorm:"column:handle"`
		Kind            string    `gorm:"column:kind"`
		EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid"`
	}
	query := `SELECT * FROM (` + arms + `) r WHERE r.artifact_uuid::text > ? ORDER BY r.artifact_uuid::text LIMIT ?`
	if err := tx.WithContext(ctx).Raw(query, append(args, afterKey, limit)...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	records := make([]reconcile.Record, 0, len(rows))
	for _, r := range rows {
		records = append(records, reconcile.Record{
			ArtifactRef:     models.ArtifactRef{OUID: r.OUID, ArtifactUUID: r.ArtifactUUID, Kind: r.Kind, Handle: r.Handle},
			EnvironmentUUID: r.EnvironmentUUID,
			Key:             r.ArtifactUUID.String(),
		})
	}
	return records, nil
}

// placementReposIn are the repositories bound to the transaction ctx carries, if any, so a placement made inside a
// save reads the save's rows on its connection.
func placementReposIn(
	ctx context.Context, deployments repositories.DeploymentRepository, gateways repositories.GatewayRepository,
) (repositories.DeploymentRepository, repositories.GatewayRepository) {
	tx := db.TxFrom(ctx)
	if tx == nil {
		return deployments, gateways
	}
	return deployments.WithTx(tx), gateways.WithTx(tx)
}

// gatewaysIn is gateways bound to the transaction ctx carries, if any.
func gatewaysIn(ctx context.Context, gateways repositories.GatewayRepository) repositories.GatewayRepository {
	if tx := db.TxFrom(ctx); tx != nil {
		return gateways.WithTx(tx)
	}
	return gateways
}

// validateEgressPlacement checks that the named gateway may host this artifact: it must
// be egress-capable, and it must not share an environment with an existing deployment of
// the same artifact (one gateway per (artifact, environment)).
//
// No environment is present in these requests, so the cap is enforced by resolving the
// target gateway's environments through gateway_environment_mappings and rejecting a
// gateway that shares one with an existing deployment.
func validateEgressPlacement(
	repo repositories.GatewayRepository, gateway *models.Gateway, existingDeployments []string,
) error {
	if !gateway.IsEgressCapable() {
		return fmt.Errorf("%w: gateway %s (%s) is designated for ingress only",
			errInvalidEgressGateway, gateway.Name, gateway.UUID)
	}
	if len(existingDeployments) == 0 {
		return nil
	}
	targetEnvs, err := repo.GetEnvironmentMappingsByGatewayID(gateway.UUID.String())
	if err != nil {
		return fmt.Errorf("failed to list environments for gateway %s: %w", gateway.UUID, err)
	}
	targetEnvSet := make(map[string]struct{}, len(targetEnvs))
	for _, m := range targetEnvs {
		targetEnvSet[m.EnvironmentUUID.String()] = struct{}{}
	}
	for _, existingID := range existingDeployments {
		if existingID == gateway.UUID.String() {
			continue // already deployed here: idempotent
		}
		existingEnvs, err := repo.GetEnvironmentMappingsByGatewayID(existingID)
		if err != nil {
			return fmt.Errorf("failed to list environments for gateway %s: %w", existingID, err)
		}
		for _, m := range existingEnvs {
			if _, clash := targetEnvSet[m.EnvironmentUUID.String()]; clash {
				return fmt.Errorf("%w: already deployed to gateway %s in environment %s",
					errPlacementFixed, existingID, m.EnvironmentUUID)
			}
		}
	}
	return nil
}

// describeGateways renders "name (uuid), name (uuid)" for error messages.
func describeGateways(gateways []*models.Gateway) string {
	parts := make([]string, 0, len(gateways))
	for _, gw := range gateways {
		parts = append(parts, fmt.Sprintf("%s (%s)", gw.Name, gw.UUID))
	}
	return strings.Join(parts, ", ")
}
