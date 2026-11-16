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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// A gateway artifact names an upstream credential by a handle, in a {{ secret "handle" }} placeholder the gateway
// resolves when it renders the artifact. The gateway fetches the value from AMS when it first sees the handle and keeps
// it; it does not fetch a handle it already has again, and a value it refreshes reaches no deployment it has already
// rendered. So a handle names one value for good: a new credential gets a new secret and a new handle, which changes
// the artifact and makes the gateway render it again. The previous secret is retired, not released, and stays
// readable until no gateway can still be running a deployment that names it.

// secretPlaceholder matches a {{ secret "handle" }} placeholder, as the gateway does.
var secretPlaceholder = regexp.MustCompile(`\{\{\s*secret\s+\\?"([^"\\]+)\\?"\s*\}\}`)

// gatewaySecretHandlePrefix starts every handle AMS issues.
const gatewaySecretHandlePrefix = "amp-"

func newGatewaySecretHandle() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate a secret handle: %w", err)
	}
	return gatewaySecretHandlePrefix + hex.EncodeToString(b), nil
}

// putGatewaySecret stores value as a new secret of the owner and issues the handle gateway artifacts name it by.
func (s *SecretStore) putGatewaySecret(tx *gorm.DB, orgID, ownerKind string, ownerID uuid.UUID, value string) (*uuid.UUID, error) {
	id, err := s.put(tx, orgID, ownerKind, ownerID, nil, value)
	if err != nil || id == nil {
		return id, err
	}
	handle, err := newGatewaySecretHandle()
	if err != nil {
		return nil, err
	}
	if err := tx.Create(&models.GatewaySecretHandle{
		Handle: handle, OrgID: orgID, SecretID: *id, CreatedAt: dbx.Now(),
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to issue a secret handle: %w", err)
	}
	return id, nil
}

// gatewaySecretHandle is the handle gateway artifacts name the secret by; found is false for a secret stored before
// handles were issued, which artifacts carry as an encrypted secretRef instead.
func gatewaySecretHandle(tx *gorm.DB, secretID uuid.UUID) (handle string, found bool, err error) {
	var rows []models.GatewaySecretHandle
	if err := tx.Where("secret_id = ? AND retired_at IS NULL", secretID).Order("created_at DESC").Limit(1).
		Find(&rows).Error; err != nil {
		return "", false, fmt.Errorf("failed to read the handle of secret %s: %w", secretID, err)
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	return rows[0].Handle, true, nil
}

// retireSecret ends the owner's use of a secret in tx. A secret gateway artifacts name by handle stays until no gateway
// can still be running a deployment that names it; ReleaseRetiredGatewaySecrets releases it then. Any other secret is
// released now. nil is a no-op.
func retireSecret(tx *gorm.DB, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	res := tx.Model(&models.GatewaySecretHandle{}).Where("secret_id = ? AND retired_at IS NULL", *id).
		Update("retired_at", dbx.Now())
	if res.Error != nil {
		return fmt.Errorf("failed to retire secret %s: %w", *id, res.Error)
	}
	var handles int64
	if err := tx.Model(&models.GatewaySecretHandle{}).Where("secret_id = ?", *id).Count(&handles).Error; err != nil {
		return fmt.Errorf("failed to read the handles of secret %s: %w", *id, err)
	}
	if handles > 0 {
		return nil
	}
	return releaseSecret(tx, id)
}

// Placement statuses in which the gateway may still be running a deployment older than the one the row names.
var gatewayTransitionalStatuses = []models.DeploymentStatus{
	models.DeploymentStatusPending, models.DeploymentStatusDeploying, models.DeploymentStatusFailed,
	models.DeploymentStatusUndeploying,
}

// gatewayHandlesInUse returns the handles named by the deployments a gateway may be running: each placement's current
// deployment unless it is undeployed, and, while a placement is between deployments or failed, every deployment of it
// the history keeps. gatewayID empty covers every gateway of the organization.
func gatewayHandlesInUse(ctx context.Context, db *gorm.DB, orgID, gatewayID string) (map[string]bool, error) {
	db = db.WithContext(ctx)
	current := db.Table("artifact_gateway_status s").Select("d.content").
		Joins("INNER JOIN artifact_deployments d ON d.id = s.deployment_id").
		Where("s.org_id = ? AND s.status <> ?", orgID, models.DeploymentStatusUndeployed)
	history := db.Table("artifact_deployments d").Select("d.content").
		Joins("INNER JOIN artifact_gateway_status s ON s.artifact_id = d.artifact_id AND s.gateway_id = d.gateway_id").
		Where("s.org_id = ? AND s.status IN ?", orgID, gatewayTransitionalStatuses)
	if gatewayID != "" {
		current = current.Where("s.gateway_id = ?", gatewayID)
		history = history.Where("s.gateway_id = ?", gatewayID)
	}
	handles := map[string]bool{}
	for _, q := range []*gorm.DB{current, history} {
		var contents [][]byte
		if err := q.Pluck("d.content", &contents).Error; err != nil {
			return nil, fmt.Errorf("failed to read the deployments a gateway may be running: %w", err)
		}
		for _, content := range contents {
			for _, m := range secretPlaceholder.FindAllSubmatch(content, -1) {
				handles[string(m[1])] = true
			}
		}
	}
	return handles, nil
}

// GatewaySecretRepository serves the secrets a gateway's artifacts name to that gateway.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/gateway_secret_repository_mock.go . GatewaySecretRepository:GatewaySecretRepositoryMock
type GatewaySecretRepository interface {
	// ListForGateway returns the secrets that deployments the gateway may be running name, changed after updatedAfter
	// when it is set.
	ListForGateway(ctx context.Context, orgID, gatewayID string, updatedAfter *time.Time) ([]models.GatewaySecretRef, error)
	// ReadForGateway returns the stored value of a secret the gateway may read; found is false for a handle no
	// deployment of the gateway names, or one of another organization.
	ReadForGateway(ctx context.Context, orgID, gatewayID, handle string) (value []byte, found bool, err error)
}

type gatewaySecretRepo struct {
	db      *gorm.DB
	secrets *SecretStore
}

// NewGatewaySecretRepository serves gateway secrets from db through secrets.
func NewGatewaySecretRepository(db *gorm.DB, secrets *SecretStore) GatewaySecretRepository {
	return &gatewaySecretRepo{db: db, secrets: secrets}
}

func (r *gatewaySecretRepo) ListForGateway(ctx context.Context, orgID, gatewayID string, updatedAfter *time.Time) ([]models.GatewaySecretRef, error) {
	inUse, err := gatewayHandlesInUse(ctx, r.db, orgID, gatewayID)
	if err != nil {
		return nil, err
	}
	if len(inUse) == 0 {
		return []models.GatewaySecretRef{}, nil
	}
	names := make([]string, 0, len(inUse))
	for h := range inUse {
		names = append(names, h)
	}
	var refs []models.GatewaySecretRef
	for _, chunk := range dbx.Chunks(names) {
		var rows []models.GatewaySecretRef
		q := r.db.WithContext(ctx).Table("gateway_secret_handles h").
			Select("h.handle, h.secret_id, s.version, h.created_at, s.updated_at").
			Joins("INNER JOIN secrets s ON s.id = h.secret_id AND s.org_id = h.org_id").
			Where("h.org_id = ? AND h.handle IN ?", orgID, chunk)
		if updatedAfter != nil {
			q = q.Where("s.updated_at > ? OR h.created_at > ?", *updatedAfter, *updatedAfter)
		}
		if err := q.Order("h.handle").Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("failed to list the gateway's secrets: %w", err)
		}
		refs = append(refs, rows...)
	}
	if refs == nil {
		refs = []models.GatewaySecretRef{}
	}
	return refs, nil
}

func (r *gatewaySecretRepo) ReadForGateway(ctx context.Context, orgID, gatewayID, handle string) ([]byte, bool, error) {
	var rows []models.GatewaySecretHandle
	if err := r.db.WithContext(ctx).Where("handle = ? AND org_id = ?", handle, orgID).Find(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("failed to read secret handle: %w", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	inUse, err := gatewayHandlesInUse(ctx, r.db, orgID, gatewayID)
	if err != nil {
		return nil, false, err
	}
	if !inUse[handle] {
		return nil, false, nil
	}
	return r.secrets.Read(ctx, r.db, rows[0].SecretID)
}

// ReleaseRetiredGatewaySecrets releases the retired secrets no gateway can still be running a deployment of, and
// returns how many it released. A release queues the removal of an external value as any other does.
func ReleaseRetiredGatewaySecrets(ctx context.Context, db *gorm.DB) (int, error) {
	var retired []models.GatewaySecretHandle
	if err := db.WithContext(ctx).Where("retired_at IS NOT NULL").Order("org_id, retired_at").
		Find(&retired).Error; err != nil {
		return 0, fmt.Errorf("failed to list retired secret handles: %w", err)
	}
	inUseByOrg := map[string]map[string]bool{}
	released := 0
	for _, h := range retired {
		inUse, ok := inUseByOrg[h.OrgID]
		if !ok {
			var err error
			if inUse, err = gatewayHandlesInUse(ctx, db, h.OrgID, ""); err != nil {
				return released, err
			}
			inUseByOrg[h.OrgID] = inUse
		}
		if inUse[h.Handle] {
			continue
		}
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Where("handle = ? AND retired_at IS NOT NULL", h.Handle).Delete(&models.GatewaySecretHandle{})
			if res.Error != nil || res.RowsAffected == 0 {
				return res.Error
			}
			var left int64
			if err := tx.Model(&models.GatewaySecretHandle{}).Where("secret_id = ?", h.SecretID).Count(&left).Error; err != nil {
				return err
			}
			if left > 0 {
				return nil
			}
			return releaseSecret(tx, &h.SecretID)
		})
		if err != nil {
			return released, fmt.Errorf("failed to release retired secret %s: %w", h.SecretID, err)
		}
		released++
	}
	return released, nil
}

// resolveGatewayAuth fills auth for a gateway artifact from the secret the owner row refers to: a {{ secret "handle" }}
// placeholder in Value, which the gateway resolves, or, for a secret stored before handles were issued, the encrypted
// secretRef AMS resolves when the gateway fetches the artifact.
func (s *SecretStore) resolveGatewayAuth(ctx context.Context, tx *gorm.DB, secretID uuid.UUID, auth *models.UpstreamAuth) error {
	handle, found, err := gatewaySecretHandle(tx.WithContext(ctx), secretID)
	if err != nil {
		return err
	}
	if found {
		placeholder := models.GatewaySecretPlaceholder(handle)
		auth.Value, auth.SecretRef = &placeholder, nil
		return nil
	}
	value, found, err := s.Read(ctx, tx, secretID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("secret %s is missing", secretID)
	}
	ref := string(value)
	auth.SecretRef, auth.Value = &ref, nil
	return nil
}
