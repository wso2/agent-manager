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
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ShadowCorrelationSource is the signal source of signals derived across
// connections (not from one provider call).
const ShadowCorrelationSource = "correlation"

// ShadowFindingInput is one agent of a connection's scan, as persisted.
type ShadowFindingInput struct {
	Finding models.ShadowFinding
	Signals []models.ShadowFindingSignal
}

// ShadowConnectionApply is what one connection's scan found.
type ShadowConnectionApply struct {
	OUID          string
	ConnectionID  uuid.UUID
	ScanID        uuid.UUID
	ScanStartedAt time.Time
	Findings      []ShadowFindingInput
	// Complete is true when every listing the connection's settings enable
	// succeeded completely. Only then is an agent of the connection that the
	// scan did not return marked as no longer present, and its stale
	// evidence removed.
	Complete bool
}

// ShadowFindingFilter selects agents.
type ShadowFindingFilter struct {
	Provider     string
	Status       string
	ConnectionID *uuid.UUID
	// ScanID keeps the agents that scan found, present or not. Without it,
	// only the agents present now are listed.
	ScanID *uuid.UUID
	Search string
	Limit  int
	Offset int
}

// ShadowFindingSource is one connection that found an agent: the agent's
// finding in that connection.
type ShadowFindingSource struct {
	FindingID      string `json:"findingId"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Present        bool   `json:"present"`
}

// ShadowFindingRow is one agent: a finding, its connection's name, and
// every connection that found the same agent (same platform, kind and
// external ID).
type ShadowFindingRow struct {
	models.ShadowFinding
	ConnectionName string                `gorm:"column:connection_name"`
	FoundByJSON    string                `gorm:"column:found_by"`
	FoundBy        []ShadowFindingSource `gorm:"-"`
}

// ShadowSummary counts an org's agents (each agent once, however many
// connections found it) and connections.
type ShadowSummary struct {
	Total       int
	ByProvider  map[string]int
	ByStatus    map[string]int
	Connections int
}

// ShadowAgentRepository persists Shadow Agent Discovery connections, scans
// and findings. Every method is scoped to an org (ou_id) except the scan
// worker's claim, which works across orgs by design.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/shadow_agent_repository_mock.go . ShadowAgentRepository:ShadowAgentRepositoryMock
type ShadowAgentRepository interface {
	ListConnections(ctx context.Context, ouID string) ([]models.ShadowConnection, error)
	GetConnection(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowConnection, error)
	// CreateConnection fails with utils.ErrShadowConnectionNameTaken on a
	// duplicate name.
	CreateConnection(ctx context.Context, c *models.ShadowConnection) error
	UpdateConnection(ctx context.Context, c *models.ShadowConnection) error
	DeleteConnection(ctx context.Context, ouID string, id uuid.UUID) (bool, error)
	SaveConnectionTest(ctx context.Context, ouID string, id uuid.UUID, at time.Time, result models.ShadowConnectionTestInfo) error

	// CreateScan fails with utils.ErrShadowScanInProgress while another scan
	// of the org is queued or running.
	CreateScan(ctx context.Context, s *models.ShadowScan) error
	ListScans(ctx context.Context, ouID string, limit int) ([]models.ShadowScan, error)
	GetScan(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowScan, error)
	// DeleteFinishedScans deletes the org's scans that are not queued or
	// running, and returns how many it deleted. Agents and their evidence are
	// kept; only the scan records and their per-scan lists go.
	DeleteFinishedScans(ctx context.Context, ouID string) (int64, error)
	// LatestScan returns gorm.ErrRecordNotFound when the org has none.
	LatestScan(ctx context.Context, ouID string) (*models.ShadowScan, error)
	// ClaimDueScans leases queued scans, and running scans whose lease ran
	// out (their worker died), across orgs.
	ClaimDueScans(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.ShadowScan, error)
	RenewScanLease(ctx context.Context, id uuid.UUID, until time.Time) error
	FinishScan(ctx context.Context, s *models.ShadowScan) error

	// ApplyConnectionResult writes one connection's scan in one transaction
	// and returns how many agents it wrote.
	ApplyConnectionResult(ctx context.Context, in ShadowConnectionApply) (int, error)
	// ListOrgFindings returns the org's present agents (for correlation).
	ListOrgFindings(ctx context.Context, ouID string) ([]models.ShadowFinding, error)
	// ApplyCorrelation replaces the org's cross-connection relations and
	// evidence.
	ApplyCorrelation(ctx context.Context, ouID string, relations map[uuid.UUID][]models.ShadowFindingRelation,
		signals map[uuid.UUID][]models.ShadowFindingSignal) error

	// ListFindings lists agents, one row per agent however many connections
	// found it.
	ListFindings(ctx context.Context, ouID string, f ShadowFindingFilter) ([]ShadowFindingRow, int, error)
	GetFinding(ctx context.Context, ouID string, id uuid.UUID) (*ShadowFindingRow, error)
	ListSignals(ctx context.Context, ouID string, findingID uuid.UUID) ([]models.ShadowFindingSignal, error)
	// UpdateFindingStatus sets the review status of the agent, on its
	// finding in every connection that found it.
	UpdateFindingStatus(ctx context.Context, ouID string, id uuid.UUID, status string) error
	// DeleteFinding deletes an agent: every row of it (one per connection
	// that found it) with its evidence and its per-scan links. It returns
	// false when the org has no finding with that ID.
	DeleteFinding(ctx context.Context, ouID string, id uuid.UUID) (bool, error)
	// DeleteFindings deletes every agent of the org with its evidence, and
	// returns how many agents it deleted. Connections and scans are kept.
	DeleteFindings(ctx context.Context, ouID string) (int64, error)
	Summary(ctx context.Context, ouID string) (ShadowSummary, error)
}

type shadowAgentRepository struct {
	db *gorm.DB
}

// NewShadowAgentRepository creates a ShadowAgentRepository.
func NewShadowAgentRepository(db *gorm.DB) ShadowAgentRepository {
	return &shadowAgentRepository{db: db}
}

// ---- connections ----

func (r *shadowAgentRepository) ListConnections(ctx context.Context, ouID string) ([]models.ShadowConnection, error) {
	var out []models.ShadowConnection
	err := r.db.WithContext(ctx).Where("ou_id = ?", ouID).Order("lower(name)").Find(&out).Error
	return out, err
}

func (r *shadowAgentRepository) GetConnection(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowConnection, error) {
	var c models.ShadowConnection
	if err := r.db.WithContext(ctx).Where("ou_id = ? AND id = ?", ouID, id).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *shadowAgentRepository) CreateConnection(ctx context.Context, c *models.ShadowConnection) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	c.CreatedAt, c.UpdatedAt = now, now
	err := r.db.WithContext(ctx).Create(c).Error
	if pgErrorCode(err) == pgUniqueViolation {
		return utils.ErrShadowConnectionNameTaken
	}
	return err
}

func (r *shadowAgentRepository) UpdateConnection(ctx context.Context, c *models.ShadowConnection) error {
	c.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	cfg, err := toJSON(c.Config)
	if err != nil {
		return err
	}
	hints, err := toJSON(c.SecretHints)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&models.ShadowConnection{}).
		Where("ou_id = ? AND id = ?", c.OUID, c.ID).
		Updates(map[string]any{
			"name":             c.Name,
			"config":           gorm.Expr("?::jsonb", cfg),
			"secret_encrypted": c.SecretEncrypted,
			"secret_hints":     gorm.Expr("?::jsonb", hints),
			"updated_at":       c.UpdatedAt,
		})
	if pgErrorCode(res.Error) == pgUniqueViolation {
		return utils.ErrShadowConnectionNameTaken
	}
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *shadowAgentRepository) DeleteConnection(ctx context.Context, ouID string, id uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Where("ou_id = ? AND id = ?", ouID, id).Delete(&models.ShadowConnection{})
	return res.RowsAffected > 0, res.Error
}

func (r *shadowAgentRepository) SaveConnectionTest(
	ctx context.Context, ouID string, id uuid.UUID, at time.Time, result models.ShadowConnectionTestInfo,
) error {
	res, err := toJSON(result)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&models.ShadowConnection{}).
		Where("ou_id = ? AND id = ?", ouID, id).
		Updates(map[string]any{"last_tested_at": at, "last_test_result": gorm.Expr("?::jsonb", res)}).Error
}

// ---- scans ----

func (r *shadowAgentRepository) CreateScan(ctx context.Context, s *models.ShadowScan) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	s.CreatedAt, s.UpdatedAt = now, now
	if s.Results == nil {
		s.Results = []models.ShadowScanConnectionResult{}
	}
	err := r.db.WithContext(ctx).Create(s).Error
	if pgErrorCode(err) == pgUniqueViolation {
		return utils.ErrShadowScanInProgress
	}
	return err
}

func (r *shadowAgentRepository) ListScans(ctx context.Context, ouID string, limit int) ([]models.ShadowScan, error) {
	var out []models.ShadowScan
	err := r.db.WithContext(ctx).Where("ou_id = ?", ouID).Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

func (r *shadowAgentRepository) GetScan(ctx context.Context, ouID string, id uuid.UUID) (*models.ShadowScan, error) {
	var s models.ShadowScan
	if err := r.db.WithContext(ctx).Where("ou_id = ? AND id = ?", ouID, id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *shadowAgentRepository) DeleteFinishedScans(ctx context.Context, ouID string) (int64, error) {
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		finished := `SELECT id FROM shadow_scans WHERE ou_id = ? AND status NOT IN (?, ?)`
		active := []any{ouID, models.ShadowScanQueued, models.ShadowScanRunning}
		// last_scan_id and a signal's scan_id have no foreign key: clear them
		// so nothing points at a deleted scan.
		if err := tx.Exec(`UPDATE shadow_findings SET last_scan_id = NULL
			WHERE ou_id = ? AND last_scan_id IN (`+finished+`)`, append([]any{ouID}, active...)...).Error; err != nil {
			return fmt.Errorf("clearing last scan of findings: %w", err)
		}
		if err := tx.Exec(`UPDATE shadow_finding_signals SET scan_id = NULL
			WHERE ou_id = ? AND scan_id IN (`+finished+`)`, append([]any{ouID}, active...)...).Error; err != nil {
			return fmt.Errorf("clearing scan of signals: %w", err)
		}
		// shadow_scan_findings rows go with their scan (ON DELETE CASCADE).
		res := tx.Exec(`DELETE FROM shadow_scans WHERE ou_id = ? AND status NOT IN (?, ?)`, active...)
		if res.Error != nil {
			return fmt.Errorf("deleting scans: %w", res.Error)
		}
		deleted = res.RowsAffected
		return nil
	})
	return deleted, err
}

func (r *shadowAgentRepository) LatestScan(ctx context.Context, ouID string) (*models.ShadowScan, error) {
	var s models.ShadowScan
	if err := r.db.WithContext(ctx).Where("ou_id = ?", ouID).Order("created_at DESC").First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *shadowAgentRepository) ClaimDueScans(
	ctx context.Context, now time.Time, lease time.Duration, limit int,
) ([]models.ShadowScan, error) {
	var out []models.ShadowScan
	err := r.db.WithContext(ctx).Raw(`
		UPDATE shadow_scans s
		SET status = 'running', claimed_until = ?, started_at = COALESCE(s.started_at, ?),
			attempts = s.attempts + 1, updated_at = ?
		WHERE s.id IN (
			SELECT id FROM shadow_scans
			WHERE status = 'queued' OR (status = 'running' AND claimed_until < ?)
			ORDER BY created_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING s.*`,
		now.Add(lease), now, now, now, limit,
	).Scan(&out).Error
	return out, err
}

func (r *shadowAgentRepository) RenewScanLease(ctx context.Context, id uuid.UUID, until time.Time) error {
	return r.db.WithContext(ctx).Model(&models.ShadowScan{}).
		Where("id = ? AND status = 'running'", id).Update("claimed_until", until).Error
}

func (r *shadowAgentRepository) FinishScan(ctx context.Context, s *models.ShadowScan) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	results, err := toJSON(s.Results)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&models.ShadowScan{}).
		Where("ou_id = ? AND id = ?", s.OUID, s.ID).
		Updates(map[string]any{
			"status":         s.Status,
			"results":        gorm.Expr("?::jsonb", results),
			"findings_count": s.FindingsCount,
			"finished_at":    s.FinishedAt,
			"claimed_until":  nil,
			"updated_at":     now,
		}).Error
}

// ---- scan results ----

func (r *shadowAgentRepository) ApplyConnectionResult(ctx context.Context, in ShadowConnectionApply) (int, error) {
	written := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range in.Findings {
			fi := &in.Findings[i]
			id, err := upsertShadowFinding(tx, in, &fi.Finding)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO shadow_scan_findings (ou_id, scan_id, finding_id) VALUES (?, ?, ?)
				ON CONFLICT DO NOTHING`, in.OUID, in.ScanID, id).Error; err != nil {
				return fmt.Errorf("recording the scan's finding: %w", err)
			}
			for j := range fi.Signals {
				if err := upsertShadowSignal(tx, in.OUID, id, in.ScanID, &fi.Signals[j]); err != nil {
					return err
				}
			}
			written++
		}
		if !in.Complete {
			return nil
		}
		// Every listing succeeded: an agent this scan did not return is no
		// longer listed by the platform (or no longer in the connection's
		// scope), and evidence this scan did not see again is gone.
		if err := tx.Exec(`UPDATE shadow_findings SET present = FALSE, updated_at = NOW()
			WHERE ou_id = ? AND connection_id = ? AND present AND last_seen_at < ?`,
			in.OUID, in.ConnectionID, in.ScanStartedAt).Error; err != nil {
			return fmt.Errorf("marking agents no longer listed: %w", err)
		}
		if err := tx.Exec(`DELETE FROM shadow_finding_signals s USING shadow_findings f
			WHERE s.finding_id = f.id AND f.ou_id = ? AND f.connection_id = ? AND f.present
				AND s.source <> ? AND s.detected_at < ?`,
			in.OUID, in.ConnectionID, ShadowCorrelationSource, in.ScanStartedAt).Error; err != nil {
			return fmt.Errorf("removing evidence no longer seen: %w", err)
		}
		return nil
	})
	return written, err
}

