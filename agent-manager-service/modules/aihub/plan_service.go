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

package aihub

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dberr"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// Plan statuses. Only an ACTIVE plan that isn't deprecated can be offered or subscribed to.
const (
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
)

// OriginControlPlane is the origin of a plan made here. A plan copied in from a developer portal has the portal's
// name as its origin (the webhook_source_name setting).
const OriginControlPlane = "ai-control-plane"

// Limits on a plan's fields.
const (
	maxHandleChars      = 40
	maxDescriptionChars = 1023
)

// handlePattern keeps a handle safe in a URL path.
var handlePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// plan is a stored subscription plan.
type plan struct {
	UUID        string
	OrgID       string
	Handle      string
	Name        string
	Description string
	Status      string
	ExpiryTime  *time.Time
	Conditions  []Condition
	CreatedBy   string
	UpdatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// SourceChangedAt is when the plan last changed at the developer portal it was copied from, by the portal's
	// clock; nil for a plan made here.
	SourceChangedAt *time.Time
	// Origin is where the plan was made: OriginControlPlane, or the developer portal that copied it in.
	Origin string
	// DeprecatedAt is when the plan was deprecated; nil while it is current.
	DeprecatedAt *time.Time
}

// deprecated reports whether the plan is deprecated: kept for the subscriptions on it, but closed to changes,
// offers and new subscriptions.
func (p *plan) deprecated() bool { return p.DeprecatedAt != nil }

// planUpdate is a change to a plan. A nil field keeps the stored value.
type planUpdate struct {
	Name            *string
	Description     *string
	Conditions      *[]Condition
	RequestLimit    *Condition // from the old "limits" field: replaces the request-count condition
	ClearLimit      bool       // "limits": [] removes the request-count condition
	ExpiryTime      *time.Time
	ClearExpiryTime bool // "expiryTime": null
	Status          *string
	// FromWebhook marks a change that came in by webhook from where the plan was made: the rule
	// that a plan in use keeps its name, conditions and expiry does not apply to the source's own changes.
	FromWebhook bool
	// SourceChangedAt is when the change was made at the source (FromWebhook): a change older than the
	// plan's last one there is out of date and is not applied (errStaleEvent).
	SourceChangedAt *time.Time
}

// usage is what offers or uses a plan. A plan in use (one with subscriptions; later also one published to a
// developer portal) keeps its name, conditions and expiry. Offering a plan on an MCP proxy doesn't lock it.
type usage struct {
	MCPProxies    int
	Subscriptions int
}

func (u usage) inUse() bool { return u.Subscriptions > 0 }

// Errors the API maps to a status and code. A validationError is a 400.
var (
	errPlanNotFound = errors.New("the specified subscription plan could not be found")
	errPlanExists   = errors.New("a subscription plan with this name already exists")
	// errPlanDeprecated refuses changing a deprecated plan, offering it or subscribing to it.
	errPlanDeprecated = errors.New("the subscription plan is deprecated")
	// errPlanOriginConflict refuses a plan created at a developer portal with the handle of a plan made elsewhere.
	errPlanOriginConflict = errors.New("a subscription plan with this handle was made elsewhere")
	// errStaleEvent refuses a webhook change made at the source before the plan's last change there:
	// applying it would bring back an older state.
	errStaleEvent = errors.New("the change is older than the plan's last change at its source")
)

// inUseError refuses changing a plan's name, conditions or expiry while subscriptions use it.
type inUseError struct{ msg string }

func (e *inUseError) Error() string { return e.msg }

func planEditInUse(name string) error {
	return &inUseError{msg: fmt.Sprintf("The plan %s has subscriptions. To change its name, conditions or expiry, create a new plan.", name)}
}

