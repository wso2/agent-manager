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
	"fmt"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// BudgetRepository stores budgets, whoever created them: the control plane, or a
// gateway through its own API. Names are unique in the organization. Read budgets carry
// their organization's and project's handles, from the core.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/budget_repository_mock.go . BudgetRepository:BudgetRepositoryMock
type BudgetRepository interface {
	// CreateForControlPlane creates a control-plane budget named name. budget.ID is set
	// when nil, and Name and the timestamps are filled in. It returns
	// gwerr.ErrBudgetAlreadyExists when another budget holds the name.
	CreateForControlPlane(ctx context.Context, budget *gwmodels.Budget, name string) error
	// CreateManyForControlPlane creates control-plane budgets and updates others in one
	// transaction, all or none: budgets[i] is named names[i]; updates replace stored
	// budgets' configurations, as Update does. When a name is held by another budget
	// nothing is saved and the error is a *BudgetNameTakenError naming it.
	CreateManyForControlPlane(ctx context.Context, budgets []*gwmodels.Budget, names []string, updates []*gwmodels.Budget) error
	// TakenNames returns which of names another budget of the organization holds.
	TakenNames(ctx context.Context, ouID string, names []string) ([]string, error)
	// SaveForGateway stores a budget a gateway pushed, matched by name: it updates the
	// gateway's earlier copy, keeping its ID and the fields Agent Manager owns (display
	// name, description, warnings), or creates it. budget.ID is set to the stored budget's
	// ID. It reports false, storing nothing, when the name is taken by another budget (a
	// control-plane budget or another gateway's).
	SaveForGateway(ctx context.Context, budget *gwmodels.Budget, name string) (bool, error)
	// DeleteForGateway deletes the gateway's budgets with the given names.
	DeleteForGateway(ctx context.Context, ouID string, gatewayID uuid.UUID, names []string) error
	// DeleteForGatewayExcept deletes the gateway's budgets whose name is not in keep.
	DeleteForGatewayExcept(ctx context.Context, ouID string, gatewayID uuid.UUID, keep []string) error
	// Get returns the organization's budget named name, or gwerr.ErrBudgetNotFound.
	Get(ctx context.Context, ouID, name string) (*gwmodels.Budget, error)
	// Update replaces a budget's configuration and touches its updated_at.
	Update(ctx context.Context, budget *gwmodels.Budget) error
	// Delete deletes the organization's budget named name, or returns
	// gwerr.ErrBudgetNotFound.
	Delete(ctx context.Context, ouID, name string) error
	// List returns the organization's budgets of every origin, ordered by name.
	List(ctx context.Context, ouID string) ([]gwmodels.Budget, error)
}

// BudgetOwners looks up the handles of the organization and project that own a budget:
// the core's, which the module cannot join.
type BudgetOwners interface {
	GetOrg(ctx context.Context, orgID string) (modkit.Org, error)
	ListProjects(ctx context.Context, orgID string) ([]modkit.Project, error)
}

type budgetRepository struct {
	db     *gorm.DB
	owners BudgetOwners
}

// NewBudgetRepository creates a BudgetRepository. owners, when not nil, fills in the
// owners' handles of the budgets it reads.
func NewBudgetRepository(db *gorm.DB, owners BudgetOwners) BudgetRepository {
	return &budgetRepository{db: db, owners: owners}
}

// BudgetNameTakenError is a budget name another budget of the organization holds.
type BudgetNameTakenError struct{ Name string }

func (e *BudgetNameTakenError) Error() string { return fmt.Sprintf("budget name %q is taken", e.Name) }

// Unwrap makes it gwerr.ErrBudgetAlreadyExists.
func (e *BudgetNameTakenError) Unwrap() error { return gwerr.ErrBudgetAlreadyExists }

func (r *budgetRepository) CreateForControlPlane(ctx context.Context, budget *gwmodels.Budget, name string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return createControlPlaneBudget(tx, budget, name)
	})
}

func (r *budgetRepository) CreateManyForControlPlane(ctx context.Context, budgets []*gwmodels.Budget, names []string, updates []*gwmodels.Budget) error {
	if len(budgets) != len(names) {
		return fmt.Errorf("got %d budgets and %d names", len(budgets), len(names))
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, b := range budgets {
			if err := createControlPlaneBudget(tx, b, names[i]); err != nil {
				return err
			}
		}
		for _, b := range updates {
			if err := updateBudget(tx, b); err != nil {
				return err
			}
		}
		return nil
	})
}

// createControlPlaneBudget creates a control-plane budget in tx.
func createControlPlaneBudget(tx *gorm.DB, budget *gwmodels.Budget, name string) error {
	if budget.ID == uuid.Nil {
		budget.ID = uuid.Must(uuid.NewV7())
	}
	now := dbx.Now()
	budget.Name = name // the stored kind is named after it
	budget.CreatedAt, budget.UpdatedAt = now, now
	inserted, err := dbx.InsertIfAbsent(tx, budget)
	if err != nil {
		return fmt.Errorf("failed to create budget %q: %w", name, err)
	}
	if !inserted {
		return &BudgetNameTakenError{Name: name}
	}
	return nil
}