func upsertShadowFinding(tx *gorm.DB, in ShadowConnectionApply, f *models.ShadowFinding) (uuid.UUID, error) {
	var row struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	now := in.ScanStartedAt
	owner, err := ownerHintJSON(f.OwnerHint)
	if err != nil {
		return uuid.Nil, err
	}
	attrs, err := toJSON(nonNilMap(f.Attributes))
	if err != nil {
		return uuid.Nil, err
	}
	err = tx.Raw(`
		INSERT INTO shadow_findings (ou_id, connection_id, provider, kind, external_id, display_name, scope, state,
			owner_hint, attributes, provider_created_at, first_seen_at, last_seen_at, last_scan_id, present, status, related)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?, ?, ?, ?, TRUE, ?, '[]'::jsonb)
		ON CONFLICT (ou_id, connection_id, kind, external_id) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			scope = EXCLUDED.scope,
			state = EXCLUDED.state,
			owner_hint = EXCLUDED.owner_hint,
			attributes = EXCLUDED.attributes,
			provider_created_at = COALESCE(EXCLUDED.provider_created_at, shadow_findings.provider_created_at),
			last_seen_at = EXCLUDED.last_seen_at,
			last_scan_id = EXCLUDED.last_scan_id,
			present = TRUE,
			updated_at = NOW()
		RETURNING id`,
		in.OUID, in.ConnectionID, f.Provider, f.Kind, f.ExternalID, f.DisplayName, f.Scope, f.State,
		owner, attrs, f.ProviderCreatedAt, now, now, in.ScanID, shadowNewFindingStatus(tx, in.OUID, f),
	).Scan(&row).Error
	if err != nil {
		return uuid.Nil, fmt.Errorf("writing finding: %w", err)
	}
	return row.ID, nil
}