// planChanged is the payload of EventPlanChanged.
type planChanged struct {
	OrgID      string      `json:"orgId"`
	PlanID     string      `json:"planId"`
	Handle     string      `json:"handle"`
	PlanName   string      `json:"planName"`
	Action     string      `json:"action"` // created, updated or deleted
	Status     string      `json:"status,omitempty"`
	ExpiryTime *time.Time  `json:"expiryTime,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// planService holds the plan rules. It reads and writes only the module's own tables.
type planService struct {
	db      func() *gorm.DB
	publish func(ctx context.Context, tx *gorm.DB, data planChanged) error
	// usage counts what offers or uses a plan: the MCP proxies that offer it and the subscriptions on it.
	usage func(ctx context.Context, tx *gorm.DB, orgID, handle string) (usage, error)
	// offers counts the MCP proxies that offer a plan, as the AI Gateway reports them
	// (subplanv1.Source.CountOffersWith); nil without a gateway.
	offers atomic.Pointer[subplanv1.OfferCounter]
	log    *slog.Logger
	now    func() time.Time
}

type planRow struct {
	ID          string
	OrgID       string
	Handle      string
	DisplayName string
	Description *string
	Status      string
	ExpiryTime  *time.Time
	CreatedBy   *string
	UpdatedBy   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// SourceChangedAt, Origin and DeprecatedAt, see plan.
	SourceChangedAt *time.Time
	Origin          string
	DeprecatedAt    *time.Time
}

type conditionRow struct {
	PlanID        string
	ConditionID   string
	ConditionType string
	Params        string
}

const planColumns = `id, org_id, handle, display_name, description, status, expiry_time, created_by, updated_by, ` +
	`created_at, updated_at, source_changed_at, origin, deprecated_at`

// foldName is how names are compared in a search: Unicode case-folded, in one normal form. It is stored with each
// plan (display_name_folded), since SQLite's LOWER folds ASCII only.
func foldName(s string) string { return cases.Fold().String(norm.NFC.String(s)) }

// listOptions pages and filters a plan list (GET /subscription-plans).
type listOptions struct {
	Limit  int
	Offset int
	Search string // case-insensitive match on the name or handle
	Status string // "" for any
}

// searchClause matches the name or handle, case-insensitively, with LIKE wildcards in the search taken literally.
func searchClause(search string) (string, []any) {
	s := strings.TrimSpace(search)
	if s == "" {
		return "", nil
	}
	escape := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace
	// A handle is ASCII, so LOWER folds it on every database.
	return ` AND (display_name_folded LIKE ? ESCAPE '\' OR LOWER(handle) LIKE ? ESCAPE '\')`,
		[]any{"%" + escape(foldName(s)) + "%", "%" + escape(strings.ToLower(s)) + "%"}
}

func listFilter(orgID string, opts listOptions) (string, []any) {
	where := ` WHERE org_id = ?`
	args := []any{orgID}
	clause, searchArgs := searchClause(opts.Search)
	where += clause
	args = append(args, searchArgs...)
	if opts.Status != "" {
		where += ` AND status = ?`
		args = append(args, opts.Status)
	}
	return where, args
}

// list returns a page of the organization's plans, current ones first and then deprecated ones, each newest
// first, and the number of plans that match. The order is in the query, so every page follows it. The plans
// and their conditions are read in one snapshot (read), so a plan never pairs one version's fields with
// another's conditions.
func (s *planService) list(ctx context.Context, orgID string, opts listOptions) ([]*plan, int, error) {
	var out []*plan
	var total int64
	err := s.read(ctx, func(tx *gorm.DB) error {
		where, args := listFilter(orgID, opts)
		if err := tx.Raw(`SELECT COUNT(*) FROM mod_ai_hub_subscription_plans`+where, args...).Scan(&total).Error; err != nil {
			return fmt.Errorf("count plans: %w", err)
		}
		var rows []planRow
		if err := tx.Raw(`SELECT `+planColumns+` FROM mod_ai_hub_subscription_plans`+where+
			` ORDER BY CASE WHEN deprecated_at IS NULL THEN 0 ELSE 1 END, created_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, opts.Limit, opts.Offset)...).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("list plans: %w", err)
		}
		var err error
		out, err = s.withConditions(tx, orgID, rows)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return out, int(total), nil
}

// read runs fn in a read-only transaction that sees one snapshot: REPEATABLE READ on PostgreSQL; on SQLite a
// transaction already does.
func (s *planService) read(ctx context.Context, fn func(tx *gorm.DB) error) error {
	db := s.db().WithContext(ctx)
	if db.Name() == "postgres" {
		return db.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	}
	return db.Transaction(fn)
}

// lockPlan holds the plan's row until the transaction ends, so two changes of one plan run one after the
// other and the second reads what the first wrote. A plan that doesn't exist locks nothing; the read that
// follows reports it.
func lockPlan(tx *gorm.DB, orgID, handle string) error {
	err := dbx.LockRow(tx, "mod_ai_hub_subscription_plans", "id", "org_id = ? AND handle = ?", orgID, handle)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lock plan %s: %w", handle, err)
	}
	return nil
}

func (s *planService) get(ctx context.Context, orgID, handle string) (*plan, error) {
	var p *plan
	err := s.read(ctx, func(tx *gorm.DB) error {
		var err error
		p, err = s.getTx(tx, orgID, handle)
		return err
	})
	return p, err
}

// getByID reads a plan by its ID.
func (s *planService) getByID(ctx context.Context, orgID, id string) (*plan, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, errPlanNotFound
	}
	var p *plan
	err := s.read(ctx, func(tx *gorm.DB) error {
		var rows []planRow
		if err := tx.Raw(`SELECT `+planColumns+` FROM mod_ai_hub_subscription_plans WHERE org_id = ? AND id = ?`,
			orgID, id).Scan(&rows).Error; err != nil {
			return fmt.Errorf("get plan %s: %w", id, err)
		}
		if len(rows) == 0 {
			return errPlanNotFound
		}
		plans, err := s.withConditions(tx, orgID, rows)
		if err != nil {
			return err
		}
		p = plans[0]
		return nil
	})
	return p, err
}

func (s *planService) getTx(tx *gorm.DB, orgID, handle string) (*plan, error) {
	var rows []planRow
	if err := tx.Raw(`SELECT `+planColumns+` FROM mod_ai_hub_subscription_plans WHERE org_id = ? AND handle = ?`,
		orgID, handle).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("get plan %s: %w", handle, err)
	}
	if len(rows) == 0 {
		return nil, errPlanNotFound
	}
	plans, err := s.withConditions(tx, orgID, rows)
	if err != nil {
		return nil, err
	}
	return plans[0], nil
}

// withConditions reads the plans' conditions, in one query. Stored params are normalized again, because
// PostgreSQL's JSONB reorders keys and adds spaces: without this, equal conditions would compare as different.
// A stored condition that no longer validates is kept as it is.
func (s *planService) withConditions(tx *gorm.DB, orgID string, rows []planRow) ([]*plan, error) {
	out := make([]*plan, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	var conds []conditionRow
	if err := tx.Raw(`SELECT plan_id, condition_id, condition_type, params FROM mod_ai_hub_subscription_plan_conditions `+
		`WHERE org_id = ? AND plan_id IN ? ORDER BY plan_id, sort_order`, orgID, ids).Scan(&conds).Error; err != nil {
		return nil, fmt.Errorf("get conditions of plans: %w", err)
	}
	byPlan := map[string][]Condition{}
	for _, c := range conds {
		byPlan[c.PlanID] = append(byPlan[c.PlanID], Condition{ID: c.ConditionID, Type: c.ConditionType, Params: []byte(c.Params)})
	}
	for _, r := range rows {
		out = append(out, s.planFrom(r, byPlan[r.ID]))
	}
	return out, nil
}

// planFrom builds a plan from its row and its stored conditions.
func (s *planService) planFrom(r planRow, raw []Condition) *plan {
	p := &plan{
		UUID: r.ID, OrgID: r.OrgID, Handle: r.Handle, Name: r.DisplayName, Description: "", Status: r.Status,
		ExpiryTime: nil, Conditions: []Condition{}, CreatedBy: "", UpdatedBy: "",
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), SourceChangedAt: nil,
		Origin: r.Origin, DeprecatedAt: nil,
	}
	if r.DeprecatedAt != nil {
		t := r.DeprecatedAt.UTC()
		p.DeprecatedAt = &t
	}
	if r.Description != nil {
		p.Description = *r.Description
	}
	if r.ExpiryTime != nil {
		t := r.ExpiryTime.UTC()
		p.ExpiryTime = &t
	}
	if r.CreatedBy != nil {
		p.CreatedBy = *r.CreatedBy
	}
	if r.UpdatedBy != nil {
		p.UpdatedBy = *r.UpdatedBy
	}
	if r.SourceChangedAt != nil {
		t := r.SourceChangedAt.UTC()
		p.SourceChangedAt = &t
	}
	if len(raw) == 0 {
		return p
	}
	if normalized, err := normalizeConditions(raw); err == nil {
		p.Conditions = normalized
	} else {
		s.log.Warn("subscription plan has stored conditions that no longer validate", "orgId", r.OrgID,
			"plan", r.Handle, "error", err)
		p.Conditions = raw
	}
	return p
}

func (s *planService) getUsage(ctx context.Context, p *plan) (usage, error) {
	return s.usage(ctx, s.db().WithContext(ctx), p.OrgID, p.Handle)
}

// create stores a new plan. The handler has checked the id, name and status.
func (s *planService) create(ctx context.Context, orgID, actor string, p *plan) (*plan, error) {
	if p.Status == "" {
		p.Status = StatusActive
	}
	if p.Origin == "" {
		p.Origin = OriginControlPlane
	}
	if err := checkDescription(p.Description); err != nil {
		return nil, err
	}
	conditions, err := normalizeConditions(p.Conditions)
	if err != nil {
		return nil, err
	}
	p.Conditions = conditions
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("new plan id: %w", err)
	}
	now := s.now().UTC()
	createdAt := now
	if !p.CreatedAt.IsZero() {
		// A plan copied in by webhook is dated when it was made at its source (upsertFromWebhook).
		createdAt = p.CreatedAt.UTC()
	}
	var created *plan
	err = s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Only a taken handle is a conflict; any other error is returned as it is. The insert runs in a
		// savepoint, since a failed statement aborts the transaction on PostgreSQL.
		err := tx.Transaction(func(sp *gorm.DB) error {
			return sp.Exec(`INSERT INTO mod_ai_hub_subscription_plans (`+planColumns+`, display_name_folded) `+
				`VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id.String(), orgID, p.Handle, p.Name, nullable(p.Description), p.Status, p.ExpiryTime,
				nullable(actor), nullable(actor), createdAt, now, p.SourceChangedAt, p.Origin, nil, foldName(p.Name)).Error
		})
		if dberr.IsDuplicateKey(err) {
			return errPlanExists
		}
		if err != nil {
			return fmt.Errorf("insert plan %s: %w", p.Handle, err)
		}
		if err := insertConditions(tx, orgID, id.String(), p.Conditions); err != nil {
			return err
		}
		if created, err = s.getTx(tx, orgID, p.Handle); err != nil {
			return err
		}
		return s.publish(ctx, tx, changedEvent(created, "created"))
	})
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "subscription plan created", "orgId", orgID, "plan", created.Handle, "planId", created.UUID,
		"conditions", len(created.Conditions), "status", created.Status, "origin", created.Origin, "actor", actor)
	return created, nil
}

