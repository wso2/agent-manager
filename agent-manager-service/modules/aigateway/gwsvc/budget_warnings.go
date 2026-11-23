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

package gwsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"
)

// GatewayMessageBudgetUsage is the type of a gateway's usage message (Interfaces 4.2),
// dotted like deployment.ack.
const GatewayMessageBudgetUsage = "budget.usage"

// GatewayUsageMessage is the usage message a gateway sends on its connection
// (Interfaces 4.2): budget allowance totals.
type GatewayUsageMessage struct {
	Allowances []GatewayAllowanceUsage `json:"allowances"`
	// Costs are the AI cost stats, from gateways with [ai_cost_management.tracking] on (Interfaces 4.2).
	Costs []GatewayCostUsage `json:"costs"`
}

// GatewayAllowanceUsage is one allowance's total in one period, across every gateway
// sharing the spend tracker.
type GatewayAllowanceUsage struct {
	Budget           string            `json:"budget"`
	PeriodStart      string            `json:"periodStart"`
	AllowanceUUID    string            `json:"allowanceUuid"`
	Allowance        map[string]string `json:"allowance"`
	Spent            string            `json:"spent"`
	UnpricedRequests int64             `json:"unpricedRequests"`
}

// BudgetWarner checks a budget's thresholds against its stored spend; run when a
// budget is edited, since a lower limit or a new threshold may already be crossed.
type BudgetWarner interface {
	CheckBudget(ctx context.Context, b *gwmodels.Budget)
}

// BudgetWarningService keeps the spend gateways report for each budget allowance and
// raises a warning, once, for every threshold an allowance crosses (Interfaces 1.2, 6):
// sent to the budget's notification channels through notifications/v1, queued in the
// transaction that records the crossing.
type BudgetWarningService struct {
	budgets  gwrepo.BudgetRepository
	usage    gwrepo.BudgetUsageRepository
	gateways gwrepo.GatewayRepository
	costs    CostRecorder
	// notifier sends warnings to channels; nil records spend without warnings.
	notifier notifyv1.Notifier

	// Reports are coalesced here, per organization, latest total per allowance and cost key,
	// and written every flush interval, so the database load follows how many totals
	// changed, not how many reports, gateways or requests there were (Interfaces 4.2). A
	// gateway's connection never waits on the database. Before Start (tests), HandleUsage
	// records inline.
	mu       sync.Mutex
	pending  map[string]*pendingUsage
	ouOf     sync.Map // gateway UUID -> organization; a gateway never changes organization
	flush    time.Duration
	started  atomic.Bool
	stopCh   chan struct{}
	stopOnce sync.Once
	done     sync.WaitGroup
}

// pendingUsage is one organization's totals waiting to be written: the highest total
// reported for each allowance period and each cost month and key.
type pendingUsage struct {
	allowances map[allowanceUsageKey]GatewayAllowanceUsage
	costs      map[costUsageKey]GatewayCostUsage
}

type (
	allowanceUsageKey struct{ budget, allowance, period string }
	costUsageKey      struct{ month, key string }
)

// defaultUsageFlush is the flush interval when the settings give none.
const defaultUsageFlush = 2 * time.Second

// NewBudgetWarningService creates a BudgetWarningService; settings give the flush interval.
func NewBudgetWarningService(
	budgets gwrepo.BudgetRepository,
	usage gwrepo.BudgetUsageRepository,
	gateways gwrepo.GatewayRepository,
	costs CostRecorder,
	notifier notifyv1.Notifier,
	settings gwconfig.Settings,
) *BudgetWarningService {
	flush := settings.BudgetUsageFlush
	if flush <= 0 {
		flush = defaultUsageFlush
	}
	return &BudgetWarningService{
		budgets: budgets, usage: usage, gateways: gateways, costs: costs, notifier: notifier,
		pending: map[string]*pendingUsage{}, flush: flush, stopCh: make(chan struct{}),
	}
}

// publisher returns how a newly recorded warning is sent, or nil when warnings are not
// sent.
func (s *BudgetWarningService) publisher() gwrepo.WarningPublisher {
	if s.notifier == nil {
		return nil
	}
	return func(ctx context.Context, tx *gorm.DB, w gwmodels.BudgetWarning) error {
		return s.notifier.Notify(ctx, tx, w.Notice)
	}
}

// Start writes the coalesced totals every flush interval until Stop, and once more then.
func (s *BudgetWarningService) Start(ctx context.Context) error {
	s.started.Store(true)
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		ticker := time.NewTicker(s.flush)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.writePending(context.WithoutCancel(ctx))
				return
			case <-s.stopCh:
				s.writePending(ctx)
				return
			case <-ticker.C:
				s.writePending(ctx)
			}
		}
	}()
	return nil
}