func (r *budgetRepository) TakenNames(ctx context.Context, ouID string, names []string) ([]string, error) {
	taken := []string{}
	for _, chunk := range dbx.Chunks(names) {
		var found []string
		if err := r.db.WithContext(ctx).Model(&gwmodels.Budget{}).
			Where("org_id = ? AND name IN ?", ouID, chunk).Pluck("name", &found).Error; err != nil {
			return nil, fmt.Errorf("failed to check budget names: %w", err)
		}
		taken = append(taken, found...)
	}
	return taken, nil
}

func (r *budgetRepository) SaveForGateway(ctx context.Context, budget *gwmodels.Budget, name string) (bool, error) {
	stored := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stored = false
		if budget.ID == uuid.Nil {
			budget.ID = uuid.Must(uuid.NewV7())
		}
		now := dbx.Now()
		budget.Name = name // the stored kind is named after it
		budget.CreatedAt, budget.UpdatedAt = now, now
		inserted, err := dbx.InsertIfAbsent(tx, budget)
		if err != nil {
			return fmt.Errorf("failed to create budget %q: %w", name, err)
		}
		if inserted {
			stored = true
			return nil
		}

		// The name is taken: update it only if it is this gateway's budget. A control-plane
		// budget or another gateway's holding the name refuses it.
		var existing gwmodels.Budget
		err = tx.Where("org_id = ? AND name = ?", budget.OrgID, name).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read budget %q: %w", name, err)
		}
		if existing.Origin != gwmodels.BudgetOriginGateway || existing.GatewayID == nil || budget.GatewayID == nil || *existing.GatewayID != *budget.GatewayID {
			return nil
		}
		// The gateway owns what it enforces; Agent Manager owns what gateways never get,
		// so its display name, description and warnings are kept.
		budget.Configuration.DisplayName = existing.Configuration.DisplayName
		budget.Configuration.Description = existing.Configuration.Description
		budget.Configuration.Warnings = existing.Configuration.Warnings
		if err := tx.Model(&gwmodels.Budget{}).Where("id = ? AND org_id = ?", existing.ID, budget.OrgID).
			Updates(map[string]any{
				"configuration": dbx.JSON(gwmodels.NewBudgetDocument(name, budget.ProjectName, budget.Configuration)),
				"project_id":    budget.ProjectID,
				"updated_at":    now,
			}).Error; err != nil {
			return fmt.Errorf("failed to update budget %q: %w", name, err)
		}
		budget.ID, budget.CreatedAt, stored = existing.ID, existing.CreatedAt, true
		return nil
	})
	return stored, err
}

func (r *budgetRepository) DeleteForGateway(ctx context.Context, ouID string, gatewayID uuid.UUID, names []string) error {
	if len(names) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, chunk := range dbx.Chunks(names) {
			if err := tx.Where("org_id = ? AND origin = ? AND gateway_id = ? AND name IN ?",
				ouID, gwmodels.BudgetOriginGateway, gatewayID, chunk).Delete(&gwmodels.Budget{}).Error; err != nil {
				return fmt.Errorf("failed to delete gateway budgets: %w", err)
			}
		}
		return nil
	})
}

func (r *budgetRepository) DeleteForGatewayExcept(ctx context.Context, ouID string, gatewayID uuid.UUID, keep []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Found here and deleted by ID, since a NOT IN list cannot be split into the
		// database's IN-list limit.
		var current []struct {
			ID   uuid.UUID
			Name string
		}
		if err := tx.Model(&gwmodels.Budget{}).Select("id, name").
			Where("org_id = ? AND origin = ? AND gateway_id = ?", ouID, gwmodels.BudgetOriginGateway, gatewayID).
			Scan(&current).Error; err != nil {
			return fmt.Errorf("failed to list gateway budgets: %w", err)
		}
		kept := make(map[string]bool, len(keep))
		for _, name := range keep {
			kept[name] = true
		}
		stale := make([]uuid.UUID, 0, len(current))
		for _, c := range current {
			if !kept[c.Name] {
				stale = append(stale, c.ID)
			}
		}
		for _, chunk := range dbx.Chunks(stale) {
			if err := tx.Where("org_id = ? AND id IN ?", ouID, chunk).Delete(&gwmodels.Budget{}).Error; err != nil {
				return fmt.Errorf("failed to delete gateway budgets: %w", err)
			}
		}
		return nil
	})
}

// BudgetsUsing returns the names of the organization's budgets for which uses is true, in
// name order: what refuses deleting a provider they use. An organization
// has at most a few thousand budgets, so they are checked in code rather than with
// database-specific JSON queries.
func BudgetsUsing(tx *gorm.DB, ouID string, uses func(gwmodels.BudgetConfig) bool) ([]string, error) {
	var budgets []gwmodels.Budget
	if err := tx.Where("org_id = ?", ouID).Order("name").Find(&budgets).Error; err != nil {
		return nil, fmt.Errorf("failed to read budgets: %w", err)
	}
	names := []string{}
	for _, b := range budgets {
		if uses(b.Configuration) {
			names = append(names, b.Name)
		}
	}
	return names, nil
}