// update changes a plan. The name, conditions and expiry can change only while no subscription uses the plan;
// the description and status can always change. A deprecated plan can't change (errPlanDeprecated).
func (s *planService) update(ctx context.Context, orgID, handle, actor string, u *planUpdate) (*plan, error) {
	var updated *plan
	err := s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlan(tx, orgID, handle); err != nil {
			return err
		}
		existing, err := s.getTx(tx, orgID, handle)
		if err != nil {
			return err
		}
		if existing.deprecated() {
			return errPlanDeprecated
		}
		if at := u.SourceChangedAt; at != nil {
			if existing.SourceChangedAt != nil && at.Before(*existing.SourceChangedAt) {
				return errStaleEvent
			}
			existing.SourceChangedAt = at
		}
		conditions, err := updatedConditions(existing, u)
		if err != nil {
			return err
		}
		if !u.FromWebhook && changesLockedValues(existing, u, conditions) {
			use, err := s.usage(ctx, tx, orgID, existing.Handle)
			if err != nil {
				return err
			}
			if use.inUse() {
				return planEditInUse(existing.Name)
			}
		}
		if u.Name != nil {
			name := strings.TrimSpace(*u.Name)
			if name == "" {
				return invalid("name is required")
			}
			existing.Name = name
		}
		if u.Description != nil {
			existing.Description = strings.TrimSpace(*u.Description)
			if err := checkDescription(existing.Description); err != nil {
				return err
			}
		}
		if u.ExpiryTime != nil {
			t := u.ExpiryTime.UTC()
			existing.ExpiryTime = &t
		} else if u.ClearExpiryTime {
			existing.ExpiryTime = nil
		}
		if u.Status != nil {
			switch *u.Status {
			case StatusActive, StatusInactive:
				existing.Status = *u.Status
			default:
				return invalid("invalid status: %s", *u.Status)
			}
		}
		now := s.now().UTC()
		if err := tx.Exec(`UPDATE mod_ai_hub_subscription_plans SET display_name = ?, display_name_folded = ?, `+
			`description = ?, status = ?, expiry_time = ?, updated_by = ?, updated_at = ?, source_changed_at = ? `+
			`WHERE org_id = ? AND id = ?`,
			existing.Name, foldName(existing.Name), nullable(existing.Description), existing.Status, existing.ExpiryTime,
			nullable(actor), now, existing.SourceChangedAt, orgID, existing.UUID).Error; err != nil {
			return fmt.Errorf("update plan %s: %w", handle, err)
		}
		if !sameConditions(existing.Conditions, conditions) {
			if err := tx.Exec(`DELETE FROM mod_ai_hub_subscription_plan_conditions WHERE org_id = ? AND plan_id = ?`,
				orgID, existing.UUID).Error; err != nil {
				return fmt.Errorf("replace conditions of plan %s: %w", handle, err)
			}
			if err := insertConditions(tx, orgID, existing.UUID, conditions); err != nil {
				return err
			}
		}
		if updated, err = s.getTx(tx, orgID, handle); err != nil {
			return err
		}
		return s.publish(ctx, tx, changedEvent(updated, "updated"))
	})
	if err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "subscription plan updated", "orgId", orgID, "plan", handle, "planId", updated.UUID,
		"status", updated.Status, "actor", actor)
	return updated, nil
}

