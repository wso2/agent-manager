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
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MCPCapabilityDiscoveryClaimLease is how long a claimed binding stays out of other claims: one attempt has this
// long to record its outcome before another replica may take it.
const MCPCapabilityDiscoveryClaimLease = 5 * time.Minute

// ErrMCPCapabilityDiscoverySuperseded means the binding was queued again, or its discovery dropped, after the caller
// read it.
var ErrMCPCapabilityDiscoverySuperseded = errors.New("mcp capability discovery was queued again since it was read")

// MCPCapabilityDiscoveryRepository queues and records project MCP proxy bindings' capability discoveries. The state
// lives in the binding endpoint's configuration (MCPEndpointConfig.CapabilityDiscovery); every write here changes
// only that key, so it never overwrites the binding's configuration.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/mcp_capability_discovery_repository_mock.go . MCPCapabilityDiscoveryRepository:MCPCapabilityDiscoveryRepositoryMock
type MCPCapabilityDiscoveryRepository interface {
	// WithTx returns the repository bound to tx, so its writes commit or roll back with the caller's.
	WithTx(tx *gorm.DB) MCPCapabilityDiscoveryRepository
	// Enqueue makes the binding's discovery pending and due now, with a fresh attempt budget and token. It keeps
	// discoveredAt.
	Enqueue(ctx context.Context, artifactUUID uuid.UUID) error
	// ClaimDue leases up to limit pending discoveries whose nextAttemptAt has passed.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.MCPCapabilityDiscovery, error)

	// The Mark* methods and Drop act on the discovery as ClaimDue returned it, and return
	// ErrMCPCapabilityDiscoverySuperseded when it was queued again since.
	MarkAttemptFailed(ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string, nextAttemptAt time.Time) error
	MarkFailed(ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string) error
	Drop(ctx context.Context, read models.MCPCapabilityDiscovery) error
}

type mcpCapabilityDiscoveryRepository struct {
	db *gorm.DB
}

// NewMCPCapabilityDiscoveryRepository creates an MCPCapabilityDiscoveryRepository.
func NewMCPCapabilityDiscoveryRepository(db *gorm.DB) MCPCapabilityDiscoveryRepository {
	return &mcpCapabilityDiscoveryRepository{db: db}
}

func (r *mcpCapabilityDiscoveryRepository) WithTx(tx *gorm.DB) MCPCapabilityDiscoveryRepository {
	return &mcpCapabilityDiscoveryRepository{db: tx}
}

// bindingEndpoint selects the endpoint row of the binding whose gateway artifact is the first argument.
const bindingEndpoint = `uuid = (SELECT endpoint_uuid FROM mcp_proxy_endpoint_environments WHERE artifact_uuid = ?)`

// setDiscovery replaces the binding's discovery state with state, when where (on the endpoint row) holds.
func (r *mcpCapabilityDiscoveryRepository) setDiscovery(
	ctx context.Context, artifactUUID uuid.UUID, state *models.MCPCapabilityDiscoveryState, where string, args ...any,
) (int64, error) {
	set := "configuration - 'capabilityDiscovery'"
	values := []any{}
	if state != nil {
		encoded, err := json.Marshal(state)
		if err != nil {
			return 0, fmt.Errorf("failed to encode the capability discovery: %w", err)
		}
		set = "jsonb_set(COALESCE(configuration, '{}'::jsonb), '{capabilityDiscovery}', ?::jsonb)"
		values = append(values, string(encoded))
	}
	sql := "UPDATE mcp_proxy_endpoints SET configuration = " + set + " WHERE " + bindingEndpoint
	values = append(values, artifactUUID)
	if where != "" {
		sql += " AND " + where
		values = append(values, args...)
	}
	result := r.db.WithContext(ctx).Exec(sql, values...)
	return result.RowsAffected, result.Error
}

