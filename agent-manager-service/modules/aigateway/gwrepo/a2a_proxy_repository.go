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

package gwrepo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsecret"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// ErrA2AProxyNotFound means the agent's environment is not proxied.
var ErrA2AProxyNotFound = errors.New("a2a proxy not found")

// A2AProxyRepository stores which external agent environments are published through their gateway.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/a2a_proxy_repository_mock.go . A2AProxyRepository:A2AProxyRepositoryMock
type A2AProxyRepository interface {
	// WithTx returns the repository bound to tx, so its writes commit or roll back with the caller's.
	WithTx(tx *gorm.DB) A2AProxyRepository

	// LockAgentEnv serialises the environment's proxy transitions until the transaction ends; call it
	// first, on WithTx, so every transition takes its row locks in the same order.
	LockAgentEnv(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error

	// Get looks the row up by the environment name in the URL path.
	Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*gwmodels.A2AProxy, error)
	// GetByEnvUUID looks the row up by environment UUID.
	GetByEnvUUID(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*gwmodels.A2AProxy, error)
	// GetByArtifact returns the proxy of the agent's per-environment artifact, joined on its handle
	// project/agent/<environment UUID>.
	GetByArtifact(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*gwmodels.A2AProxy, error)

	// Upsert writes the row, replacing a same-named row from a deleted environment.
	Upsert(ctx context.Context, proxy *gwmodels.A2AProxy) error

	// Delete returns ErrA2AProxyNotFound when the environment was not proxied.
	Delete(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error
	// DeleteForAgent removes every environment's row for a deleted agent.
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
	// DeleteForEnvironment removes every agent's row in a deleted environment.
	DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error
	// ListEnvironmentUUIDs returns the distinct environment UUIDs with rows recorded under environmentName.
	ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error)
	// ListForEnvironment returns every agent's proxy in the environment, ordered by project and agent name.
	ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]gwmodels.A2AProxy, error)
}

type a2aProxyRepository struct {
	db      *gorm.DB
	secrets modkit.Secrets
}

// NewA2AProxyRepository creates an A2AProxyRepository; secrets keeps the proxies' upstream credentials.
func NewA2AProxyRepository(db *gorm.DB, secrets modkit.Secrets) A2AProxyRepository {
	return &a2aProxyRepository{db: db, secrets: secrets}
}

func (r *a2aProxyRepository) WithTx(tx *gorm.DB) A2AProxyRepository {
	return &a2aProxyRepository{db: tx, secrets: r.secrets}
}

// withSecretRef points a read row at its stored credential: a row has one exactly while it has a header.
func withSecretRef(row *gwmodels.A2AProxy) {
	if row.UpstreamAuthHeader != "" {
		row.UpstreamAuthSecret = gwsecret.A2AProxy(row.ID)
	}
}

// LockAgentEnv locks the agent's row: Lite has one environment per agent, so the agent row serialises its proxy.
func (r *a2aProxyRepository) LockAgentEnv(ctx context.Context, ouID, projectName, agentName string, _ uuid.UUID) error {
	_, err := repositories.LockAgent(ctx, r.db, ouID, projectName, agentName)
	return err
}

func (r *a2aProxyRepository) Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*gwmodels.A2AProxy, error) {
	cond, args := repositories.AgentEnvCond(r.db, ouID, projectName, agentName, environmentName)
	return r.first(ctx, cond, args...)
}

func (r *a2aProxyRepository) GetByEnvUUID(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*gwmodels.A2AProxy, error) {
	cond, args := agentEnvIDCond(r.db, ouID, projectName, agentName, environmentUUID)
	return r.first(ctx, cond, args...)
}

// A2AProxyArtifactJoin joins a2a_proxies p to the agent's per-environment artifact a.
const A2AProxyArtifactJoin = `JOIN mod_ai_gateway_artifacts a ON a.id = p.artifact_id`

func (r *a2aProxyRepository) GetByArtifact(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*gwmodels.A2AProxy, error) {
	return r.first(ctx, "org_id = ? AND artifact_id = ?", ouID, artifactUUID)
}

// first reads one proxy with its upstream secret.
func (r *a2aProxyRepository) first(ctx context.Context, query string, args ...interface{}) (*gwmodels.A2AProxy, error) {
	var row gwmodels.A2AProxy
	db := r.db.WithContext(ctx)
	err := db.Where(query, args...).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrA2AProxyNotFound
	}
	if err != nil {
		return nil, err
	}
	withSecretRef(&row)
	return &row, nil
}