// deprecate marks a plan deprecated. It stays, for the subscriptions on it, and gateways still get it; but it
// can't be changed, offered or subscribed to again. Deprecating a deprecated plan changes nothing.
func (s *planService) deprecate(ctx context.Context, orgID, handle, actor string) (*plan, error) {
	return s.deprecateAt(ctx, orgID, handle, actor, nil)
}

// deprecateAt deprecates a plan; at, when set, is when its source deprecated it, by the source's clock.
func (s *planService) deprecateAt(ctx context.Context, orgID, handle, actor string, at *time.Time) (*plan, error) {
	var deprecated *plan
	changed := false
	err := s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlan(tx, orgID, handle); err != nil {
			return err
		}
		existing, err := s.getTx(tx, orgID, handle)
		if err != nil {
			return err
		}
		if existing.deprecated() {
			deprecated = existing
			return nil
		}
		now := s.now().UTC()
		when, sourceChangedAt := now, existing.SourceChangedAt
		if at != nil {
			when, sourceChangedAt = at.UTC(), at
		}
		if err := tx.Exec(`UPDATE mod_ai_hub_subscription_plans SET deprecated_at = ?, updated_by = ?, updated_at = ?, `+
			`source_changed_at = ? WHERE org_id = ? AND id = ?`,
			when, nullable(actor), now, sourceChangedAt, orgID, existing.UUID).Error; err != nil {
			return fmt.Errorf("deprecate plan %s: %w", handle, err)
		}
		if deprecated, err = s.getTx(tx, orgID, handle); err != nil {
			return err
		}
		changed = true
		return s.publish(ctx, tx, changedEvent(deprecated, "updated"))
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.log.InfoContext(ctx, "subscription plan deprecated", "orgId", orgID, "plan", handle, "planId", deprecated.UUID,
			"actor", actor)
	}
	return deprecated, nil
}