// Stop writes what is pending and stops. Totals a crashed replica never wrote reach
// another one: its gateways reconnect there and send every current total again.
func (s *BudgetWarningService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.done.Wait()
	})
	return nil
}

// HandleUsage takes a gateway's usage message from its connection: coalesced into the
// pending totals once started (no database work), recorded inline before that.
func (s *BudgetWarningService) HandleUsage(ctx context.Context, gatewayID string, payload json.RawMessage) {
	if !s.started.Load() {
		s.RecordUsage(ctx, gatewayID, payload)
		return
	}
	log := slog.With("gatewayID", gatewayID)
	var msg GatewayUsageMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		log.Error("Failed to parse budget usage", "error", err)
		return
	}
	ouID, ok := s.orgOf(gatewayID, log)
	if !ok {
		return
	}
	log.Debug("Budget usage received", "allowances", len(msg.Allowances), "costs", len(msg.Costs))
	s.coalesce(ouID, msg)
}

// coalesce merges totals into the organization's pending ones, keeping the higher.
func (s *BudgetWarningService) coalesce(ouID string, msg GatewayUsageMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pending[ouID]
	if p == nil {
		p = &pendingUsage{allowances: map[allowanceUsageKey]GatewayAllowanceUsage{}, costs: map[costUsageKey]GatewayCostUsage{}}
		s.pending[ouID] = p
	}
	for _, a := range msg.Allowances {
		k := allowanceUsageKey{a.Budget, a.AllowanceUUID, a.PeriodStart}
		if old, seen := p.allowances[k]; seen {
			if !gwutil.DecimalGreater(a.Spent, old.Spent) {
				a.Spent = old.Spent
			}
			a.UnpricedRequests = max(a.UnpricedRequests, old.UnpricedRequests)
		}
		p.allowances[k] = a
	}
	for _, c := range msg.Costs {
		k := costUsageKey{c.Month, c.CostKey}
		if old, seen := p.costs[k]; seen {
			if !gwutil.DecimalGreater(c.Cost, old.Cost) {
				c.Cost = old.Cost
			}
			c.MissingPriceRequests = max(c.MissingPriceRequests, old.MissingPriceRequests)
			c.UsageUnavailableRequests = max(c.UsageUnavailableRequests, old.UsageUnavailableRequests)
		}
		p.costs[k] = c
	}
}

// orgOf returns the organization of a gateway, read once and then kept.
func (s *BudgetWarningService) orgOf(gatewayID string, log *slog.Logger) (string, bool) {
	if ou, ok := s.ouOf.Load(gatewayID); ok {
		return ou.(string), true
	}
	gateway, err := s.gateways.GetByUUID(gatewayID)
	if err != nil || gateway == nil {
		log.Error("Failed to find the gateway of a budget usage report", "error", err)
		return "", false
	}
	s.ouOf.Store(gatewayID, gateway.OUID)
	return gateway.OUID, true
}

// writePending writes every organization's coalesced totals: one batch per table.
func (s *BudgetWarningService) writePending(ctx context.Context) {
	s.mu.Lock()
	pending := s.pending
	s.pending = map[string]*pendingUsage{}
	s.mu.Unlock()
	for ouID, p := range pending {
		msg := GatewayUsageMessage{
			Allowances: make([]GatewayAllowanceUsage, 0, len(p.allowances)),
			Costs:      make([]GatewayCostUsage, 0, len(p.costs)),
		}
		for _, a := range p.allowances {
			msg.Allowances = append(msg.Allowances, a)
		}
		for _, c := range p.costs {
			msg.Costs = append(msg.Costs, c)
		}
		started := time.Now()
		result := s.recordForOrg(ctx, ouID, msg, slog.With("orgId", ouID))
		if result.costsFailed || len(result.heldAllowances) > 0 {
			keep := GatewayUsageMessage{Allowances: result.heldAllowances}
			if result.costsFailed {
				keep.Costs = msg.Costs
			}
			s.coalesce(ouID, keep)
		}
		slog.Debug("Budget usage written", "orgId", ouID, "allowances", len(msg.Allowances),
			"costs", len(msg.Costs), "took", time.Since(started))
	}
}

// RecordUsage records a gateway's usage message: its costs in one transaction and its
// allowances in another. An allowance whose budget is unknown or whose fields are invalid
// is skipped and logged; the gateway sends totals again.
func (s *BudgetWarningService) RecordUsage(ctx context.Context, gatewayID string, payload json.RawMessage) {
	log := slog.With("gatewayID", gatewayID)
	var msg GatewayUsageMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		log.Error("Failed to parse budget usage", "error", err)
		return
	}
	ouID, ok := s.orgOf(gatewayID, log)
	if !ok {
		return
	}
	s.recordForOrg(ctx, ouID, msg, log)
}