// shadowNewFindingStatus is the review status a new finding starts with:
// the status the same agent already has through another connection, so the
// agent has one status however many connections found it.
func shadowNewFindingStatus(tx *gorm.DB, ouID string, f *models.ShadowFinding) string {
	var status string
	err := tx.Raw(`SELECT status FROM shadow_findings
		WHERE ou_id = ? AND provider = ? AND kind = ? AND external_id = ?
		ORDER BY updated_at DESC LIMIT 1`, ouID, f.Provider, f.Kind, f.ExternalID).Scan(&status).Error
	if err != nil || status == "" {
		return models.ShadowStatusNew
	}
	return status
}

func upsertShadowSignal(tx *gorm.DB, ouID string, findingID, scanID uuid.UUID, s *models.ShadowFindingSignal) error {
	observed, err := toJSON(nonNilMap(s.Observed))
	if err != nil {
		return err
	}
	var scan *uuid.UUID
	if scanID != uuid.Nil {
		scan = &scanID
	}
	err = tx.Exec(`
		INSERT INTO shadow_finding_signals (ou_id, finding_id, code, source, observed, scan_id, detected_at)
		VALUES (?, ?, ?, ?, ?::jsonb, ?, ?)
		ON CONFLICT (finding_id, code, source) DO UPDATE SET
			observed = EXCLUDED.observed, scan_id = EXCLUDED.scan_id, detected_at = EXCLUDED.detected_at`,
		ouID, findingID, s.Code, s.Source, observed, scan, s.DetectedAt,
	).Error
	if err != nil {
		return fmt.Errorf("writing evidence: %w", err)
	}
	return nil
}