func changedEvent(p *plan, action string) planChanged {
	return planChanged{
		OrgID: p.OrgID, PlanID: p.UUID, Handle: p.Handle, PlanName: p.Name, Action: action,
		Status: p.Status, ExpiryTime: p.ExpiryTime, Conditions: p.Conditions,
	}
}

// changesLockedValues reports whether the update changes the name, conditions or expiry, which a plan in use keeps. Values
// are compared, not which fields were sent; names ignore surrounding spaces.
func changesLockedValues(existing *plan, u *planUpdate, conditions []Condition) bool {
	if u.Name != nil && strings.TrimSpace(*u.Name) != strings.TrimSpace(existing.Name) {
		return true
	}
	if changesExpiry(existing.ExpiryTime, u) {
		return true
	}
	return !sameConditions(existing.Conditions, conditions)
}

// changesExpiry reports whether the update sets an expiry other than the plan's, or clears one it has. Times are
// compared to the second, so an expiry read back and sent unchanged is not a change.
func changesExpiry(current *time.Time, u *planUpdate) bool {
	switch {
	case u.ExpiryTime != nil:
		return current == nil || !current.Truncate(time.Second).Equal(u.ExpiryTime.Truncate(time.Second))
	case u.ClearExpiryTime:
		return current != nil
	default:
		return false
	}
}

