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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// MonitorRepository defines the interface for monitor and monitor run data access
type MonitorRepository interface {
	// Transaction support
	WithTx(tx *gorm.DB) MonitorRepository
	RunInTransaction(fn func(txRepo MonitorRepository) error) error

	// Monitor CRUD
	CreateMonitor(monitor *models.Monitor) error
	GetMonitorByName(orgName, monitorName string) (*models.Monitor, error)
	GetMonitorByID(monitorID uuid.UUID) (*models.Monitor, error)
	ListMonitorsByAgent(orgName, projectName, agentName string) ([]models.Monitor, error)
	UpdateMonitor(monitor *models.Monitor) error
	DeleteMonitor(monitor *models.Monitor) error
	UpdateNextRunTime(monitorID uuid.UUID, nextRunTime *time.Time) error
	ListDueMonitors(monitorType string, dueBy time.Time) ([]models.Monitor, error)

	// MonitorRun CRUD
	CreateMonitorRun(run *models.MonitorRun) error
	GetMonitorRunByID(runID, monitorID uuid.UUID) (*models.MonitorRun, error)
	ListMonitorRuns(monitorID uuid.UUID, limit, offset int) ([]models.MonitorRun, error)
	CountMonitorRuns(monitorID uuid.UUID) (int64, error)
	GetMonitorRunsByMonitorID(monitorID uuid.UUID) ([]models.MonitorRun, error)
	GetLatestMonitorRun(monitorID uuid.UUID) (*models.MonitorRun, error)
	GetLatestMonitorRuns(monitorIDs []uuid.UUID) (map[uuid.UUID]models.MonitorRun, error)
	UpdateMonitorRun(run *models.MonitorRun, updates map[string]interface{}) error
	ListPendingOrRunningRuns(limit int) ([]models.MonitorRun, error)
}

// MonitorRepo implements MonitorRepository using GORM
type MonitorRepo struct {
	db *gorm.DB
}

// NewMonitorRepo creates a new monitor repository
func NewMonitorRepo(db *gorm.DB) MonitorRepository {
	return &MonitorRepo{db: db}
}

// WithTx returns a new MonitorRepository backed by the given transaction
func (r *MonitorRepo) WithTx(tx *gorm.DB) MonitorRepository {
	return &MonitorRepo{db: tx}
}

// RunInTransaction executes fn within a database transaction, providing a transaction-bound repository
func (r *MonitorRepo) RunInTransaction(fn func(txRepo MonitorRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(r.WithTx(tx))
	})
}

// --- Monitor operations ---

// CreateMonitor creates a new monitor record
func (r *MonitorRepo) CreateMonitor(monitor *models.Monitor) error {
	return r.db.Create(monitor).Error
}

// GetMonitorByName retrieves a monitor by name and org
func (r *MonitorRepo) GetMonitorByName(orgName, monitorName string) (*models.Monitor, error) {
	var monitor models.Monitor
	if err := r.db.Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		return nil, err
	}
	return &monitor, nil
}

// GetMonitorByID retrieves a monitor by its ID
func (r *MonitorRepo) GetMonitorByID(monitorID uuid.UUID) (*models.Monitor, error) {
	var monitor models.Monitor
	if err := r.db.Where("id = ?", monitorID).First(&monitor).Error; err != nil {
		return nil, err
	}
	return &monitor, nil
}

// ListMonitorsByAgent lists all monitors for an org/project/agent combination
func (r *MonitorRepo) ListMonitorsByAgent(orgName, projectName, agentName string) ([]models.Monitor, error) {
	var monitors []models.Monitor
	err := r.db.Where("org_name = ? AND project_name = ? AND agent_name = ?", orgName, projectName, agentName).
		Order("created_at DESC").
		Find(&monitors).Error
	return monitors, err
}

// UpdateMonitor saves all fields of the monitor
func (r *MonitorRepo) UpdateMonitor(monitor *models.Monitor) error {
	return r.db.Save(monitor).Error
}

// DeleteMonitor deletes a monitor record
func (r *MonitorRepo) DeleteMonitor(monitor *models.Monitor) error {
	return r.db.Delete(monitor).Error
}