func (r *shadowAgentRepository) ListOrgFindings(ctx context.Context, ouID string) ([]models.ShadowFinding, error) {
	var out []models.ShadowFinding
	err := r.db.WithContext(ctx).Where("ou_id = ? AND present", ouID).Find(&out).Error
	return out, err
}

func (r *shadowAgentRepository) ApplyCorrelation(
	ctx context.Context, ouID string, relations map[uuid.UUID][]models.ShadowFindingRelation,
	signals map[uuid.UUID][]models.ShadowFindingSignal,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM shadow_finding_signals WHERE ou_id = ? AND source = ?`,
			ouID, ShadowCorrelationSource).Error; err != nil {
			return fmt.Errorf("clearing correlation evidence: %w", err)
		}
		for id, list := range signals {
			for i := range list {
				list[i].Source = ShadowCorrelationSource
				if err := upsertShadowSignal(tx, ouID, id, uuid.Nil, &list[i]); err != nil {
					return err
				}
			}
		}
		if err := tx.Exec(`UPDATE shadow_findings SET related = '[]'::jsonb WHERE ou_id = ? AND related <> '[]'::jsonb`,
			ouID).Error; err != nil {
			return fmt.Errorf("clearing relations: %w", err)
		}
		for id, rels := range relations {
			for i := range rels {
				rels[i].Source = ShadowCorrelationSource
			}
			encoded, err := toJSON(rels)
			if err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE shadow_findings SET related = ?::jsonb WHERE ou_id = ? AND id = ?`,
				encoded, ouID, id).Error; err != nil {
				return fmt.Errorf("writing relations: %w", err)
			}
		}
		return nil
	})
}