func (r *a2aProxyRepository) Upsert(ctx context.Context, proxy *gwmodels.A2AProxy) error {
	now := dbx.Now()
	proxy.UpdatedAt = now
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if proxy.AgentUUID == nil || *proxy.AgentUUID == uuid.Nil {
			id, err := gwmodels.AgentIDByNames(tx, proxy.OUID, proxy.ProjectName, proxy.AgentName)
			if err != nil {
				return err
			}
			proxy.AgentUUID = &id
		}
		var existing []gwmodels.A2AProxy
		if err := tx.Where("agent_id = ? AND environment_id = ?", *proxy.AgentUUID, proxy.EnvironmentUUID).
			Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) == 0 {
			if proxy.ID == uuid.Nil {
				proxy.ID = repositories.NewID()
			}
			proxy.CreatedAt = now
			if err := r.storeSecret(ctx, tx, proxy); err != nil {
				return err
			}
			return tx.Create(proxy).Error
		}
		current := existing[0]
		proxy.ID, proxy.CreatedAt = current.ID, current.CreatedAt
		if err := r.storeSecret(ctx, tx, proxy); err != nil {
			return err
		}
		return tx.Model(&gwmodels.A2AProxy{}).Where("id = ?", proxy.ID).Updates(map[string]any{
			"artifact_id": proxy.ArtifactUUID, "upstream_auth_header": proxy.UpstreamAuthHeader, "updated_at": now,
		}).Error
	})
}

// storeSecret writes the proxy's new credential, or deletes the stored one when there is no header to send it in.
// Without a new value the stored one stays.
func (r *a2aProxyRepository) storeSecret(ctx context.Context, tx *gorm.DB, proxy *gwmodels.A2AProxy) error {
	secrets, path := r.secrets.In(tx), gwsecret.A2AProxy(proxy.ID)
	switch {
	case proxy.UpstreamAuthHeader == "":
		proxy.UpstreamAuthSecret = ""
		return secrets.Delete(ctx, proxy.OUID, path)
	case proxy.UpstreamAuthValue != nil:
		if err := secrets.Put(ctx, proxy.OUID, path, *proxy.UpstreamAuthValue); err != nil {
			return err
		}
		proxy.UpstreamAuthValue = nil
	}
	proxy.UpstreamAuthSecret = path
	return nil
}

// DeleteA2AProxySecrets deletes, in tx, the stored credentials of the A2A proxies matching cond, before their rows
// go.
func DeleteA2AProxySecrets(ctx context.Context, tx *gorm.DB, secrets modkit.Secrets, cond string, args ...any) error {
	var rows []gwmodels.A2AProxy
	if err := tx.Session(&gorm.Session{SkipHooks: true}).Select("id, org_id").Where(cond, args...).
		Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := secrets.In(tx).Delete(ctx, row.OUID, gwsecret.A2AProxy(row.ID)); err != nil {
			return err
		}
	}
	return nil
}

// deleteProxies removes the proxies matching cond and their secrets, and reports how many it removed.
func (r *a2aProxyRepository) deleteProxies(ctx context.Context, cond string, args ...any) (int, error) {
	var rows []gwmodels.A2AProxy
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Select("id, org_id").Where(cond, args...).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Where("id = ?", row.ID).Delete(&gwmodels.A2AProxy{}).Error; err != nil {
				return err
			}
			if err := r.secrets.In(tx).Delete(ctx, row.OUID, gwsecret.A2AProxy(row.ID)); err != nil {
				return err
			}
		}
		return nil
	})
	return len(rows), err
}

func (r *a2aProxyRepository) Delete(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error {
	cond, args := agentEnvIDCond(r.db, ouID, projectName, agentName, environmentUUID)
	n, err := r.deleteProxies(ctx, cond, args...)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrA2AProxyNotFound
	}
	return nil
}

func (r *a2aProxyRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	cond, args := repositories.AgentCond(r.db, ouID, projectName, agentName)
	_, err := r.deleteProxies(ctx, cond, args...)
	return err
}

func (r *a2aProxyRepository) DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error {
	_, err := r.deleteProxies(ctx, "org_id = ? AND environment_id = ?", ouID, environmentUUID)
	return err
}

func (r *a2aProxyRepository) ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]gwmodels.A2AProxy, error) {
	rows, err := listForEnvironment[gwmodels.A2AProxy](ctx, r.db, gwmodels.A2AProxy{}.TableName(), ouID, environmentUUID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		withSecretRef(&rows[i])
	}
	return rows, nil
}

func (r *a2aProxyRepository) ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error) {
	return listEnvironmentUUIDs[gwmodels.A2AProxy](ctx, r.db, ouID, environmentName)
}
