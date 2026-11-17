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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MonitorConsumer holds the proxy and monitor names needed to build a consumer list item.
type MonitorConsumer struct {
	ProxyHandle string
	ProxyName   string
	ProjectName string
	MonitorName string
}

// MonitorLLMMappingRepository defines data access for monitor-to-LLM-proxy mappings
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/monitor_llm_mapping_repository_mock.go . MonitorLLMMappingRepository:MonitorLLMMappingRepositoryMock
type MonitorLLMMappingRepository interface {
	// Create creates a new monitor LLM mapping (use within transaction)
	Create(ctx context.Context, tx *gorm.DB, mapping *models.MonitorLLMMapping) error

	// Update persists changes to an existing mapping row (use within transaction)
	Update(ctx context.Context, tx *gorm.DB, mapping *models.MonitorLLMMapping) error

	// ListByMonitorID retrieves all mappings for a monitor (with LLMProxy preloaded)
	ListByMonitorID(ctx context.Context, monitorID uuid.UUID) ([]models.MonitorLLMMapping, error)

	// ListMonitorConsumersByProxyUUIDs returns distinct monitor consumers for the given proxy UUIDs.
	ListMonitorConsumersByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]MonitorConsumer, error)

	// DeleteByMonitorID deletes all mappings for a monitor (use within transaction)
	DeleteByMonitorID(ctx context.Context, tx *gorm.DB, monitorID uuid.UUID) error

	// DeleteByMonitorIDAndProxyUUID deletes a specific mapping (use within transaction)
	DeleteByMonitorIDAndProxyUUID(ctx context.Context, tx *gorm.DB, monitorID, proxyUUID uuid.UUID) error
}

type monitorLLMMappingRepository struct {
	db *gorm.DB
}

// NewMonitorLLMMappingRepository creates a new repository
func NewMonitorLLMMappingRepository(db *gorm.DB) MonitorLLMMappingRepository {
	return &monitorLLMMappingRepository{db: db}
}

func (r *monitorLLMMappingRepository) Create(ctx context.Context, tx *gorm.DB, mapping *models.MonitorLLMMapping) error {
	return tx.WithContext(ctx).Create(mapping).Error
}

func (r *monitorLLMMappingRepository) Update(ctx context.Context, tx *gorm.DB, mapping *models.MonitorLLMMapping) error {
	return tx.WithContext(ctx).Save(mapping).Error
}

func (r *monitorLLMMappingRepository) ListByMonitorID(ctx context.Context, monitorID uuid.UUID) ([]models.MonitorLLMMapping, error) {
	var mappings []models.MonitorLLMMapping
	err := r.db.WithContext(ctx).
		Preload("LLMProxy").
		Where("monitor_id = ?", monitorID).
		Find(&mappings).Error
	return mappings, err
}

func (r *monitorLLMMappingRepository) DeleteByMonitorID(ctx context.Context, tx *gorm.DB, monitorID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("monitor_id = ?", monitorID).
		Delete(&models.MonitorLLMMapping{}).Error
}

func (r *monitorLLMMappingRepository) DeleteByMonitorIDAndProxyUUID(ctx context.Context, tx *gorm.DB, monitorID, proxyUUID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("monitor_id = ? AND llm_proxy_uuid = ?", monitorID, proxyUUID).
		Delete(&models.MonitorLLMMapping{}).Error
}

func (r *monitorLLMMappingRepository) ListMonitorConsumersByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]MonitorConsumer, error) {
	if len(proxyUUIDs) == 0 {
		return nil, nil
	}
	var results []MonitorConsumer
	err := r.db.WithContext(ctx).
		Table("monitor_llm_mapping mlm").
		Select("DISTINCT a.handle AS proxy_handle, a.name AS proxy_name, m.project_name, m.name AS monitor_name").
		Joins("JOIN artifacts a ON a.uuid = mlm.llm_proxy_uuid").
		Joins("JOIN monitors m ON m.id = mlm.monitor_id").
		Where("mlm.llm_proxy_uuid IN ?", proxyUUIDs).
		Scan(&results).Error
	return results, err
}