// ProjectBudgets returns the names of the control-plane budgets the project owns, and
// hands the gateway budgets it owns to the organization, in tx: what deleting the
// project does (a gateway owns its budgets; a control-plane one must be deleted first,
// so its gateways are told).
func ProjectBudgets(tx *gorm.DB, ouID, projectID string) ([]string, error) {
	var names []string
	if err := tx.Model(&gwmodels.Budget{}).
		Where("org_id = ? AND project_id = ? AND origin = ?", ouID, projectID, gwmodels.BudgetOriginControlPlane).
		Order("name").Pluck("name", &names).Error; err != nil {
		return nil, fmt.Errorf("failed to read the project's budgets: %w", err)
	}
	if len(names) > 0 {
		return names, nil
	}
	if err := tx.Model(&gwmodels.Budget{}).Where("org_id = ? AND project_id = ?", ouID, projectID).
		Update("project_id", nil).Error; err != nil {
		return nil, fmt.Errorf("failed to release the project's gateway budgets: %w", err)
	}
	return nil, nil
}

// ChannelBudgets returns the names of the organization's budgets that send warnings to
// the channel, sorted: budgets name their channels, so one they name keeps its name and
// is not deleted.
func ChannelBudgets(db *gorm.DB, ouID, channel string) ([]string, error) {
	var budgets []gwmodels.Budget
	if err := db.Where("org_id = ?", ouID).Order("name").Find(&budgets).Error; err != nil {
		return nil, fmt.Errorf("failed to read the organization's budgets: %w", err)
	}
	var names []string
	for i := range budgets {
		if w := budgets[i].Configuration.Warnings; w != nil && slices.Contains(w.Channels, channel) {
			names = append(names, budgets[i].Name)
		}
	}
	return names, nil
}

// withOwners fills in the budgets' organization and project handles.
func (r *budgetRepository) withOwners(ctx context.Context, ouID string, budgets []gwmodels.Budget) error {
	if r.owners == nil || len(budgets) == 0 {
		return nil
	}
	org, err := r.owners.GetOrg(ctx, ouID)
	if err != nil && !errors.Is(err, modkit.ErrNotFound) {
		return fmt.Errorf("failed to get organization: %w", err)
	}
	projects := map[string]string{}
	for _, b := range budgets {
		if b.ProjectID != nil {
			list, err := r.owners.ListProjects(ctx, ouID)
			if err != nil {
				return fmt.Errorf("failed to list projects: %w", err)
			}
			for _, p := range list {
				projects[p.ID] = p.Name
			}
			break
		}
	}
	for i := range budgets {
		budgets[i].OrgName = org.Handle
		if id := budgets[i].ProjectID; id != nil {
			budgets[i].ProjectName = projects[id.String()]
		}
	}
	return nil
}

func (r *budgetRepository) Get(ctx context.Context, ouID, name string) (*gwmodels.Budget, error) {
	var b gwmodels.Budget
	err := r.db.WithContext(ctx).Where("org_id = ? AND name = ?", ouID, name).Take(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gwerr.ErrBudgetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get budget %q: %w", name, err)
	}
	one := []gwmodels.Budget{b}
	if err := r.withOwners(ctx, ouID, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (r *budgetRepository) Update(ctx context.Context, budget *gwmodels.Budget) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return updateBudget(tx, budget)
	})
}

// updateBudget replaces a budget's configuration and touches its updated_at in tx.
func updateBudget(tx *gorm.DB, budget *gwmodels.Budget) error {
	now := dbx.Now()
	res := tx.Model(&gwmodels.Budget{}).Where("id = ? AND org_id = ?", budget.ID, budget.OrgID).
		Updates(map[string]any{
			"configuration": dbx.JSON(gwmodels.NewBudgetDocument(budget.Name, budget.ProjectName, budget.Configuration)),
			"updated_at":    now,
		})
	if res.Error != nil {
		return fmt.Errorf("failed to update budget %q: %w", budget.Name, res.Error)
	}
	if res.RowsAffected == 0 {
		return gwerr.ErrBudgetNotFound
	}
	budget.UpdatedAt = now
	return nil
}

func (r *budgetRepository) Delete(ctx context.Context, ouID, name string) error {
	res := r.db.WithContext(ctx).Where("org_id = ? AND name = ?", ouID, name).Delete(&gwmodels.Budget{})
	if res.Error != nil {
		return fmt.Errorf("failed to delete budget %q: %w", name, res.Error)
	}
	if res.RowsAffected == 0 {
		return gwerr.ErrBudgetNotFound
	}
	return nil
}

func (r *budgetRepository) List(ctx context.Context, ouID string) ([]gwmodels.Budget, error) {
	rows := []gwmodels.Budget{}
	if err := r.db.WithContext(ctx).Where("org_id = ?", ouID).Order("name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list budgets: %w", err)
	}
	if err := r.withOwners(ctx, ouID, rows); err != nil {
		return nil, err
	}
	return rows, nil
}
