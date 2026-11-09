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
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MCPCapabilityDiscoveryClaimLease is how long a claimed row stays out of other claims: one attempt has this long to
// record its outcome before another replica may take the row.
const MCPCapabilityDiscoveryClaimLease = 5 * time.Minute

// ErrMCPCapabilityDiscoverySuperseded means the row was queued again after the caller read it.
var ErrMCPCapabilityDiscoverySuperseded = errors.New("mcp capability discovery was queued again since it was read")

// ErrMCPCapabilityDiscoveryNotFound means the binding has no discovery row.
var ErrMCPCapabilityDiscoveryNotFound = errors.New("mcp capability discovery not found")

// MCPCapabilityDiscoveryRepository queues and records project MCP proxy bindings' capability discoveries.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/mcp_capability_discovery_repository_mock.go . MCPCapabilityDiscoveryRepository:MCPCapabilityDiscoveryRepositoryMock
type MCPCapabilityDiscoveryRepository interface {
	// WithTx returns the repository bound to tx, so its writes commit or roll back with the caller's.
	WithTx(tx *gorm.DB) MCPCapabilityDiscoveryRepository
	// Enqueue makes the binding's row pending and due now, with a fresh attempt budget. It keeps discovered_at.
	Enqueue(ctx context.Context, ouID string, artifactUUID uuid.UUID) error
	// ClaimDue leases up to limit pending rows whose next_attempt_at has passed.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.MCPCapabilityDiscovery, error)

	// The Mark* methods and DeleteIfUnchanged act on the row as ClaimDue returned it, and return
	// ErrMCPCapabilityDiscoverySuperseded when it was queued again since.
	MarkDiscovered(ctx context.Context, read models.MCPCapabilityDiscovery, at time.Time) error
	MarkAttemptFailed(ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string, nextAttemptAt time.Time) error
	MarkFailed(ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string) error
	DeleteIfUnchanged(ctx context.Context, read models.MCPCapabilityDiscovery) error

	// Get returns ErrMCPCapabilityDiscoveryNotFound when the binding has no row.
	Get(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*models.MCPCapabilityDiscovery, error)
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

// discoveryNow is a timestamp as Postgres stores it, so a row read back compares equal on updated_at.
func discoveryNow() time.Time {
	return time.Now().Truncate(time.Microsecond)
}

func (r *mcpCapabilityDiscoveryRepository) Enqueue(ctx context.Context, ouID string, artifactUUID uuid.UUID) error {
	now := discoveryNow()
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO mcp_capability_discoveries (artifact_uuid, ou_id, status, attempt_count, next_attempt_at, last_error,
			created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, '', ?, ?)
		ON CONFLICT (artifact_uuid) DO UPDATE SET status = EXCLUDED.status, attempt_count = 0,
			next_attempt_at = EXCLUDED.next_attempt_at, last_error = '', updated_at = EXCLUDED.updated_at`,
		artifactUUID, ouID, models.MCPCapabilityDiscoveryPending, now, now, now).Error
}

func (r *mcpCapabilityDiscoveryRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.MCPCapabilityDiscovery, error) {
	var claimed []models.MCPCapabilityDiscovery
	err := r.db.WithContext(ctx).Raw(
		`
		UPDATE mcp_capability_discoveries SET next_attempt_at = ?
		WHERE artifact_uuid IN (
			SELECT artifact_uuid FROM mcp_capability_discoveries
			WHERE status = ? AND next_attempt_at <= ?
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING artifact_uuid, ou_id, status, attempt_count, next_attempt_at, last_error, discovered_at, created_at,
			updated_at`,
		now.Add(MCPCapabilityDiscoveryClaimLease), models.MCPCapabilityDiscoveryPending, now, limit,
	).Scan(&claimed).Error
	return claimed, err
}

func (r *mcpCapabilityDiscoveryRepository) MarkDiscovered(ctx context.Context, read models.MCPCapabilityDiscovery, at time.Time) error {
	return r.updateIfUnchanged(ctx, read, map[string]any{
		"status":          models.MCPCapabilityDiscoveryDiscovered,
		"next_attempt_at": nil,
		"last_error":      "",
		"discovered_at":   at,
	})
}

func (r *mcpCapabilityDiscoveryRepository) MarkAttemptFailed(
	ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string, nextAttemptAt time.Time,
) error {
	return r.updateIfUnchanged(ctx, read, map[string]any{
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nextAttemptAt,
	})
}

func (r *mcpCapabilityDiscoveryRepository) MarkFailed(ctx context.Context, read models.MCPCapabilityDiscovery, lastErr string) error {
	return r.updateIfUnchanged(ctx, read, map[string]any{
		"status":          models.MCPCapabilityDiscoveryFailed,
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nil,
	})
}

func (r *mcpCapabilityDiscoveryRepository) DeleteIfUnchanged(ctx context.Context, read models.MCPCapabilityDiscovery) error {
	result := r.db.WithContext(ctx).
		Where("artifact_uuid = ? AND updated_at = ?", read.ArtifactUUID, read.UpdatedAt).
		Delete(&models.MCPCapabilityDiscovery{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMCPCapabilityDiscoverySuperseded
	}
	return nil
}

func (r *mcpCapabilityDiscoveryRepository) updateIfUnchanged(
	ctx context.Context, read models.MCPCapabilityDiscovery, updates map[string]any,
) error {
	updates["updated_at"] = discoveryNow()
	result := r.db.WithContext(ctx).Model(&models.MCPCapabilityDiscovery{}).
		Where("artifact_uuid = ? AND updated_at = ?", read.ArtifactUUID, read.UpdatedAt).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMCPCapabilityDiscoverySuperseded
	}
	return nil
}

func (r *mcpCapabilityDiscoveryRepository) Get(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*models.MCPCapabilityDiscovery, error) {
	var row models.MCPCapabilityDiscovery
	err := r.db.WithContext(ctx).Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, ouID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPCapabilityDiscoveryNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