// recordForOrg writes an organization's totals: its costs in one transaction and its
// allowances in another. An allowance whose budget is unknown or whose fields are invalid
// is skipped and logged; the gateway sends totals again.
func (s *BudgetWarningService) recordForOrg(ctx context.Context, ouID string, msg GatewayUsageMessage, log *slog.Logger) usageWriteResult {
	var err error
	var result usageWriteResult
	if len(msg.Costs) > 0 && s.costs != nil {
		result.costsFailed = s.costs.RecordCosts(ctx, ouID, msg.Costs) != nil
	}
	budgets := map[string]*gwmodels.Budget{}
	byID := map[uuid.UUID]*gwmodels.Budget{}
	rows := make([]gwmodels.BudgetUsage, 0, len(msg.Allowances))
	for _, a := range msg.Allowances {
		b, ok := budgets[a.Budget]
		if !ok {
			b, err = s.budgets.Get(ctx, ouID, a.Budget)
			if errors.Is(err, gwerr.ErrBudgetNotFound) {
				b = nil
			} else if err != nil {
				log.Error("Failed to load a budget for its usage", "budget", a.Budget, "error", err)
				result.heldAllowances = msg.Allowances
				return result
			}
			budgets[a.Budget] = b
			if b != nil {
				byID[b.ID] = b
			}
		}
		if b == nil {
			log.Debug("Usage for an unknown budget ignored", "budget", a.Budget)
			continue
		}
		usage, err := usageRow(b, ouID, a)
		if err != nil {
			log.Warn("Invalid budget usage ignored", "budget", a.Budget, "error", err)
			continue
		}
		rows = append(rows, usage)
	}
	if !s.record(ctx, byID, rows, log) {
		result.heldAllowances = msg.Allowances
	}
	return result
}

// usageWriteResult says which of an organization's totals were not written; the caller
// keeps them for the next flush, since a quiet gateway may not report them again.
type usageWriteResult struct {
	costsFailed    bool
	heldAllowances []GatewayAllowanceUsage
}

// usageRow validates one reported allowance.
func usageRow(b *gwmodels.Budget, ouID string, a GatewayAllowanceUsage) (gwmodels.BudgetUsage, error) {
	allowanceID, err := uuid.Parse(a.AllowanceUUID)
	if err != nil {
		return gwmodels.BudgetUsage{}, fmt.Errorf("allowanceUuid: %w", err)
	}
	periodStart, err := time.Parse(time.DateOnly, a.PeriodStart)
	if err != nil {
		return gwmodels.BudgetUsage{}, fmt.Errorf("periodStart: %w", err)
	}
	spent, ok := new(big.Rat).SetString(a.Spent)
	if !ok || spent.Sign() < 0 || a.UnpricedRequests < 0 {
		return gwmodels.BudgetUsage{}, fmt.Errorf("spent %q or unpricedRequests %d", a.Spent, a.UnpricedRequests)
	}
	return gwmodels.BudgetUsage{
		BudgetID: b.ID, AllowanceID: allowanceID, OUID: ouID,
		Allowance: allowanceValues(a.Allowance), PeriodStart: periodStart,
		Spent: a.Spent, UnpricedRequests: a.UnpricedRequests,
	}, nil
}

// allowanceValues are the values an allowance is separated by, as pages and warnings
// show them: an agent ("project/agent" on the gateway) is split into project and agent.
func allowanceValues(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if k == budgetTypeAgent {
			if project, agent, ok := strings.Cut(v, "/"); ok {
				out[budgetTypeProject], out[budgetTypeAgent] = project, agent
				continue
			}
		}
		out[k] = v
	}
	return out
}

// record stores the allowances' totals in one transaction and records the warnings they
// cross, raising each warning's event in that transaction.
func (s *BudgetWarningService) record(ctx context.Context, budgets map[uuid.UUID]*gwmodels.Budget,
	rows []gwmodels.BudgetUsage, log *slog.Logger,
) bool {
	if len(rows) == 0 {
		return true
	}
	publish := s.publisher()
	var warn func(gwmodels.BudgetUsage) []gwmodels.BudgetWarning
	if publish != nil {
		warn = func(stored gwmodels.BudgetUsage) []gwmodels.BudgetWarning {
			b := budgets[stored.BudgetID]
			if b == nil {
				return nil
			}
			return budgetWarnings(b, stored)
		}
	}
	if err := s.usage.Record(ctx, rows, warn, publish); err != nil {
		log.Error("Failed to record budget usage", "allowances", len(rows), "error", err)
		return false
	}
	return true
}