// updatedConditions returns the plan's conditions after the update, normalized.
func updatedConditions(existing *plan, u *planUpdate) ([]Condition, error) {
	switch {
	case u.Conditions != nil:
		return normalizeConditions(*u.Conditions)
	case u.RequestLimit != nil:
		return normalizeConditions(withRequestLimit(existing.Conditions, u.RequestLimit))
	case u.ClearLimit:
		return normalizeConditions(withRequestLimit(existing.Conditions, nil))
	default:
		return existing.Conditions, nil
	}
}

// withRequestLimit replaces the request-count condition with requestLimit, or removes it when nil, and keeps the
// other conditions. Used when a client sends the old "limits" field.
func withRequestLimit(conditions []Condition, requestLimit *Condition) []Condition {
	out := make([]Condition, 0, len(conditions)+1)
	replaced := false
	for _, c := range conditions {
		if c.Type != ConditionTypeRequestCount {
			out = append(out, c)
			continue
		}
		if requestLimit != nil && !replaced {
			out = append(out, *requestLimit)
			replaced = true
		}
	}
	if requestLimit != nil && !replaced {
		out = append([]Condition{*requestLimit}, out...)
	}
	return out
}

func checkDescription(description string) error {
	if utf8.RuneCountInString(description) > maxDescriptionChars {
		return invalid("description must be at most %d characters", maxDescriptionChars)
	}
	return nil
}

func insertConditions(tx *gorm.DB, orgID, planID string, conditions []Condition) error {
	for i, c := range conditions {
		if err := tx.Exec(`INSERT INTO mod_ai_hub_subscription_plan_conditions (org_id, plan_id, condition_id, condition_type, `+
			`params, sort_order) VALUES (?, ?, ?, ?, ?, ?)`, orgID, planID, c.ID, c.Type, string(c.Params), i).Error; err != nil {
			return fmt.Errorf("insert condition %s: %w", c.ID, err)
		}
	}
	return nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// proxyUsage counts what uses a plan: the subscriptions on it, and the MCP proxies that offer it, with the
// counter the AI Gateway handed over (without one, nothing offers plans).
func (s *planService) proxyUsage(ctx context.Context, tx *gorm.DB, orgID, handle string) (usage, error) {
	subs, err := countOnPlan(tx, orgID, handle)
	if err != nil {
		return usage{}, err
	}
	count := s.offers.Load()
	if count == nil || *count == nil {
		return usage{Subscriptions: subs}, nil
	}
	n, err := (*count)(ctx, orgID, handle)
	if err != nil {
		return usage{}, fmt.Errorf("count MCP proxies offering plan %s: %w", handle, err)
	}
	return usage{MCPProxies: n, Subscriptions: subs}, nil
}