// UpdateNextRunTime updates the next_run_time field for a monitor
func (r *MonitorRepo) UpdateNextRunTime(monitorID uuid.UUID, nextRunTime *time.Time) error {
	return r.db.Model(&models.Monitor{}).Where("id = ?", monitorID).Update("next_run_time", nextRunTime).Error
}

// ListDueMonitors returns monitors of the given type whose next_run_time is at or before dueBy
func (r *MonitorRepo) ListDueMonitors(monitorType string, dueBy time.Time) ([]models.Monitor, error) {
	var monitors []models.Monitor
	err := r.db.Where("type = ? AND next_run_time <= ?", monitorType, dueBy).Find(&monitors).Error
	return monitors, err
}

// --- MonitorRun operations ---

// CreateMonitorRun creates a new monitor run record
func (r *MonitorRepo) CreateMonitorRun(run *models.MonitorRun) error {
	return r.db.Create(run).Error
}

// GetMonitorRunByID retrieves a monitor run by its ID and monitor ID
func (r *MonitorRepo) GetMonitorRunByID(runID, monitorID uuid.UUID) (*models.MonitorRun, error) {
	var run models.MonitorRun
	if err := r.db.Where("id = ? AND monitor_id = ?", runID, monitorID).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// ListMonitorRuns returns paginated runs for a monitor, ordered by most recent first
func (r *MonitorRepo) ListMonitorRuns(monitorID uuid.UUID, limit, offset int) ([]models.MonitorRun, error) {
	var runs []models.MonitorRun
	query := r.db.Where("monitor_id = ?", monitorID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}
	err := query.Find(&runs).Error
	return runs, err
}

// CountMonitorRuns returns the total count of runs for a monitor
func (r *MonitorRepo) CountMonitorRuns(monitorID uuid.UUID) (int64, error) {
	var total int64
	err := r.db.Model(&models.MonitorRun{}).Where("monitor_id = ?", monitorID).Count(&total).Error
	return total, err
}

// GetMonitorRunsByMonitorID returns all runs for a monitor
func (r *MonitorRepo) GetMonitorRunsByMonitorID(monitorID uuid.UUID) ([]models.MonitorRun, error) {
	var runs []models.MonitorRun
	err := r.db.Where("monitor_id = ?", monitorID).Find(&runs).Error
	return runs, err
}

// GetLatestMonitorRun returns the most recent run for a monitor
func (r *MonitorRepo) GetLatestMonitorRun(monitorID uuid.UUID) (*models.MonitorRun, error) {
	var run models.MonitorRun
	if err := r.db.Where("monitor_id = ?", monitorID).Order("created_at DESC").First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// GetLatestMonitorRuns batch-loads the latest run for each monitor in a single query.
// It fetches the most recent run per monitor using a subquery for max created_at.
func (r *MonitorRepo) GetLatestMonitorRuns(monitorIDs []uuid.UUID) (map[uuid.UUID]models.MonitorRun, error) {
	result := make(map[uuid.UUID]models.MonitorRun)
	if len(monitorIDs) == 0 {
		return result, nil
	}

	// Subquery: get the max created_at per monitor_id
	subQuery := r.db.Model(&models.MonitorRun{}).
		Select("monitor_id, MAX(created_at) AS max_created_at").
		Where("monitor_id IN ?", monitorIDs).
		Group("monitor_id")

	var runs []models.MonitorRun
	err := r.db.Where("(monitor_id, created_at) IN (?)", subQuery).Find(&runs).Error
	if err != nil {
		return nil, err
	}

	for _, run := range runs {
		result[run.MonitorID] = run
	}
	return result, nil
}

// UpdateMonitorRun updates specific fields of a monitor run
func (r *MonitorRepo) UpdateMonitorRun(run *models.MonitorRun, updates map[string]interface{}) error {
	return r.db.Model(run).Updates(updates).Error
}

// ListPendingOrRunningRuns returns runs with pending or running status
func (r *MonitorRepo) ListPendingOrRunningRuns(limit int) ([]models.MonitorRun, error) {
	var runs []models.MonitorRun
	err := r.db.Where("status IN ?", []string{models.RunStatusPending, models.RunStatusRunning}).
		Order("created_at ASC").
		Limit(limit).
		Find(&runs).Error
	return runs, err
}