// ---- agents ----

// shadowFoundBy aggregates every connection whose finding is the same agent
// (same provider, kind and external ID), present or not.
const shadowFoundBy = `jsonb_agg(jsonb_build_object('findingId', id, 'connectionId', connection_id,
	'connectionName', connection_name, 'present', present) ORDER BY lower(connection_name), id)`

// shadowFoundByAll is, per agent of the org (first argument), every
// connection that found it, whatever the list's filters.
const shadowFoundByAll = `SELECT provider, kind, external_id, ` + shadowFoundBy + ` AS found_by
	FROM (SELECT a.id, a.provider, a.kind, a.external_id, a.connection_id, a.present, ac.name AS connection_name
		FROM shadow_findings a JOIN shadow_connections ac ON ac.id = a.connection_id WHERE a.ou_id = ?) same
	GROUP BY provider, kind, external_id`

func (r *shadowAgentRepository) ListFindings(
	ctx context.Context, ouID string, flt ShadowFindingFilter,
) ([]ShadowFindingRow, int, error) {
	where := []string{"f.ou_id = ?"}
	args := []any{ouID}
	if flt.ScanID != nil {
		where = append(where, "EXISTS (SELECT 1 FROM shadow_scan_findings sf WHERE sf.finding_id = f.id AND sf.scan_id = ?)")
		args = append(args, *flt.ScanID)
	} else {
		where = append(where, "f.present")
	}
	if flt.Provider != "" {
		where = append(where, "f.provider = ?")
		args = append(args, flt.Provider)
	}
	if flt.Status != "" {
		where = append(where, "f.status = ?")
		args = append(args, flt.Status)
	}
	if flt.ConnectionID != nil {
		where = append(where, "f.connection_id = ?")
		args = append(args, *flt.ConnectionID)
	}
	if s := strings.TrimSpace(flt.Search); s != "" {
		like := "%" + escapeLike(strings.ToLower(s)) + "%"
		where = append(where, `(lower(f.display_name) LIKE ? ESCAPE '\' OR lower(f.external_id) LIKE ? ESCAPE '\'
			OR lower(f.scope) LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	base := `SELECT f.*, c.name AS connection_name FROM shadow_findings f
		JOIN shadow_connections c ON c.id = f.connection_id WHERE ` + strings.Join(where, " AND ")

	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM (SELECT DISTINCT provider, kind, external_id FROM (`+
		base+`) b) g`, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ShadowFindingRow
	// One row per agent: its finding in the connection that saw it last.
	// found_by lists every connection that found the agent, whatever the
	// filters, so a row always says where else the agent was found.
	q := `WITH b AS (` + base + `),
		g AS (
			SELECT b.*,
				ROW_NUMBER() OVER (PARTITION BY provider, kind, external_id
					ORDER BY present DESC, last_seen_at DESC, lower(connection_name), id) AS rn
			FROM b),
		fb AS (` + shadowFoundByAll + `)
		SELECT g.*, fb.found_by FROM g JOIN fb USING (provider, kind, external_id)
		WHERE g.rn = 1
		ORDER BY lower(g.display_name), g.provider, g.external_id
		LIMIT ? OFFSET ?`
	qargs := append(append([]any{}, args...), ouID, flt.Limit, flt.Offset)
	if err := r.db.WithContext(ctx).Raw(q, qargs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	for i := range rows {
		if err := decodeFoundBy(&rows[i]); err != nil {
			return nil, 0, err
		}
	}
	return rows, int(total), nil
}

func (r *shadowAgentRepository) GetFinding(ctx context.Context, ouID string, id uuid.UUID) (*ShadowFindingRow, error) {
	var rows []ShadowFindingRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT f.*, c.name AS connection_name,
			(SELECT `+shadowFoundBy+` FROM (
				SELECT g.id, g.connection_id, gc.name AS connection_name, g.present
				FROM shadow_findings g JOIN shadow_connections gc ON gc.id = g.connection_id
				WHERE g.ou_id = f.ou_id AND g.provider = f.provider AND g.kind = f.kind AND g.external_id = f.external_id
			) same) AS found_by
		FROM shadow_findings f JOIN shadow_connections c ON c.id = f.connection_id
		WHERE f.ou_id = ? AND f.id = ?
		LIMIT 1`, ouID, id).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if err := decodeFoundBy(&rows[0]); err != nil {
		return nil, err
	}
	return &rows[0], nil
}

func decodeFoundBy(row *ShadowFindingRow) error {
	row.FoundBy = []ShadowFindingSource{}
	if strings.TrimSpace(row.FoundByJSON) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(row.FoundByJSON), &row.FoundBy); err != nil {
		return fmt.Errorf("decoding the connections that found an agent: %w", err)
	}
	return nil
}

// ListSignals returns the evidence of the agent a finding stands for, from
// every connection that found it: each code and source once, as last seen.
func (r *shadowAgentRepository) ListSignals(ctx context.Context, ouID string, findingID uuid.UUID) ([]models.ShadowFindingSignal, error) {
	var out []models.ShadowFindingSignal
	err := r.db.WithContext(ctx).Raw(`
		SELECT * FROM (
			SELECT DISTINCT ON (s.code, s.source) s.*
			FROM shadow_finding_signals s
			JOIN shadow_findings g ON g.id = s.finding_id
			JOIN shadow_findings f ON f.ou_id = g.ou_id AND f.provider = g.provider AND f.kind = g.kind
				AND f.external_id = g.external_id
			WHERE f.ou_id = ? AND f.id = ? AND s.ou_id = ?
			ORDER BY s.code, s.source, s.detected_at DESC
		) evidence ORDER BY code ASC, source ASC`, ouID, findingID, ouID).Scan(&out).Error
	return out, err
}

func (r *shadowAgentRepository) UpdateFindingStatus(ctx context.Context, ouID string, id uuid.UUID, status string) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE shadow_findings f SET status = ?, updated_at = NOW()
		FROM shadow_findings o
		WHERE o.ou_id = ? AND o.id = ?
			AND f.ou_id = o.ou_id AND f.provider = o.provider AND f.kind = o.kind AND f.external_id = o.external_id`,
		status, ouID, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *shadowAgentRepository) DeleteFinding(ctx context.Context, ouID string, id uuid.UUID) (bool, error) {
	// Evidence and per-scan links go with their rows (ON DELETE CASCADE).
	res := r.db.WithContext(ctx).Exec(`
		DELETE FROM shadow_findings f
		USING shadow_findings o
		WHERE o.ou_id = ? AND o.id = ?
			AND f.ou_id = o.ou_id AND f.provider = o.provider AND f.kind = o.kind AND f.external_id = o.external_id`,
		ouID, id)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *shadowAgentRepository) DeleteFindings(ctx context.Context, ouID string) (int64, error) {
	var agents int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT COUNT(DISTINCT (provider, kind, external_id)) FROM shadow_findings WHERE ou_id = ?`,
			ouID).Scan(&agents).Error; err != nil {
			return fmt.Errorf("counting agents: %w", err)
		}
		if err := tx.Exec(`DELETE FROM shadow_findings WHERE ou_id = ?`, ouID).Error; err != nil {
			return fmt.Errorf("deleting agents: %w", err)
		}
		return nil
	})
	return agents, err
}

func (r *shadowAgentRepository) Summary(ctx context.Context, ouID string) (ShadowSummary, error) {
	sum := ShadowSummary{ByProvider: map[string]int{}, ByStatus: map[string]int{}}
	var rows []struct {
		Provider string `gorm:"column:provider"`
		Status   string `gorm:"column:status"`
		N        int    `gorm:"column:n"`
	}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT provider, status, COUNT(*) AS n FROM (
			SELECT DISTINCT ON (provider, kind, external_id) provider, status
			FROM shadow_findings WHERE ou_id = ? AND present
			ORDER BY provider, kind, external_id, last_seen_at DESC
		) agents GROUP BY provider, status`, ouID).Scan(&rows).Error; err != nil {
		return sum, err
	}
	for _, row := range rows {
		sum.Total += row.N
		sum.ByProvider[row.Provider] += row.N
		sum.ByStatus[row.Status] += row.N
	}
	var conns int64
	if err := r.db.WithContext(ctx).Model(&models.ShadowConnection{}).Where("ou_id = ?", ouID).Count(&conns).Error; err != nil {
		return sum, err
	}
	sum.Connections = int(conns)
	return sum, nil
}

// ---- helpers ----

func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encoding JSON: %w", err)
	}
	return string(b), nil
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func ownerHintJSON(h models.ShadowOwnerHint) (string, error) {
	if h == (models.ShadowOwnerHint{}) {
		return "{}", nil
	}
	return toJSON(h)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// IsShadowNotFound reports gorm's not-found error.
func IsShadowNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