// CheckBudget raises the warnings a budget's stored spend already crosses: after an
// edit lowers the limit or adds a threshold. Raising the limit gives new amounts, so
// warnings re-arm (BR-15).
func (s *BudgetWarningService) CheckBudget(ctx context.Context, b *gwmodels.Budget) {
	log := slog.With("budget", b.Name)
	publish := s.publisher()
	if publish == nil || b.Configuration.Warnings == nil || len(b.Configuration.Warnings.Thresholds) == 0 {
		return
	}
	rows, err := s.usage.ForBudget(ctx, b.ID)
	if err != nil {
		log.Error("Failed to load a budget's spend", "error", err)
		return
	}
	var warnings []gwmodels.BudgetWarning
	for _, row := range rows {
		warnings = append(warnings, budgetWarnings(b, row)...)
	}
	if err := s.usage.RecordWarnings(ctx, warnings, publish); err != nil {
		log.Error("Failed to raise budget warnings", "error", err)
	}
}

// budgetWarnings returns a warning for every threshold the stored spend has reached,
// with its event. A warning is keyed by the budget, allowance, period and threshold amount,
// so each is raised once (BR-13).
func budgetWarnings(b *gwmodels.Budget, row gwmodels.BudgetUsage) []gwmodels.BudgetWarning {
	w := b.Configuration.Warnings
	if w == nil || b.Configuration.Status != budgetStatusActive {
		return nil
	}
	amount, ok := new(big.Rat).SetString(b.Configuration.Limit.Amount)
	if !ok || amount.Sign() <= 0 {
		return nil
	}
	spent, ok := new(big.Rat).SetString(row.Spent)
	if !ok {
		return nil
	}
	var out []gwmodels.BudgetWarning
	for _, t := range w.Thresholds {
		at := new(big.Rat).Mul(amount, big.NewRat(int64(t), 100))
		if spent.Cmp(at) < 0 {
			continue
		}
		out = append(out, gwmodels.BudgetWarning{
			BudgetID: b.ID, AllowanceID: row.AllowanceID, PeriodStart: row.PeriodStart.UTC(),
			Amount: at.FloatString(2), OUID: b.OrgID, Threshold: int(t),
			Notice: warningNotice(b, row, int(t), at, spent),
		})
	}
	return out
}

// BudgetWarningEvent is the type of a budget warning sent to its channels.
const BudgetWarningEvent = "ai-gateway.budget.threshold_crossed"

// warningNotice is the warning of one threshold crossed, for the budget's channels
// (Interfaces 6). Its ID is the budget, period, allowance and threshold amount, so each
// reaches a channel once (BR-13).
func warningNotice(b *gwmodels.Budget, row gwmodels.BudgetUsage, threshold int, at, spent *big.Rat) notifyv1.Notification {
	period := row.PeriodStart.UTC()
	project := row.Allowance[budgetTypeProject]
	if project == "" {
		project = budgetProject(&b.Configuration)
	}
	agent := ""
	if project != "" {
		agent = row.Allowance[budgetTypeAgent]
	}
	limit := b.Configuration.Limit
	resets := periodEnd(limit.Period, period)
	displayName := b.Configuration.DisplayName
	if displayName == "" {
		displayName = b.Name
	}
	var channels []string
	if w := b.Configuration.Warnings; w != nil {
		channels = w.Channels
	}
	return notifyv1.Notification{
		OrgID: b.OrgID, OrgName: b.OrgName, Channels: channels, Type: BudgetWarningEvent,
		ID:      fmt.Sprintf("budget:%s:%s:%s:%s", b.ID, period.Format(time.DateOnly), row.AllowanceID, at.FloatString(2)),
		Project: project, Agent: agent, Subject: b.Name,
		Data: map[string]any{
			"budget":      b.Name,
			"displayName": displayName,
			"allowance":   row.Allowance,
			"limit":       limit.Amount,
			"action":      limit.Action,
			"threshold":   threshold,
			"spent":       spent.FloatString(2),
			"resetsAt":    resets.Format(time.RFC3339),
			"message": fmt.Sprintf("%s at %d%% ($%s of $%s), resets %s", allowanceLabel(row.Allowance), threshold,
				spent.FloatString(2), amountString(limit.Amount), resets.Format("2 Jan")),
		},
	}
}

// allowanceLabel names an allowance in a warning: its values, or "Spend" when the
// budget is shared.
func allowanceLabel(values map[string]string) string {
	if len(values) == 0 {
		return "Spend"
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		if k == budgetTypeProject && values[budgetTypeAgent] != "" {
			continue // the agent names it
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = values[k]
	}
	return strings.Join(parts, ", ")
}

func amountString(amount string) string {
	if r, ok := new(big.Rat).SetString(amount); ok {
		return r.FloatString(2)
	}
	return amount
}

// periodEnd is when a budget period starting at start resets (UTC calendar; weeks
// start Monday).
func periodEnd(period string, start time.Time) time.Time {
	switch period {
	case "day":
		return start.AddDate(0, 0, 1)
	case "week":
		return start.AddDate(0, 0, 7)
	default:
		return start.AddDate(0, 1, 0)
	}
}