func (r *mcpCapabilityDiscoveryRepository) Enqueue(ctx context.Context, artifactUUID uuid.UUID) error {
	var raw *string
	err := r.db.WithContext(ctx).Raw(`SELECT configuration->'capabilityDiscovery' FROM mcp_proxy_endpoints WHERE `+bindingEndpoint,
		artifactUUID).Row().Scan(&raw)
	if err != nil {
		return fmt.Errorf("failed to read the capability discovery: %w", err)
	}
	now := time.Now().UTC()
	state := &models.MCPCapabilityDiscoveryState{
		Status: models.MCPCapabilityDiscoveryPending, NextAttemptAt: &now, Token: uuid.NewString(),
	}
	if raw != nil {
		var prev models.MCPCapabilityDiscoveryState
		if json.Unmarshal([]byte(*raw), &prev) == nil {
			state.DiscoveredAt = prev.DiscoveredAt
		}
	}
	_, err = r.setDiscovery(ctx, artifactUUID, state, "")
	return err
}

// claimedDiscovery is one row ClaimDue returns.
type claimedDiscovery struct {
	ArtifactUUID uuid.UUID `gorm:"column:artifact_uuid"`
	OUID         string    `gorm:"column:ou_id"`
	State        string    `gorm:"column:state"`
}

func (r *mcpCapabilityDiscoveryRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.MCPCapabilityDiscovery, error) {
	lease, err := json.Marshal(now.Add(MCPCapabilityDiscoveryClaimLease).UTC())
	if err != nil {
		return nil, err
	}
	var rows []claimedDiscovery
	err = r.db.WithContext(ctx).Raw(
		`
		UPDATE mcp_proxy_endpoints e
		SET configuration = jsonb_set(e.configuration, '{capabilityDiscovery,nextAttemptAt}', ?::jsonb)
		FROM mcp_proxy_endpoint_environments ee, artifacts a
		WHERE ee.endpoint_uuid = e.uuid AND a.uuid = e.mcp_proxy_uuid
		AND e.uuid IN (
			SELECT d.uuid FROM mcp_proxy_endpoints d
			WHERE d.configuration->'capabilityDiscovery'->>'status' = ?
			AND (d.configuration->'capabilityDiscovery'->>'nextAttemptAt')::timestamptz <= ?
			ORDER BY (d.configuration->'capabilityDiscovery'->>'nextAttemptAt')::timestamptz ASC
			LIMIT ?
			FOR UPDATE OF d SKIP LOCKED
		)
		RETURNING ee.artifact_uuid AS artifact_uuid, a.ou_id AS ou_id, e.configuration->'capabilityDiscovery' AS state`,
		string(lease), models.MCPCapabilityDiscoveryPending, now, limit,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	claimed := make([]models.MCPCapabilityDiscovery, 0, len(rows))
	for _, row := range rows {
		d := models.MCPCapabilityDiscovery{ArtifactUUID: row.ArtifactUUID, OUID: row.OUID}
		if err := json.Unmarshal([]byte(row.State), &d.MCPCapabilityDiscoveryState); err != nil {
			return nil, fmt.Errorf("failed to decode the capability discovery of %s: %w", row.ArtifactUUID, err)
		}
		claimed = append(claimed, d)
	}
	return claimed, nil
}

// update writes state while the binding still has read's token.
func (r *mcpCapabilityDiscoveryRepository) update(
	ctx context.Context, read models.MCPCapabilityDiscovery, state *models.MCPCapabilityDiscoveryState,
) error {
	affected, err := r.setDiscovery(ctx, read.ArtifactUUID, state,
		"configuration->'capabilityDiscovery'->>'token' = ?", read.Token)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrMCPCapabilityDiscoverySuperseded
	}
	return nil
}

func (r *mcpCapabilityDiscoveryRepository) MarkAttemptFailed(
	ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string, nextAttemptAt time.Time,
) error {
	state := read.MCPCapabilityDiscoveryState
	state.AttemptCount++
	state.LastError = lastErr
	next := nextAttemptAt.UTC()
	state.NextAttemptAt = &next
	return r.update(ctx, read, &state)
}

func (r *mcpCapabilityDiscoveryRepository) MarkFailed(ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string) error {
	state := read.MCPCapabilityDiscoveryState
	state.AttemptCount++
	state.Status = models.MCPCapabilityDiscoveryFailed
	state.LastError = lastErr
	state.NextAttemptAt = nil
	return r.update(ctx, read, &state)
}

func (r *mcpCapabilityDiscoveryRepository) Drop(ctx context.Context, read models.MCPCapabilityDiscovery) error {
	return r.update(ctx, read, nil)
}
